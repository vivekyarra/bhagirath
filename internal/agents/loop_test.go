package agents_test

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/asr"
	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/extract"
	"github.com/vivekyarra/bhagirath/internal/notify"
	"github.com/vivekyarra/bhagirath/internal/rule"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
	"github.com/vivekyarra/bhagirath/internal/store/storetest"
)

const ward = "demo-urban-12"

type harness struct {
	t   *testing.T
	ctx context.Context
	e   *agents.Engine
	st  *store.Store
	clk *clock.Fixed
	d   *notify.Dispatcher
}

func newHarness(t *testing.T, mutate ...func(*config.Config)) *harness {
	t.Helper()
	st := storetest.New(t)
	r, err := rule.Load("../../config/alert_rule_v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	l, err := config.LoadLadder("../../config/ladder.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := secure.NewVault([]byte(strings.Repeat("s", 32)))
	t.Setenv("BHAGIRATH_MODE", "demo")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BaseURL = "https://bhagirath.test"
	for _, m := range mutate {
		m(cfg)
	}
	clk := clock.NewFixed(time.Date(2026, 11, 6, 4, 0, 0, 0, time.UTC))
	e := &agents.Engine{Store: st, Clock: clk, Rule: r, Ladder: l, Vault: v, Extractor: extract.Rules{}, ASR: asr.None{}, Cfg: cfg,
		Log: slog.New(slog.DiscardHandler)}
	if err := e.SyncLadder(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, ctx: context.Background(), e: e, st: st, clk: clk,
		d: &notify.Dispatcher{Store: st, Vault: v, Clock: clk, Cfg: cfg}}
}

func (h *harness) whatsapp(phone, text string, p *domain.Point) agents.Outcome {
	h.t.Helper()
	out, err := h.e.HandleMessage(h.ctx, agents.Message{Phone: phone, Text: text, Point: p, ProviderMsgID: phone + text + h.clk.Now().String()})
	if err != nil {
		h.t.Fatalf("message %q: %v", text, err)
	}
	return out
}

func (h *harness) alert() domain.Alert {
	h.t.Helper()
	as, err := h.st.ListAlerts(h.ctx, store.AlertFilter{WardID: ward, Status: "active"})
	if err != nil {
		h.t.Fatal(err)
	}
	if len(as) == 0 {
		return domain.Alert{}
	}
	return as[0]
}

func (h *harness) escalations(f store.EscalationFilter) []domain.Escalation {
	h.t.Helper()
	es, err := h.st.ListEscalations(h.ctx, f)
	if err != nil {
		h.t.Fatal(err)
	}
	return es
}

func (h *harness) count(action string) int {
	n, err := h.st.CountEvents(h.ctx, action)
	if err != nil {
		h.t.Fatal(err)
	}
	return n
}

func near(dy, dx float64) *domain.Point {
	return &domain.Point{Lat: 22.7200 + dy/111_195, Lon: 75.8600 + dx/102_550}
}

// TestFullAccountabilityLoop drives the demo story end to end:
// complaints → Watch → failed test → Warning → advisories → timeout
// escalation → fake closure caught → verified fix → illness → Critical.
func TestFullAccountabilityLoop(t *testing.T) {
	h := newHarness(t)
	const a, b, c = "+910000000001", "+910000000002", "+910000000003"

	// 1. A Hindi complaint naming a landmark resolves to the ward and is escalated to rung 1.
	out := h.whatsapp(a, "शिव मंदिर के पास नल से बदबूदार पानी आ रहा है", nil)
	if out.Handled != "complaint" || out.Complaint.WardID != ward || out.Complaint.LocationMethod != domain.LocLandmark || out.Complaint.IssueType != domain.IssueFoulSmell {
		t.Fatalf("intake: %+v", out.Complaint)
	}
	if out.Complaint.Status != domain.StatusEscalated {
		t.Fatalf("complaint not escalated: %s", out.Complaint.Status)
	}
	cA := *out.Complaint
	if es := h.escalations(store.EscalationFilter{ComplaintID: cA.ID}); len(es) != 1 || es[0].Level != domain.Ladder1 {
		t.Fatalf("complaint escalations: %+v", es)
	}
	// Consent was requested; "हाँ" grants it.
	if out := h.whatsapp(a, "हाँ", nil); out.Handled != "consent" {
		t.Fatalf("consent reply handled as %s", out.Handled)
	}
	// Webhook retries are idempotent.
	dupe, err := h.e.HandleMessage(h.ctx, agents.Message{Phone: a, Text: "x", ProviderMsgID: "wamid.same"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.e.HandleMessage(h.ctx, agents.Message{Phone: a, Text: "x", ProviderMsgID: "wamid.same"})
	if err != nil || again.Complaint == nil || again.Complaint.ID != dupe.Complaint.ID {
		t.Fatalf("retry created a second complaint: %+v %v", again, err)
	}
	if h.alert().ID != 0 {
		t.Fatal("alert raised from a single household")
	}

	// 2. Two more households within 300 m → complaint cluster → Watch.
	h.clk.Advance(2 * time.Hour)
	h.whatsapp(b, "गंदा पानी आ रहा है, बहुत बदबू", near(120, 40))
	h.clk.Advance(3 * time.Hour)
	h.whatsapp(c, "dirty water with smell from the tap", near(-80, 150))
	al := h.alert()
	if al.Level != domain.LevelWatch || !strings.Contains(al.ReasonText, "3 households within") {
		t.Fatalf("expected Watch, got %v %q", al.Level, al.ReasonText)
	}
	if es := h.escalations(store.EscalationFilter{AlertID: al.ID}); len(es) != 1 || es[0].Level != domain.Ladder1 {
		t.Fatalf("watch escalations: %+v", es)
	}

	// 3. A failed coliform test → Warning → rungs 1, 2 and H; advisories wait for partner sign-off.
	val, lim := 9.0, 0.0
	if _, err := h.e.AddWaterTest(h.ctx, domain.WaterTest{Source: domain.TestFieldKit, WardID: ward, SampledAt: h.clk.Now().Add(-time.Hour),
		Parameter: "Total coliform", Value: &val, LimitValue: &lim, Unit: "MPN/100ml"}, "partner"); err != nil {
		t.Fatal(err)
	}
	al = h.alert()
	if al.Level != domain.LevelWarning || !strings.HasSuffix(al.ReasonText, "Two signals of three.") {
		t.Fatalf("expected Warning, got %v %q", al.Level, al.ReasonText)
	}
	rungs := map[domain.LadderLevel]int{}
	for _, x := range h.escalations(store.EscalationFilter{AlertID: al.ID}) {
		rungs[x.Level]++
	}
	if rungs[domain.Ladder1] != 1 || rungs[domain.Ladder2] != 1 || rungs[domain.LadderHealth] != 1 {
		t.Fatalf("warning rungs: %v", rungs)
	}
	if h.count("advisory.awaiting_signoff") != 1 || h.count("advisory.sent") != 0 {
		t.Fatal("advisories must wait for partner sign-off")
	}
	sent, err := h.e.ApproveAdvisories(h.ctx, al.ID, "partner")
	if err != nil || sent != 1 { // only household A consented
		t.Fatalf("advisories sent=%d err=%v", sent, err)
	}
	if again, _ := h.e.SendAdvisories(h.ctx, al.ID); again != 0 {
		t.Fatal("advisories re-sent to the same household")
	}
	advs, _ := h.st.ListAdvisories(h.ctx, al.ID)
	if len(advs) != 1 || !strings.Contains(advs[0].Text, "यह कोई सरकारी आदेश नहीं है") {
		t.Fatalf("advisory text/label wrong: %+v", advs)
	}

	// 4. Nobody acknowledges: after 48 h the Warning escalations time out and go to rung 3.
	h.clk.Advance(49 * time.Hour)
	n, err := h.e.ProcessTimeouts(h.ctx)
	if err != nil || n < 2 {
		t.Fatalf("timeouts n=%d err=%v", n, err)
	}
	var l3 int
	for _, x := range h.escalations(store.EscalationFilter{AlertID: al.ID}) {
		if x.Level == domain.Ladder3 && x.Outcome == domain.OutcomePending && x.Reason == "timeout" {
			l3++
		}
	}
	if l3 != 1 {
		t.Fatalf("expected exactly one rung-3 escalation (deduplicated), got %d", l3)
	}

	// 5. The AE acknowledges A's complaint and claims a fix with a photo; A says it is NOT fixed.
	esA := h.escalations(store.EscalationFilter{ComplaintID: cA.ID})
	var pendingA domain.Escalation
	for _, x := range esA {
		if x.Outcome == domain.OutcomePending || x.Outcome == domain.OutcomeTimedOut {
			pendingA = x
		}
	}
	if pendingA.Outcome == domain.OutcomeTimedOut {
		// The 72 h complaint deadline may have passed already; use the newest pending one.
		for _, x := range esA {
			if x.Outcome == domain.OutcomePending {
				pendingA = x
			}
		}
	}
	if err := h.e.Acknowledge(h.ctx, pendingA.ID, "link", "", "official"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.e.ClaimFixed(h.ctx, agents.Claim{ComplaintID: cA.ID}); !errors.Is(err, agents.ErrNoEvidence) {
		t.Fatalf("claim without evidence accepted: %v", err)
	}
	photo, _ := h.st.SaveMedia(h.ctx, domain.Media{SHA256: "x", ContentType: "image/jpeg", Data: []byte{1}, Kind: "photo", CreatedAt: h.clk.Now()})
	cl, err := h.e.ClaimFixed(h.ctx, agents.Claim{ComplaintID: cA.ID, EscalationID: &pendingA.ID, ClaimedBy: "Demo AE", PhotoMediaID: &photo})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := h.st.GetComplaint(h.ctx, cA.ID)
	if got.Status != domain.StatusClaimedFixed {
		t.Fatalf("status after claim = %s", got.Status)
	}
	cbs, _ := h.st.ListMessages(h.ctx, store.MessageFilter{Kind: "callback"})
	if len(cbs) != 1 || !strings.Contains(cbs[0].Body, cA.Ref) {
		t.Fatalf("callback not sent: %+v", cbs)
	}
	if out := h.whatsapp(a, "2", nil); out.Handled != "callback" {
		t.Fatalf("callback answer handled as %s", out.Handled)
	}
	got, _ = h.st.GetComplaint(h.ctx, cA.ID)
	if got.Status != domain.StatusEscalated || got.ReopenCount != 1 {
		t.Fatalf("fake closure not caught: status=%s reopen=%d", got.Status, got.ReopenCount)
	}
	if h.count("closure.denied") != 1 || h.count("complaint.reopened") != 1 {
		t.Fatal("fake-closure metric events missing")
	}
	var up bool
	for _, x := range h.escalations(store.EscalationFilter{ComplaintID: cA.ID}) {
		up = up || (x.Reason == "reopened" && x.Level.Rank() >= 2 && x.Outcome == domain.OutcomePending)
	}
	if !up {
		t.Fatal("reopened complaint not escalated one level up")
	}
	if err := h.e.RecordCallback(h.ctx, cl.ID, true, "whatsapp", "x"); !errors.Is(err, agents.ErrAlreadyDecided) {
		t.Fatalf("decided closure changed: %v", err)
	}

	// 6. A real fix with a passing retest; A confirms via the WhatsApp button → verified.
	pass := 0.0
	rt, err := h.e.AddWaterTest(h.ctx, domain.WaterTest{Source: domain.TestFieldKit, WardID: ward, SampledAt: h.clk.Now(), Parameter: "Total coliform", Value: &pass, LimitValue: &lim}, "partner")
	if err != nil || rt.IsContaminated {
		t.Fatalf("retest: %+v %v", rt, err)
	}
	cl2, err := h.e.ClaimFixed(h.ctx, agents.Claim{ComplaintID: cA.ID, ClaimedBy: "Demo EE", RetestID: &rt.ID})
	if err != nil {
		t.Fatal(err)
	}
	out, err = h.e.HandleMessage(h.ctx, agents.Message{Phone: a, ReplyID: "cb:" + itoa(cl2.ID) + ":yes"})
	if err != nil || out.Handled != "callback" {
		t.Fatalf("button reply: %+v %v", out, err)
	}
	got, _ = h.st.GetComplaint(h.ctx, cA.ID)
	if got.Status != domain.StatusVerifiedFixed || got.VerifiedFixedAt == nil {
		t.Fatalf("not verified: %s", got.Status)
	}
	for _, x := range h.escalations(store.EscalationFilter{ComplaintID: cA.ID}) {
		if x.Outcome == domain.OutcomePending || x.Outcome == domain.OutcomeAcknowledged {
			t.Fatalf("escalation %s still open after verified fix", x.Ref)
		}
	}

	// 7. A new E. coli failure plus a clinic reporting 5 diarrhoea cases → T + I(≥5) → Critical,
	// reaching every rung including the partner.
	ec := 4.0
	if _, err := h.e.AddWaterTest(h.ctx, domain.WaterTest{Source: domain.TestLabReport, WardID: ward, SampledAt: h.clk.Now(), Parameter: "E. coli", Value: &ec, LimitValue: &lim, Unit: "MPN/100ml"}, "partner"); err != nil {
		t.Fatal(err)
	}
	clinic, err := h.e.RegisterSignalReporter(h.ctx, "+910000000099", domain.KindClinic, "Demo Clinic", ward, "hi", "admin")
	if err != nil {
		t.Fatal(err)
	}
	_ = clinic
	if out := h.whatsapp("+910000000099", "D 5 V 2 ORS 30", nil); out.Handled != "illness" {
		t.Fatalf("clinic message handled as %s (%s)", out.Handled, out.Detail)
	}
	al = h.alert()
	if al.Level != domain.LevelCritical || al.PeakLevel != domain.LevelCritical {
		t.Fatalf("expected Critical, got %v: %s", al.Level, al.ReasonText)
	}
	rungs = map[domain.LadderLevel]int{}
	for _, x := range h.escalations(store.EscalationFilter{AlertID: al.ID}) {
		rungs[x.Level]++
	}
	if rungs[domain.LadderPartner] == 0 || rungs[domain.Ladder3] == 0 {
		t.Fatalf("critical must reach rung 3 and partner: %v", rungs)
	}

	// 8. Everything in the outbox is delivered (simulated in demo mode); nothing is stuck.
	if _, err := h.d.Run(h.ctx); err != nil {
		t.Fatal(err)
	}
	msgs, _ := h.st.ListMessages(h.ctx, store.MessageFilter{Limit: 1000})
	for _, m := range msgs {
		if m.Status == domain.MsgQueued || m.Status == domain.MsgFailed {
			t.Fatalf("message %d (%s/%s) left %s: %s", m.ID, m.Kind, m.Channel, m.Status, m.Error)
		}
	}
	// No plain phone number ever reaches the database outside ciphertext.
	for _, m := range msgs {
		if strings.Contains(m.Body+m.RecipientLabel, "0000000001") {
			t.Fatal("phone number leaked into the outbox")
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
