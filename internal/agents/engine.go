// Package agents implements Bhagirath's five agents (spec Section 4.3):
//
//	Intake      audio/text/form → structured, located, de-duplicated complaint
//	Fusion      all signals → frozen rule → alert with plain-language reason
//	Escalation  alert/complaint → right official, evidence pack, deadline, auto-escalate
//	Closure     "fixed" claim → evidence + complainant callback → verified or reopened
//	Advisory    Warning+ alert → precautionary message in the household's language
//
// Every state change is written with an audit event in the same transaction.
package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vivekyarra/bhagirath/internal/asr"
	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/extract"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/lang"
	"github.com/vivekyarra/bhagirath/internal/rule"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// Engine wires the agents to their dependencies.
type Engine struct {
	Store     *store.Store
	Clock     clock.Clock
	Rule      *rule.Rule
	Ladder    *config.Ladder
	Vault     *secure.Vault
	Extractor extract.Extractor
	ASR       asr.Transcriber
	Cfg       *config.Config
	Log       *slog.Logger

	evalMu   sync.Mutex
	evalRuns map[string]*wardRun
}

func (e *Engine) now() time.Time { return e.Clock.Now() }

func (e *Engine) event(ctx context.Context, st *store.Store, actor, action, entity string, id any, ward string, details any) {
	var raw json.RawMessage
	if details != nil {
		raw, _ = json.Marshal(details)
	}
	ev := domain.Event{At: e.now(), Actor: actor, Action: action, Entity: entity, EntityID: fmt.Sprint(id), WardID: ward, Details: raw}
	if err := st.LogEvent(ctx, ev); err != nil && e.Log != nil {
		e.Log.Error("audit log write failed", "action", action, "err", err)
	}
}

func (e *Engine) partnerName(w domain.Ward) string {
	if w.PartnerName != "" {
		return w.PartnerName
	}
	return e.Cfg.PartnerName
}

// messageToReporter enqueues a message to a reporter on their channel.
// whatsappOnly messages (acks, follow-up questions) are skipped for
// voice-only reporters, who hear the equivalent inside the IVR call.
func (e *Engine) messageToReporter(ctx context.Context, st *store.Store, r domain.Reporter, kind, body string, whatsappOnly bool, entity string, entityID int64, interactive any) (int64, error) {
	if len(r.PhoneEnc) == 0 {
		return 0, nil
	}
	channel := r.PreferredChannel
	if channel == "" {
		channel = "voice"
	}
	if whatsappOnly && channel != "whatsapp" {
		return 0, nil
	}
	m := domain.OutboundMessage{Kind: kind, Channel: channel, RecipientType: "reporter", RecipientID: r.ID, RecipientLabel: r.Ref,
		Language: lang.Pick(r.PreferredLanguage, "hi"), Body: body, Entity: entity, EntityID: entityID,
		Status: domain.MsgQueued, CreatedAt: e.now(), NextAttemptAt: e.now()}
	if interactive != nil {
		m.Interactive, _ = json.Marshal(interactive)
	}
	if err := st.EnqueueMessage(ctx, &m); err != nil {
		return 0, err
	}
	return m.ID, nil
}

// Interactive describes buttons (WhatsApp) / keypad options (voice) for a message.
type Interactive struct {
	Kind         string `json:"kind"` // callback | ack
	ClosureID    int64  `json:"closure_id,omitempty"`
	EscalationID int64  `json:"escalation_id,omitempty"`
}

func issueLabel(it domain.IssueType, l string) string { return i18n.Issue(string(it), l) }

func ptr[T any](v T) *T { return &v }

// Fork returns an engine sharing this one's dependencies but with its own
// clock and transcriber (used to replay seeded history). Coalescing state is
// not shared.
func (e *Engine) Fork(c clock.Clock, tr asr.Transcriber) *Engine {
	return &Engine{Store: e.Store, Clock: c, Rule: e.Rule, Ladder: e.Ladder, Vault: e.Vault, Extractor: e.Extractor, ASR: tr, Cfg: e.Cfg, Log: e.Log}
}
