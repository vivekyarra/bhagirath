package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/extract"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/lang"
	"github.com/vivekyarra/bhagirath/internal/locate"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// Inbound is a complaint arriving on any channel.
type Inbound struct {
	Channel       domain.Channel
	ProviderMsgID string
	Phone         string // raw; normalised and protected immediately
	Text          string
	Audio         []byte
	AudioType     string
	Point         *domain.Point
	LocationText  string
	WardHint      string
	IssueHint     domain.IssueType
	Language      string
	Consent       *bool // explicit answer (web checkbox, IVR keypress)
	HasSmartphone *bool
	At            time.Time
	Actor         string // "resident", "volunteer:<user>", "simulator"
	// Backfill marks complaints migrated from the manual pilot log kept before
	// the software went live; their original timestamps (up to 60 days old)
	// are preserved instead of being clamped to "now".
	Backfill bool
}

// MaxBackfill is how old a migrated manual-log complaint may be.
const MaxBackfill = 60 * 24 * time.Hour

// IntakeResult reports what happened.
type IntakeResult struct {
	Complaint domain.Complaint
	Reporter  *domain.Reporter
	Duplicate bool // provider retry or same-reporter repeat
	Location  locate.Result
	Extracted extract.Result
}

// ErrEmpty is returned when an inbound complaint has neither text nor audio.
var ErrEmpty = errors.New("complaint has no text or audio")

// dedupeWindow: a repeat from the same household about the same problem within
// this window is linked to the first complaint rather than counted again.
const dedupeWindow = 48 * time.Hour

// MaxAudioBytes bounds stored recordings.
const MaxAudioBytes = 16 << 20

// Intake runs the Intake Agent: protect PII, transcribe, extract, locate,
// de-duplicate, store, acknowledge, ask a follow-up if location is unclear,
// escalate, and immediately re-evaluate the ward's alert level.
func (e *Engine) Intake(ctx context.Context, in Inbound) (IntakeResult, error) {
	var res IntakeResult
	if strings.TrimSpace(in.Text) == "" && len(in.Audio) == 0 {
		return res, ErrEmpty
	}
	if len(in.Audio) > MaxAudioBytes {
		return res, fmt.Errorf("audio too large (%d bytes)", len(in.Audio))
	}
	if !domain.Contains(domain.Channels, in.Channel) {
		return res, fmt.Errorf("unknown channel %q", in.Channel)
	}
	now := e.now()
	at := in.At
	maxAge := 7 * 24 * time.Hour
	if in.Backfill {
		maxAge = MaxBackfill
	}
	if at.IsZero() || at.After(now) || now.Sub(at) > maxAge {
		if in.Backfill && !at.IsZero() {
			return res, fmt.Errorf("date %s is outside the last %d days", at.Format("2006-01-02"), int(MaxBackfill.Hours()/24))
		}
		at = now
	}
	if in.Actor == "" {
		in.Actor = "resident"
	}
	if in.ProviderMsgID != "" {
		fresh, err := e.Store.MarkInboundSeen(ctx, string(in.Channel), in.ProviderMsgID, now)
		if err != nil {
			return res, err
		}
		if !fresh {
			c, err := e.Store.GetComplaintByProviderMsg(ctx, in.Channel, in.ProviderMsgID)
			if err == nil {
				res.Complaint, res.Duplicate = c, true
				return res, nil
			}
			return res, fmt.Errorf("duplicate delivery %s: %w", in.ProviderMsgID, err)
		}
	}

	// Transcription happens outside the transaction (network call).
	transcript, asrText, tstatus := strings.TrimSpace(in.Text), "", domain.TranscriptNone
	langHint := in.Language
	if len(in.Audio) > 0 {
		tstatus = domain.TranscriptNeedsReview
		if transcript == "" && e.ASR != nil {
			tr, err := e.ASR.Transcribe(ctx, in.Audio, in.AudioType, langHint)
			if err == nil && strings.TrimSpace(tr.Text) != "" {
				transcript, asrText, tstatus = tr.Text, tr.Text, domain.TranscriptASR
				if tr.Language != "" {
					langHint = tr.Language
				}
			} else if err != nil && e.Log != nil {
				e.Log.Warn("asr failed; queued for human transcription", "err", err)
			}
		} else if transcript != "" {
			tstatus = domain.TranscriptReviewed // volunteer typed it while listening
		}
	}
	ex := extract.Result{IssueType: domain.IssueOther, Severity: 2, Language: langHint, Extractor: "none"}
	if in.IssueHint != "" {
		ex.IssueType = in.IssueHint
	}
	if transcript != "" {
		var err error
		ex, err = e.Extractor.Extract(ctx, transcript, in.IssueHint)
		if err != nil {
			return res, err
		}
	}
	if ex.Language == "" {
		ex.Language = langHint
	}
	res.Extracted = ex

	wards, err := e.Store.ListWards(ctx)
	if err != nil {
		return res, err
	}

	err = e.Store.Tx(ctx, func(tx *store.Store) error {
		var rep *domain.Reporter
		if strings.TrimSpace(in.Phone) != "" {
			r, created, err := e.upsertReporter(ctx, tx, in, ex.Language, now)
			if err != nil {
				return err
			}
			rep = &r
			if created {
				res.Reporter = rep
			}
		}
		var audioID *int64
		if len(in.Audio) > 0 {
			sum := sha256.Sum256(in.Audio)
			del := now.Add(e.Cfg.AudioRetention)
			id, err := tx.SaveMedia(ctx, domain.Media{SHA256: hex.EncodeToString(sum[:]), ContentType: in.AudioType, Data: in.Audio, Kind: "audio", CreatedAt: now, DeleteAfter: &del})
			if err != nil {
				return err
			}
			audioID = &id
		}
		li := locate.Input{Point: in.Point, Text: strings.Join([]string{in.LocationText, transcript, ex.Landmark}, " "), WardHint: in.WardHint}
		if rep != nil {
			li.ReporterHome, li.ReporterWard = rep.Home, rep.WardID
		}
		loc := locate.Resolve(li, wards)
		res.Location = loc
		locText := strings.TrimSpace(in.LocationText)
		if loc.Landmark != "" && !strings.Contains(locText, loc.Landmark) {
			locText = strings.TrimSpace(locText + " (landmark: " + loc.Landmark + ")")
		}
		extraction, _ := json.Marshal(map[string]any{"extract": ex, "location": loc})
		c := domain.Complaint{Channel: in.Channel, ProviderMsgID: in.ProviderMsgID, AudioMediaID: audioID, Transcript: transcript,
			TranscriptASR: asrText, TranscriptStatus: tstatus, Language: ex.Language, IssueType: ex.IssueType, Severity: max(1, min(5, ex.Severity)),
			DescriptionEN: extract.Describe(ex, loc.Landmark), Point: loc.Point, LocationText: locText, LocationMethod: loc.Method,
			WardID: loc.WardID, Status: domain.StatusOpen, NeedsFollowup: loc.NeedsFollowup(), Extraction: extraction, CreatedAt: at}
		if ex.DescriptionEN != "" && ex.Extractor != "rules" && ex.Extractor != "none" {
			c.DescriptionEN = ex.DescriptionEN
		}
		if rep != nil {
			c.ReporterID, c.ReporterRef = &rep.ID, rep.Ref
			if dup, ok, err := e.findDuplicate(ctx, tx, rep.ID, c, c.CreatedAt); err != nil {
				return err
			} else if ok {
				c.DuplicateOf = &dup.ID
				res.Duplicate = true
			}
		}
		if err := tx.CreateComplaint(ctx, &c); err != nil {
			return err
		}
		res.Complaint = c
		e.event(ctx, tx, in.Actor, "complaint.created", "complaint", c.Ref, c.WardID, map[string]any{
			"channel": c.Channel, "issue_type": c.IssueType, "location_method": c.LocationMethod, "duplicate": res.Duplicate, "backfill": in.Backfill,
			"transcript_status": c.TranscriptStatus, "extractor": ex.Extractor, "non_smartphone": c.Channel.NonSmartphone()})

		if rep != nil {
			if rep.WardID == "" && loc.WardID != "" {
				rep.WardID = loc.WardID
				if rep.Home == nil && (loc.Method == domain.LocGPS || loc.Method == domain.LocLandmark) {
					rep.Home = loc.Point
				}
				if err := tx.UpdateReporter(ctx, *rep); err != nil {
					return err
				}
			}
			res.Reporter = rep
			if !in.Backfill {
				if err := e.afterIntakeMessages(ctx, tx, *rep, c, loc, wards); err != nil {
					return err
				}
			}
		}
		if res.Duplicate {
			return nil
		}
		if ex.MentionsIllness && c.WardID != "" {
			if err := e.residentIllness(ctx, tx, c, ex); err != nil {
				return err
			}
		}
		if c.WardID != "" && c.IssueType != domain.IssueOther {
			if err := e.EscalateComplaint(ctx, tx, &res.Complaint, domain.Ladder1, "complaint", nil); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	if res.Complaint.WardID != "" && !res.Duplicate {
		if _, err := e.EvaluateWard(ctx, res.Complaint.WardID); err != nil && e.Log != nil {
			e.Log.Error("ward evaluation after intake failed", "ward", res.Complaint.WardID, "err", err)
		}
	}
	return res, nil
}

func (e *Engine) upsertReporter(ctx context.Context, tx *store.Store, in Inbound, detected string, now time.Time) (domain.Reporter, bool, error) {
	phone, err := secure.NormalizePhone(in.Phone)
	if err != nil {
		return domain.Reporter{}, false, err
	}
	hash := e.Vault.HashPhone(phone)
	enc, err := e.Vault.Encrypt(phone)
	if err != nil {
		return domain.Reporter{}, false, err
	}
	pref := "voice"
	if in.Channel == domain.ChannelWhatsApp {
		pref = "whatsapp"
	}
	l := lang.Pick(detected, "hi")
	r := domain.Reporter{Ref: secure.PublicRef("R", hash), PhoneHash: hash, PhoneEnc: enc, Kind: domain.KindResident,
		PreferredLanguage: l, PreferredChannel: pref, HasSmartphone: in.HasSmartphone, CreatedAt: now}
	if in.Channel.NonSmartphone() && in.HasSmartphone == nil {
		r.HasSmartphone = nil // unknown; a feature-phone call does not prove the absence of a smartphone
	}
	if in.Consent != nil {
		if *in.Consent {
			r.ConsentAt = &now
		} else {
			r.ConsentDeclined = true
		}
	}
	got, created, err := tx.CreateReporter(ctx, r)
	if err != nil {
		return got, false, err
	}
	if !created {
		changed := false
		if len(got.PhoneEnc) == 0 && !got.ConsentDeclined {
			got.PhoneEnc, changed = enc, true // number was purged earlier; keep it again pending consent
		}
		if in.Consent != nil && *in.Consent && got.ConsentAt == nil {
			got.ConsentAt, got.ConsentDeclined, changed = &now, false, true
		}
		if in.Channel == domain.ChannelWhatsApp && got.PreferredChannel != "whatsapp" {
			got.PreferredChannel, changed = "whatsapp", true
		}
		if changed {
			if err := tx.UpdateReporter(ctx, got); err != nil {
				return got, false, err
			}
		}
	}
	return got, created, nil
}

func waterQuality(it domain.IssueType) bool {
	return it == domain.IssueDirtyWater || it == domain.IssueFoulSmell || it == domain.IssueSewageMixing
}

// findDuplicate looks for the same household's earlier report of the same
// problem within dedupeWindow of at (the complaint's own time, so migrated
// manual-log entries are de-duplicated against each other too).
func (e *Engine) findDuplicate(ctx context.Context, tx *store.Store, reporterID int64, c domain.Complaint, at time.Time) (domain.Complaint, bool, error) {
	recent, err := tx.RecentByReporter(ctx, reporterID, at.Add(-dedupeWindow))
	if err != nil {
		return domain.Complaint{}, false, err
	}
	for _, r := range recent {
		if r.Status.Closed() || r.CreatedAt.After(at) {
			continue
		}
		same := r.IssueType == c.IssueType || (waterQuality(r.IssueType) && waterQuality(c.IssueType))
		if same && (r.WardID == c.WardID || r.WardID == "" || c.WardID == "") {
			return r, true, nil
		}
	}
	return domain.Complaint{}, false, nil
}

func (e *Engine) afterIntakeMessages(ctx context.Context, tx *store.Store, rep domain.Reporter, c domain.Complaint, loc locate.Result, wards []domain.Ward) error {
	l := lang.Pick(rep.PreferredLanguage, "hi")
	partner := e.Cfg.PartnerName
	for _, w := range wards {
		if w.ID == c.WardID {
			partner = e.partnerName(w)
		}
	}
	if _, err := e.messageToReporter(ctx, tx, rep, "ack", i18n.T("complaint_received", l, i18n.Vars{"ref": c.Ref, "issue": issueLabel(c.IssueType, l)}), true, "complaint", c.ID, nil); err != nil {
		return err
	}
	needConsent := rep.ConsentAt == nil && !rep.ConsentDeclined && rep.PreferredChannel == "whatsapp"
	switch {
	case needConsent:
		now := e.now()
		rep.ConsentAskedAt = &now
		if err := tx.UpdateReporter(ctx, rep); err != nil {
			return err
		}
		if _, err := e.messageToReporter(ctx, tx, rep, "consent", i18n.T("consent_notice", l, i18n.Vars{"partner": partner}), true, "reporter", rep.ID, nil); err != nil {
			return err
		}
		return tx.SetConversation(ctx, store.Conversation{ReporterID: rep.ID, State: "await_consent", ComplaintID: &c.ID, ExpiresAt: now.Add(e.Cfg.ConsentWindow)}, now)
	case loc.NeedsFollowup() && c.DuplicateOf == nil:
		return e.askLocation(ctx, tx, rep, c)
	}
	return nil
}

func (e *Engine) askLocation(ctx context.Context, tx *store.Store, rep domain.Reporter, c domain.Complaint) error {
	if rep.PreferredChannel != "whatsapp" {
		return nil // IVR/volunteer complaints: the volunteer follows up (dashboard queue)
	}
	l := lang.Pick(rep.PreferredLanguage, "hi")
	now := e.now()
	if _, err := e.messageToReporter(ctx, tx, rep, "followup", i18n.T("ask_location", l, i18n.Vars{"ref": c.Ref}), true, "complaint", c.ID, nil); err != nil {
		return err
	}
	e.event(ctx, tx, "intake-agent", "complaint.followup_asked", "complaint", c.Ref, c.WardID, nil)
	return tx.SetConversation(ctx, store.Conversation{ReporterID: rep.ID, State: "await_location", ComplaintID: &c.ID, ExpiresAt: now.Add(e.Cfg.FollowupWindow)}, now)
}

// residentIllness turns "my children have diarrhoea" in a complaint into an
// aggregate illness signal (one case per household per 72 h; no names).
func (e *Engine) residentIllness(ctx context.Context, tx *store.Store, c domain.Complaint, ex extract.Result) error {
	if c.ReporterID != nil {
		recent, err := tx.ListIllness(ctx, c.WardID, c.CreatedAt.Add(-72*time.Hour), 200)
		if err != nil {
			return err
		}
		for _, r := range recent {
			if r.ReporterID != nil && *r.ReporterID == *c.ReporterID && r.ReporterType == domain.KindResident {
				return nil
			}
		}
	}
	d, v := 0, 0
	if ex.Diarrhoea || !ex.Vomiting {
		d = 1
	}
	if ex.Vomiting {
		v = 1
	}
	ir := domain.IllnessReport{ReporterType: domain.KindResident, ReporterID: c.ReporterID, WardID: c.WardID, Point: c.Point, ReportedAt: c.CreatedAt,
		CasesDiarrhoea: d, CasesVomiting: v, Notes: "derived from resident complaint " + c.Ref, ComplaintID: &c.ID, CreatedAt: e.now()}
	if err := tx.CreateIllness(ctx, &ir); err != nil {
		return err
	}
	e.event(ctx, tx, "intake-agent", "illness.created", "illness_report", ir.Ref, c.WardID, map[string]any{"source": "resident_complaint", "complaint": c.Ref})
	return nil
}
