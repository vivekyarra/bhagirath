package web

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/asr"
	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/extract"
	"github.com/vivekyarra/bhagirath/internal/locate"
	"github.com/vivekyarra/bhagirath/internal/metrics"
	"github.com/vivekyarra/bhagirath/internal/rule"
	"github.com/vivekyarra/bhagirath/internal/store"
)

type contextT = context.Context

func jsonUnmarshal(b []byte, v any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}

func actor(r *http.Request) string { return "user:" + currentSession(r.Context()).User }

func (s *Server) wardMap(ctx context.Context) map[string]domain.Ward {
	ws, _ := s.Engine.Store.ListWards(ctx)
	m := map[string]domain.Ward{}
	for _, w := range ws {
		m[w.ID] = w
	}
	return m
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st := s.Engine.Store
	now := s.Engine.Clock.Now()
	m, _ := st.ComputeMetrics(ctx, !s.Cfg.Demo(), now)
	alerts, _ := st.ListAlerts(ctx, store.AlertFilter{Status: "active"})
	pending, _ := st.ListEscalations(ctx, store.EscalationFilter{Outcomes: []domain.EscalationOutcome{domain.OutcomePending, domain.OutcomeAcknowledged}, Limit: 12})
	review, _ := st.CountComplaints(ctx, store.ComplaintFilter{TranscriptStatus: domain.TranscriptNeedsReview})
	asrN, _ := st.CountComplaints(ctx, store.ComplaintFilter{TranscriptStatus: domain.TranscriptASR})
	unloc := true
	unlocated, _ := st.CountComplaints(ctx, store.ComplaintFilter{NeedsFollowup: &unloc, Statuses: []domain.ComplaintStatus{domain.StatusOpen, domain.StatusEscalated, domain.StatusReopened}})
	closures, _ := st.ListClosures(ctx, true, 50)
	events, _ := st.ListEvents(ctx, store.EventFilter{Limit: 25})
	overdue := 0
	for _, e := range pending {
		if e.Outcome == domain.OutcomePending && e.DeadlineAt.Before(now) {
			overdue++
		}
	}
	s.render(w, r, "overview", map[string]any{"Title": "Overview", "M": m, "Alerts": alerts, "Pending": pending, "Overdue": overdue,
		"Review": review + asrN, "Unlocated": unlocated, "Closures": closures, "Events": events, "Wards": s.wardMap(ctx),
		"Officials": s.officialNames(ctx)})
}

func (s *Server) officialNames(ctx context.Context) map[int64]domain.Official {
	os, _ := s.Engine.Store.ListOfficials(ctx, "")
	m := map[int64]domain.Official{}
	for _, o := range os {
		m[o.ID] = o
	}
	return m
}

func pathID(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }

func (s *Server) alertPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := s.Engine.Store.GetAlert(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var res rule.Result
	_ = jsonUnmarshal(a.Signals, &res)
	trans, _ := s.Engine.Store.Transitions(ctx, a.ID)
	escs, _ := s.Engine.Store.ListEscalations(ctx, store.EscalationFilter{AlertID: a.ID})
	advs, _ := s.Engine.Store.ListAdvisories(ctx, a.ID)
	wd, _ := s.Engine.Store.GetWard(ctx, a.WardID)
	delivered := 0
	for _, x := range advs {
		if x.Delivered {
			delivered++
		}
	}
	s.render(w, r, "alert", map[string]any{"Title": a.Level.Title() + " — " + wd.Name, "A": a, "Res": res, "Transitions": trans, "Escalations": escs,
		"Advisories": advs, "Delivered": delivered, "Ward": wd, "Officials": s.officialNames(ctx), "SignoffRequired": s.Cfg.RequirePartnerSignoff,
		"Reviewed": s.Cfg.AdvisoryReviewedBy})
}

func (s *Server) alertApprove(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	n, err := s.Engine.ApproveAdvisories(r.Context(), id, actor(r))
	if err != nil {
		redirectOK(w, r, fmt.Sprintf("/app/alerts/%d", id), "Not sent: "+err.Error())
		return
	}
	s.kickDispatcher()
	redirectOK(w, r, fmt.Sprintf("/app/alerts/%d", id), fmt.Sprintf("Approved. %d household messages queued.", n))
}

func (s *Server) complaintsPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ComplaintFilter{WardID: q.Get("ward"), Channel: domain.Channel(q.Get("channel")), Search: strings.TrimSpace(q.Get("q")), Limit: 300,
		IncludeDuplicates: q.Get("dups") == "1"}
	if st := q.Get("status"); st != "" {
		f.Statuses = []domain.ComplaintStatus{domain.ComplaintStatus(st)}
	}
	if q.Get("followup") == "1" {
		t := true
		f.NeedsFollowup = &t
	}
	cs, err := s.Engine.Store.ListComplaints(r.Context(), f)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "complaints", map[string]any{"Title": "Complaints", "Complaints": cs, "Q": q, "Wards": s.wardMap(r.Context()),
		"Statuses": domain.ComplaintStatuses, "Channels": domain.Channels})
}

func (s *Server) complaintPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := s.Engine.Store.GetComplaintByRef(ctx, r.PathValue("ref"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	escs, _ := s.Engine.Store.ListEscalations(ctx, store.EscalationFilter{ComplaintID: c.ID})
	cls, _ := s.Engine.Store.ClosuresFor(ctx, c.ID)
	evs, _ := s.Engine.Store.ListEvents(ctx, store.EventFilter{Entity: "complaint", EntityID: c.Ref, Limit: 100})
	msgs, _ := s.Engine.Store.ListMessages(ctx, store.MessageFilter{Entity: "complaint", EntityID: c.ID, Limit: 50})
	var ex struct {
		Extract  extract.Result `json:"extract"`
		Location locate.Result  `json:"location"`
	}
	_ = jsonUnmarshal(c.Extraction, &ex)
	var rep *domain.Reporter
	if c.ReporterID != nil {
		if rp, err := s.Engine.Store.GetReporter(ctx, *c.ReporterID); err == nil {
			rep = &rp
		}
	}
	var pending *domain.Closure
	for i := range cls {
		if cls[i].DecidedAt == nil {
			pending = &cls[i]
		}
	}
	var rates *asr.ErrorRates
	if c.TranscriptASR != "" && c.TranscriptStatus == domain.TranscriptReviewed {
		e := asr.Compare(c.Transcript, c.TranscriptASR)
		rates = &e
	}
	s.render(w, r, "complaint", map[string]any{"Title": "Complaint " + c.Ref, "C": c, "Escalations": escs, "Closures": cls, "Pending": pending,
		"Events": evs, "Messages": msgs, "Ex": ex, "Reporter": rep, "Wards": s.wardMap(ctx), "Officials": s.officialNames(ctx),
		"IssueTypes": domain.IssueTypes, "Rates": rates, "TestSources": domain.TestSources})
}

// complaintUpdate lets a volunteer/partner correct classification or location,
// or mark a complaint invalid (spam). Every change is audited.
func (s *Server) complaintUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ref := r.PathValue("ref")
	var escalate bool
	var wardID string
	err := s.Engine.Store.Tx(ctx, func(tx *store.Store) error {
		c, err := tx.GetComplaintByRef(ctx, ref)
		if err != nil {
			return err
		}
		c, err = tx.LockComplaint(ctx, c.ID)
		if err != nil {
			return err
		}
		changes := map[string]any{}
		wasEligible := c.WardID != "" && c.IssueType != domain.IssueOther
		if it := domain.IssueType(r.FormValue("issue_type")); it != "" && it != c.IssueType && domain.Contains(domain.IssueTypes, it) {
			changes["issue_type"] = []any{c.IssueType, it}
			c.IssueType = it
		}
		if wid := r.FormValue("ward_id"); wid != "" && wid != c.WardID {
			if _, err := tx.GetWard(ctx, wid); err != nil {
				return errors.New("unknown ward")
			}
			changes["ward"] = []any{c.WardID, wid}
			if c.WardID != "" {
				s.event(ctx, tx, r, "complaint.ward_corrected", c, map[string]any{"from": c.WardID, "to": wid})
			}
			c.WardID = wid
			if c.LocationMethod == domain.LocUnresolved {
				c.LocationMethod = domain.LocManual
			}
		}
		if lat, e1 := strconv.ParseFloat(r.FormValue("lat"), 64); e1 == nil {
			if lon, e2 := strconv.ParseFloat(r.FormValue("lon"), 64); e2 == nil {
				p := domain.Point{Lat: lat, Lon: lon}
				if p.Valid() {
					c.Point, c.LocationMethod = &p, domain.LocManual
					changes["point"] = p
				}
			}
		}
		if c.WardID != "" && c.Point != nil {
			c.NeedsFollowup = false
		}
		if r.FormValue("invalid") == "1" && c.Status != domain.StatusInvalid {
			changes["status"] = []any{c.Status, domain.StatusInvalid}
			c.Status = domain.StatusInvalid
		}
		if len(changes) == 0 {
			return nil
		}
		if err := tx.UpdateComplaint(ctx, &c, s.Engine.Clock.Now()); err != nil {
			return err
		}
		s.event(ctx, tx, r, "complaint.updated", c, changes)
		nowEligible := c.WardID != "" && c.IssueType != domain.IssueOther && c.Status != domain.StatusInvalid && c.DuplicateOf == nil
		if !wasEligible && nowEligible {
			escalate = true
		}
		wardID = c.WardID
		if escalate {
			return s.Engine.EscalateComplaint(ctx, tx, &c, domain.Ladder1, "complaint", nil)
		}
		return nil
	})
	if err != nil {
		redirectOK(w, r, "/app/complaints/"+ref, "Not saved: "+err.Error())
		return
	}
	if wardID != "" {
		_, _ = s.Engine.EvaluateWard(ctx, wardID)
	}
	s.kickDispatcher()
	redirectOK(w, r, "/app/complaints/"+ref, "Saved.")
}

func (s *Server) event(ctx context.Context, tx *store.Store, r *http.Request, action string, c domain.Complaint, details any) {
	raw, _ := json.Marshal(details)
	_ = tx.LogEvent(ctx, domain.Event{At: s.Engine.Clock.Now(), Actor: actor(r), Action: action, Entity: "complaint", EntityID: c.Ref, WardID: c.WardID, Details: raw})
}

func (s *Server) complaintClaim(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ref := r.PathValue("ref")
	c, err := s.Engine.Store.GetComplaintByRef(ctx, ref)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	photo, err := s.savePhoto(r, "photo")
	if err != nil {
		redirectOK(w, r, "/app/complaints/"+ref, "Not saved: "+err.Error())
		return
	}
	retest, err := s.retestFromForm(r, c.WardID, actor(r))
	if err != nil {
		redirectOK(w, r, "/app/complaints/"+ref, "Not saved: "+err.Error())
		return
	}
	by := strings.TrimSpace(r.FormValue("claimed_by"))
	if by == "" {
		by = actor(r)
	} else {
		by = by + " (recorded by " + actor(r) + ")"
	}
	if _, err := s.Engine.ClaimFixed(ctx, agents.Claim{ComplaintID: c.ID, ClaimedBy: by, PhotoMediaID: photo, RetestID: retest, Note: r.FormValue("note")}); err != nil {
		redirectOK(w, r, "/app/complaints/"+ref, "Not saved: "+err.Error())
		return
	}
	s.kickDispatcher()
	redirectOK(w, r, "/app/complaints/"+ref, "Fix claim recorded; the resident will be asked to confirm.")
}

func (s *Server) closureVerify(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cl, err := s.Engine.Store.GetClosure(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	c, _ := s.Engine.Store.GetComplaint(r.Context(), cl.ComplaintID)
	err = s.Engine.RecordCallback(r.Context(), id, r.FormValue("result") == "confirmed", "field_visit", actor(r))
	if err != nil {
		redirectOK(w, r, "/app/complaints/"+c.Ref, "Not saved: "+err.Error())
		return
	}
	s.kickDispatcher()
	redirectOK(w, r, "/app/complaints/"+c.Ref, "Field verification recorded.")
}

func (s *Server) escalationsPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.EscalationFilter{WardID: q.Get("ward"), Limit: 500}
	switch q.Get("show") {
	case "", "open":
		f.Outcomes = []domain.EscalationOutcome{domain.OutcomePending, domain.OutcomeAcknowledged}
	case "timed_out":
		f.Outcomes = []domain.EscalationOutcome{domain.OutcomeTimedOut}
	}
	es, _ := s.Engine.Store.ListEscalations(r.Context(), f)
	s.render(w, r, "escalations", map[string]any{"Title": "Escalations", "Escalations": es, "Q": q, "Wards": s.wardMap(r.Context()), "Officials": s.officialNames(r.Context())})
}

func (s *Server) escalationPage(w http.ResponseWriter, r *http.Request) {
	esc, err := s.Engine.Store.GetEscalationByRef(r.Context(), r.PathValue("ref"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	pack, err := s.evidencePack(r, esc)
	if err != nil {
		http.Error(w, "could not load", http.StatusInternalServerError)
		return
	}
	msgs, _ := s.Engine.Store.ListMessages(r.Context(), store.MessageFilter{Entity: "escalation", EntityID: esc.ID})
	pack["Messages"] = msgs
	pack["Title"] = "Escalation " + esc.Ref
	pack["Internal"] = true
	s.render(w, r, "escalation", pack)
}

func (s *Server) escalationAck(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	esc, err := s.Engine.Store.GetEscalationByRef(r.Context(), ref)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	via := r.FormValue("via")
	if via == "" {
		via = "partner"
	}
	err = s.Engine.Acknowledge(r.Context(), esc.ID, via, strings.TrimSpace(r.FormValue("grievance")), actor(r))
	if err != nil && !errors.Is(err, agents.ErrClosed) {
		redirectOK(w, r, "/app/escalations/"+ref, "Not saved: "+err.Error())
		return
	}
	redirectOK(w, r, "/app/escalations/"+ref, "Recorded.")
}

func (s *Server) reviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	need, _ := s.Engine.Store.ListComplaints(ctx, store.ComplaintFilter{TranscriptStatus: domain.TranscriptNeedsReview, Limit: 100})
	machine, _ := s.Engine.Store.ListComplaints(ctx, store.ComplaintFilter{TranscriptStatus: domain.TranscriptASR, Limit: 100})
	unloc := true
	unlocated, _ := s.Engine.Store.ListComplaints(ctx, store.ComplaintFilter{NeedsFollowup: &unloc, Statuses: []domain.ComplaintStatus{domain.StatusOpen, domain.StatusEscalated, domain.StatusReopened}, Limit: 100})
	n, wer := s.asrSummary(r)
	byLang := s.asrByLanguage(r)
	s.render(w, r, "review", map[string]any{"Title": "Review queue", "Need": need, "Machine": machine, "Unlocated": unlocated, "ASRCount": n, "WER": wer,
		"ByLang": byLang, "Wards": s.wardMap(ctx), "IssueTypes": domain.IssueTypes})
}

func (s *Server) asrSummary(r *http.Request) (int, *float64) {
	n, agg, _, err := metrics.ASR(r.Context(), s.Engine.Store, !s.Cfg.Demo())
	if err != nil || agg == nil {
		return n, nil
	}
	return n, &agg.WER
}

func (s *Server) asrByLanguage(r *http.Request) []metrics.LanguageRates {
	_, _, langs, _ := metrics.ASR(r.Context(), s.Engine.Store, !s.Cfg.Demo())
	return langs
}

// reviewSubmit saves a human transcript (or correction of the machine one),
// re-runs extraction and location, and escalates if it became eligible.
func (s *Server) reviewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ref := r.PathValue("ref")
	text := strings.TrimSpace(r.FormValue("transcript"))
	if text == "" {
		redirectOK(w, r, "/app/review", "Transcript cannot be empty.")
		return
	}
	ex, err := s.Engine.Extractor.Extract(ctx, text, domain.IssueType(r.FormValue("issue_type")))
	if err != nil {
		redirectOK(w, r, "/app/review", "Extraction failed.")
		return
	}
	wards, _ := s.Engine.Store.ListWards(ctx)
	var wardID string
	err = s.Engine.Store.Tx(ctx, func(tx *store.Store) error {
		c0, err := tx.GetComplaintByRef(ctx, ref)
		if err != nil {
			return err
		}
		c, err := tx.LockComplaint(ctx, c0.ID)
		if err != nil {
			return err
		}
		wasEligible := c.WardID != "" && c.IssueType != domain.IssueOther
		details := map[string]any{"had_asr": c.TranscriptASR != ""}
		if c.TranscriptASR != "" {
			rates := asr.Compare(text, c.TranscriptASR)
			details["wer"], details["cer"], details["ref_words"] = rates.WER, rates.CER, rates.RefWords
		}
		c.Transcript, c.TranscriptStatus, c.Language = text, domain.TranscriptReviewed, ex.Language
		if it := domain.IssueType(r.FormValue("issue_type")); it != "" && domain.Contains(domain.IssueTypes, it) {
			ex.IssueType = it
		}
		c.IssueType, c.Severity = ex.IssueType, max(c.Severity, ex.Severity)
		if c.WardID == "" || c.Point == nil {
			loc := locate.Resolve(locate.Input{Point: c.Point, Text: c.LocationText + " " + text + " " + ex.Landmark, WardHint: c.WardID}, wards)
			if loc.Resolved() {
				c.WardID, c.Point, c.LocationMethod, c.NeedsFollowup = loc.WardID, loc.Point, loc.Method, loc.Point == nil
				if loc.Landmark != "" {
					c.LocationText = strings.TrimSpace(c.LocationText + " (landmark: " + loc.Landmark + ")")
				}
			}
		}
		c.DescriptionEN = extract.Describe(ex, "")
		if err := tx.UpdateComplaint(ctx, &c, s.Engine.Clock.Now()); err != nil {
			return err
		}
		s.event(ctx, tx, r, "transcript.reviewed", c, details)
		wardID = c.WardID
		if !wasEligible && c.WardID != "" && c.IssueType != domain.IssueOther && c.DuplicateOf == nil && c.Status != domain.StatusInvalid {
			return s.Engine.EscalateComplaint(ctx, tx, &c, domain.Ladder1, "complaint", nil)
		}
		return nil
	})
	if err != nil {
		redirectOK(w, r, "/app/review", "Not saved: "+err.Error())
		return
	}
	if wardID != "" {
		_, _ = s.Engine.EvaluateWard(ctx, wardID)
	}
	s.kickDispatcher()
	redirectOK(w, r, "/app/review", "Saved "+ref+".")
}

func (s *Server) volunteerForm(w http.ResponseWriter, r *http.Request) {
	wards, _ := s.Engine.Store.ListWards(r.Context())
	s.render(w, r, "volunteer", map[string]any{"Title": "Volunteer line", "Wards": wards, "Issues": formIssues})
}

const maxAudioUpload = 16 << 20

// volunteerSubmit logs a complaint taken on the volunteer phone line (the
// fallback when telephony KYC blocks a provider) — channel "volunteer".
func (s *Server) volunteerSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxAudioUpload + (1 << 20)); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "upload too large", http.StatusBadRequest)
		return
	}
	in := agents.Inbound{Channel: domain.ChannelVolunteer, Text: strings.TrimSpace(r.FormValue("text")), LocationText: strings.TrimSpace(r.FormValue("landmark")),
		WardHint: r.FormValue("ward"), IssueHint: domain.IssueType(r.FormValue("issue")), Language: r.FormValue("lang"), Actor: actor(r), Phone: strings.TrimSpace(r.FormValue("phone"))}
	if in.IssueHint != "" && !domain.Contains(domain.IssueTypes, in.IssueHint) {
		in.IssueHint = ""
	}
	if in.Phone != "" {
		consent := r.FormValue("consent") == "on"
		in.Consent = &consent
		switch r.FormValue("smartphone") {
		case "yes":
			t := true
			in.HasSmartphone = &t
		case "no":
			f := false
			in.HasSmartphone = &f
		}
	}
	if f, hdr, err := r.FormFile("audio"); err == nil {
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxAudioUpload+1))
		if err != nil || len(data) > maxAudioUpload {
			redirectOK(w, r, "/app/volunteer", "Recording too large (16 MB max).")
			return
		}
		in.Audio, in.AudioType = data, hdr.Header.Get("Content-Type")
		if !strings.HasPrefix(in.AudioType, "audio/") && !strings.HasPrefix(in.AudioType, "video/webm") {
			in.AudioType = http.DetectContentType(data)
		}
		in.Text = "" // the typed text becomes the reviewed transcript below
		if t := strings.TrimSpace(r.FormValue("text")); t != "" {
			in.Text = t
		}
	}
	res, err := s.Engine.Intake(r.Context(), in)
	if err != nil {
		redirectOK(w, r, "/app/volunteer", "Not saved: "+err.Error())
		return
	}
	s.kickDispatcher()
	redirectOK(w, r, "/app/complaints/"+res.Complaint.Ref, "Logged from the volunteer line.")
}

func (s *Server) signalsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tests, _ := s.Engine.Store.ListWaterTests(ctx, "", time.Time{}, 200)
	ill, _ := s.Engine.Store.ListIllness(ctx, "", time.Time{}, 200)
	reps, _ := s.Engine.Store.ListReporters(ctx, store.ReporterFilter{Kinds: []domain.ReporterKind{domain.KindClinic, domain.KindChemist, domain.KindASHA, domain.KindHospital}})
	wards, _ := s.Engine.Store.ListWards(ctx)
	s.render(w, r, "signals", map[string]any{"Title": "Water tests & illness", "Tests": tests, "Illness": ill, "Reporters": reps, "WardList": wards,
		"Wards": s.wardMap(ctx), "Sources": domain.TestSources, "Today": s.Engine.Clock.Now().In(clock.IST).Format("2006-01-02T15:04")})
}

func parseLocal(v string, now time.Time) time.Time {
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(v), clock.IST); err == nil {
			return t.UTC()
		}
	}
	return now
}

func optFloat(v string) *float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return nil
	}
	return &f
}

func (s *Server) addTest(w http.ResponseWriter, r *http.Request) {
	t := domain.WaterTest{Source: domain.TestSource(r.FormValue("source")), WardID: r.FormValue("ward"), SampledAt: parseLocal(r.FormValue("sampled_at"), s.Engine.Clock.Now()),
		Parameter: strings.TrimSpace(r.FormValue("parameter")), Value: optFloat(r.FormValue("value")), LimitValue: optFloat(r.FormValue("limit")),
		Unit: strings.TrimSpace(r.FormValue("unit")), IsContaminated: r.FormValue("contaminated") == "on", EvidenceURL: strings.TrimSpace(r.FormValue("evidence_url")),
		Note: strings.TrimSpace(r.FormValue("note"))}
	if lat, lon := optFloat(r.FormValue("lat")), optFloat(r.FormValue("lon")); lat != nil && lon != nil {
		t.Point = &domain.Point{Lat: *lat, Lon: *lon}
	}
	if _, err := s.Engine.AddWaterTest(r.Context(), t, actor(r)); err != nil {
		redirectOK(w, r, "/app/signals", "Not saved: "+err.Error())
		return
	}
	s.kickDispatcher()
	redirectOK(w, r, "/app/signals", "Water test saved; the ward was re-evaluated.")
}

// importTests ingests a CSV (e.g. exported from JJM-WQMIS or a lab sheet):
// ward_id,sampled_at,parameter,value,limit,unit,contaminated,source,evidence_url
func (s *Server) importTests(w http.ResponseWriter, r *http.Request) {
	f, _, err := r.FormFile("file")
	if err != nil {
		redirectOK(w, r, "/app/signals", "Choose a CSV file.")
		return
	}
	defer f.Close()
	cr := csv.NewReader(io.LimitReader(f, 4<<20))
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil || len(rows) < 2 {
		redirectOK(w, r, "/app/signals", "CSV could not be read (need a header row and at least one data row).")
		return
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, k string) string {
		if i, ok := col[k]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	for _, need := range []string{"ward_id", "sampled_at", "parameter"} {
		if _, ok := col[need]; !ok {
			redirectOK(w, r, "/app/signals", "CSV is missing column "+need)
			return
		}
	}
	ok, bad := 0, 0
	for _, row := range rows[1:] {
		src := domain.TestSource(get(row, "source"))
		if src == "" {
			src = domain.TestJJMPublic
		}
		cont := strings.ToLower(get(row, "contaminated"))
		t := domain.WaterTest{Source: src, WardID: get(row, "ward_id"), SampledAt: parseLocal(get(row, "sampled_at"), time.Time{}), Parameter: get(row, "parameter"),
			Value: optFloat(get(row, "value")), LimitValue: optFloat(get(row, "limit")), Unit: get(row, "unit"),
			IsContaminated: cont == "true" || cont == "yes" || cont == "1" || cont == "fail", EvidenceURL: get(row, "evidence_url"), Note: "CSV import"}
		if t.SampledAt.IsZero() {
			bad++
			continue
		}
		if _, err := s.Engine.AddWaterTest(r.Context(), t, actor(r)); err != nil {
			bad++
			continue
		}
		ok++
	}
	redirectOK(w, r, "/app/signals", fmt.Sprintf("Imported %d tests; %d rows skipped.", ok, bad))
}

func (s *Server) addIllness(w http.ResponseWriter, r *http.Request) {
	d, _ := strconv.Atoi(r.FormValue("diarrhoea"))
	v, _ := strconv.Atoi(r.FormValue("vomiting"))
	ir := domain.IllnessReport{ReporterType: domain.ReporterKind(r.FormValue("reporter_type")), WardID: r.FormValue("ward"),
		ReportedAt: parseLocal(r.FormValue("reported_at"), s.Engine.Clock.Now()), CasesDiarrhoea: d, CasesVomiting: v, Notes: strings.TrimSpace(r.FormValue("notes"))}
	if o := strings.TrimSpace(r.FormValue("ors")); o != "" {
		n, err := strconv.Atoi(o)
		if err == nil {
			ir.ORSUnitsSold = &n
		}
	}
	if ref := r.FormValue("reporter_ref"); ref != "" {
		if rp, err := s.Engine.Store.GetReporterByRef(r.Context(), ref); err == nil {
			ir.ReporterID, ir.ReporterType = &rp.ID, rp.Kind
		}
	}
	if _, err := s.Engine.AddIllness(r.Context(), ir, actor(r)); err != nil {
		redirectOK(w, r, "/app/signals", "Not saved: "+err.Error())
		return
	}
	s.kickDispatcher()
	redirectOK(w, r, "/app/signals", "Illness report saved; the ward was re-evaluated.")
}

func (s *Server) addSignalReporter(w http.ResponseWriter, r *http.Request) {
	_, err := s.Engine.RegisterSignalReporter(r.Context(), r.FormValue("phone"), domain.ReporterKind(r.FormValue("kind")), strings.TrimSpace(r.FormValue("label")),
		r.FormValue("ward"), r.FormValue("lang"), actor(r))
	if err != nil {
		redirectOK(w, r, "/app/signals", "Not saved: "+err.Error())
		return
	}
	redirectOK(w, r, "/app/signals", "Registered. Their WhatsApp messages like \"D 3 V 1 ORS 12\" now count as illness signals.")
}

func (s *Server) outboxPage(w http.ResponseWriter, r *http.Request) {
	msgs, _ := s.Engine.Store.ListMessages(r.Context(), store.MessageFilter{Kind: r.URL.Query().Get("kind"), Limit: 300})
	decided := map[int64]bool{}
	for _, m := range msgs {
		if m.Kind == "callback" {
			if cl, err := s.Engine.Store.GetClosure(r.Context(), m.EntityID); err == nil && cl.DecidedAt != nil {
				decided[m.EntityID] = true
			}
		}
	}
	s.render(w, r, "outbox", map[string]any{"Title": "Outbox", "Messages": msgs, "Decided": decided, "Kind": r.URL.Query().Get("kind")})
}

// outboxAnswer lets a demo user answer a simulated callback as the resident.
func (s *Server) outboxAnswer(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.Demo() {
		http.Error(w, "only available in demo mode", http.StatusForbidden)
		return
	}
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	m, err := s.Engine.Store.GetMessage(r.Context(), id)
	if err != nil || m.Kind != "callback" {
		http.NotFound(w, r)
		return
	}
	err = s.Engine.RecordCallback(r.Context(), m.EntityID, r.FormValue("answer") == "1", m.Channel, "simulator:resident")
	if err != nil {
		redirectOK(w, r, "/app/outbox?kind=callback", "Not recorded: "+err.Error())
		return
	}
	s.kickDispatcher()
	cl, _ := s.Engine.Store.GetClosure(r.Context(), m.EntityID)
	c, _ := s.Engine.Store.GetComplaint(r.Context(), cl.ComplaintID)
	redirectOK(w, r, "/app/complaints/"+c.Ref, map[bool]string{true: "Resident pressed 1: fix verified.", false: "Resident pressed 2: fake closure caught, complaint reopened and escalated."}[r.FormValue("answer") == "1"])
}

func (s *Server) pilotPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, ts, _ := s.publicMetrics(r)
	out, _ := s.Engine.Store.ListOutreach(ctx)
	base, _ := s.Engine.Store.ListBaselines(ctx)
	wards, _ := s.Engine.Store.ListWards(ctx)
	s.render(w, r, "pilot", map[string]any{"Title": "Pilot & metrics", "M": m, "Targets": ts, "Outreach": out, "Baselines": base, "WardList": wards,
		"Wards": s.wardMap(ctx), "Today": s.Engine.Clock.Now().In(clock.IST).Format("2006-01-02")})
}

func (s *Server) addOutreach(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.FormValue("households"))
	if err != nil || n < 0 || n > 100000 {
		redirectOK(w, r, "/app/pilot", "Households must be a number.")
		return
	}
	d := parseLocal(r.FormValue("date"), s.Engine.Clock.Now())
	err = s.Engine.Store.CreateOutreach(r.Context(), domain.Outreach{WardID: r.FormValue("ward"), Date: d, Households: n, Method: r.FormValue("method"),
		Note: strings.TrimSpace(r.FormValue("note")), LoggedBy: actor(r), CreatedAt: s.Engine.Clock.Now()})
	if err != nil {
		redirectOK(w, r, "/app/pilot", "Not saved: "+err.Error())
		return
	}
	redirectOK(w, r, "/app/pilot", "Outreach logged.")
}

func (s *Server) addBaseline(w http.ResponseWriter, r *http.Request) {
	d, err := strconv.ParseFloat(r.FormValue("days"), 64)
	if err != nil || d < 0 {
		redirectOK(w, r, "/app/pilot", "Days must be a number.")
		return
	}
	err = s.Engine.Store.CreateBaseline(r.Context(), domain.Baseline{WardID: r.FormValue("ward"), Source: r.FormValue("source"), DaysToFix: d,
		Resolved: r.FormValue("resolved") != "no", Note: strings.TrimSpace(r.FormValue("note")), CreatedAt: s.Engine.Clock.Now()})
	if err != nil {
		redirectOK(w, r, "/app/pilot", "Not saved: "+err.Error())
		return
	}
	redirectOK(w, r, "/app/pilot", "Baseline observation saved.")
}

func (s *Server) metricsMarkdown(w http.ResponseWriter, r *http.Request) {
	m, ts, err := s.publicMetrics(r)
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		return
	}
	dep := s.Cfg.BaseURL + " (" + string(s.Cfg.Mode) + " mode)"
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="METRICS.md"`)
	n, agg, langs, _ := metrics.ASR(r.Context(), s.Engine.Store, !s.Cfg.Demo())
	_, _ = io.WriteString(w, metrics.Markdown(m, ts, dep)+metrics.ASRMarkdown(n, agg, langs))
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wards, _ := s.Engine.Store.ListWards(ctx)
	offs, _ := s.Engine.Store.ListOfficials(ctx, "")
	var jobs map[string]time.Time
	if s.Scheduler != nil {
		jobs = s.Scheduler.LastRun()
	}
	s.render(w, r, "settings", map[string]any{"Title": "Settings", "Rule": s.Engine.Rule, "FrozenOK": s.Engine.Rule.CheckFrozen() == nil, "FrozenTag": rule.FrozenTag,
		"Providers": s.Cfg.Providers(), "WardList": wards, "Officials": offs, "Cfg": s.Cfg, "Jobs": jobs, "Ladder": s.Engine.Ladder})
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	m, err := s.Engine.Store.GetMedia(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if len(m.Data) == 0 {
		http.Error(w, "deleted under the data-retention policy", http.StatusGone)
		return
	}
	// Residents' voices are personal data: only people who review or act on
	// complaints (partner/admin; the judge account in demo) may play them.
	if m.Kind == "audio" && !s.can(currentSession(r.Context()), "operate") {
		http.Error(w, "your role cannot access recordings", http.StatusForbidden)
		return
	}
	ct, disp := m.ContentType, "inline"
	if !safeInline(ct) {
		ct, disp = "application/octet-stream", "attachment"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Header().Set("Content-Disposition", disp)
	_, _ = w.Write(m.Data)
}

func (s *Server) mapJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wards, _ := s.Engine.Store.ListWards(ctx)
	alerts, _ := s.Engine.Store.ListAlerts(ctx, store.AlertFilter{Status: "active"})
	level := map[string]string{}
	alertID := map[string]int64{}
	for _, a := range alerts {
		level[a.WardID], alertID[a.WardID] = a.Level.String(), a.ID
	}
	type wardOut struct {
		ID, Name, Level string
		AlertID         int64
		Polygon         [][]domain.Point
		Landmarks       []domain.Landmark
	}
	type pin struct {
		Ref, Issue, Status, Channel, Ward string
		Lat, Lon                          float64
		At                                time.Time
	}
	var ws []wardOut
	for _, wd := range wards {
		ws = append(ws, wardOut{ID: wd.ID, Name: wd.Name, Level: level[wd.ID], AlertID: alertID[wd.ID], Polygon: wd.Polygon, Landmarks: wd.Landmarks})
	}
	cs, _ := s.Engine.Store.ListComplaints(ctx, store.ComplaintFilter{Since: s.Engine.Clock.Now().Add(-60 * 24 * time.Hour), Limit: 5000})
	var pins []pin
	for _, c := range cs {
		if c.Point != nil {
			pins = append(pins, pin{Ref: c.Ref, Issue: extract.IssueLabel(c.IssueType), Status: string(c.Status), Channel: string(c.Channel), Ward: c.WardID, Lat: c.Point.Lat, Lon: c.Point.Lon, At: c.CreatedAt})
		}
	}
	writeJSON(w, map[string]any{"wards": ws, "complaints": pins})
}

func (s *Server) eventsJSON(w http.ResponseWriter, r *http.Request) {
	evs, _ := s.Engine.Store.ListEvents(r.Context(), store.EventFilter{Limit: 50})
	writeJSON(w, evs)
}

// safeInline reports whether a stored media type may be rendered in the
// browser; anything else (e.g. a mislabelled text/html upload) is downloaded.
func safeInline(ct string) bool {
	base := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	switch base {
	case "image/jpeg", "image/png", "image/webp", "audio/wav", "audio/x-wav", "audio/wave", "audio/ogg", "audio/mpeg", "audio/mp4", "audio/aac", "audio/webm", "audio/amr":
		return true
	}
	return false
}
