package agents_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/store"
)

func claimWithPhoto(h *harness, complaintID int64) domain.Closure {
	h.t.Helper()
	photo, _ := h.st.SaveMedia(h.ctx, domain.Media{SHA256: "p", ContentType: "image/jpeg", Data: []byte{1}, Kind: "photo", CreatedAt: h.clk.Now()})
	cl, err := h.e.ClaimFixed(h.ctx, agents.Claim{ComplaintID: complaintID, ClaimedBy: "AE", PhotoMediaID: &photo})
	if err != nil {
		h.t.Fatal(err)
	}
	return cl
}

func TestNoAnswerAfterThreeAttemptsReopens(t *testing.T) {
	h := newHarness(t)
	out := h.whatsapp("+910000000011", "शिव मंदिर के पास गंदा पानी", nil)
	h.whatsapp("+910000000011", "yes", nil)
	cl := claimWithPhoto(h, out.Complaint.ID)
	for i := 0; i < 6; i++ {
		h.clk.Advance(10 * time.Hour)
		if _, err := h.e.ProcessCallbacks(h.ctx); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := h.st.GetClosure(h.ctx, cl.ID)
	if got.CallbackAttempts != 3 || got.CallbackResult != domain.CallbackNoAnswer || got.Verified {
		t.Fatalf("closure = %+v", got)
	}
	c, _ := h.st.GetComplaint(h.ctx, out.Complaint.ID)
	if c.ReopenCount != 1 || c.Status == domain.StatusVerifiedFixed {
		t.Fatalf("complaint = %s reopen=%d", c.Status, c.ReopenCount)
	}
	msgs, _ := h.st.ListMessages(h.ctx, store.MessageFilter{Kind: "callback"})
	if len(msgs) != 3 {
		t.Fatalf("callback attempts sent = %d, want 3", len(msgs))
	}
}

func TestNoConsentNeedsFieldVisit(t *testing.T) {
	h := newHarness(t)
	out := h.whatsapp("+910000000012", "शिव मंदिर के पास गंदा पानी", nil)
	h.whatsapp("+910000000012", "no", nil) // declines consent: number is dropped
	rep, _ := h.st.GetReporterByRef(h.ctx, out.Complaint.ReporterRef)
	if len(rep.PhoneEnc) != 0 || !rep.ConsentDeclined {
		t.Fatal("declined reporter still has a stored number")
	}
	cl := claimWithPhoto(h, out.Complaint.ID)
	got, _ := h.st.GetClosure(h.ctx, cl.ID)
	if got.CallbackChannel != "field_visit_required" || got.NextCallbackAt != nil {
		t.Fatalf("closure = %+v", got)
	}
	if err := h.e.RecordCallback(h.ctx, cl.ID, true, "field_visit", "partner"); err != nil {
		t.Fatal(err)
	}
	c, _ := h.st.GetComplaint(h.ctx, out.Complaint.ID)
	if c.Status != domain.StatusVerifiedFixed {
		t.Fatalf("field verification did not verify: %s", c.Status)
	}
}

func TestFollowupLocationQuestion(t *testing.T) {
	h := newHarness(t)
	out := h.whatsapp("+910000000013", "गंदा पानी आ रहा है", nil) // no landmark, no pin
	if out.Complaint.WardID != "" || !out.Complaint.NeedsFollowup {
		t.Fatalf("expected unresolved: %+v", out.Complaint)
	}
	h.whatsapp("+910000000013", "हाँ", nil) // consent, then the location question follows
	qs, _ := h.st.ListMessages(h.ctx, store.MessageFilter{Kind: "followup"})
	if len(qs) != 1 {
		t.Fatalf("follow-up questions = %d", len(qs))
	}
	loc := h.whatsapp("+910000000013", "राशन की दुकान के पास", nil)
	if loc.Handled != "location" || loc.Complaint.WardID != ward || loc.Complaint.Status != domain.StatusEscalated {
		t.Fatalf("location reply: %+v", loc)
	}
}

func TestSameHouseholdRepeatIsDeduplicated(t *testing.T) {
	h := newHarness(t)
	h.whatsapp("+910000000014", "शिव मंदिर के पास गंदा पानी", nil)
	h.clk.Advance(time.Hour)
	out := h.whatsapp("+910000000014", "फिर से बदबूदार पानी, शिव मंदिर", nil)
	if out.Complaint.DuplicateOf == nil {
		t.Fatal("repeat complaint not linked as duplicate")
	}
	n, _ := h.st.CountComplaints(h.ctx, store.ComplaintFilter{})
	if n != 1 {
		t.Fatalf("unique complaints = %d", n)
	}
}

func TestAlertResolutionClosesEscalations(t *testing.T) {
	h := newHarness(t)
	v, l := 3.0, 0.0
	tst, err := h.e.AddWaterTest(h.ctx, domain.WaterTest{Source: domain.TestLabReport, WardID: ward, SampledAt: h.clk.Now(), Parameter: "E. coli", Value: &v, LimitValue: &l}, "p")
	if err != nil || h.alert().Level != domain.LevelWatch {
		t.Fatalf("watch not raised: %v", err)
	}
	_ = tst
	p := 0.0
	h.clk.Advance(time.Hour)
	if _, err := h.e.AddWaterTest(h.ctx, domain.WaterTest{Source: domain.TestLabReport, WardID: ward, SampledAt: h.clk.Now(), Parameter: "e. coli", Value: &p, LimitValue: &l}, "p"); err != nil {
		t.Fatal(err)
	}
	if h.alert().ID != 0 {
		t.Fatal("alert not resolved after passing retest")
	}
	for _, x := range h.escalations(store.EscalationFilter{WardID: ward}) {
		if x.Outcome == domain.OutcomePending {
			t.Fatalf("escalation %s pending after resolution", x.Ref)
		}
	}
	if h.count("alert.resolved") != 1 {
		t.Fatal("resolution not logged")
	}
}

func TestAckWithoutFixEscalates(t *testing.T) {
	h := newHarness(t)
	out := h.whatsapp("+910000000015", "शिव मंदिर के पास गंदा पानी", nil)
	es := h.escalations(store.EscalationFilter{ComplaintID: out.Complaint.ID})
	if err := h.e.Acknowledge(h.ctx, es[0].ID, "link", "", "AE"); err != nil {
		t.Fatal(err)
	}
	h.clk.Advance(73 * time.Hour)
	if _, err := h.e.ProcessTimeouts(h.ctx); err != nil {
		t.Fatal(err)
	}
	var up bool
	for _, x := range h.escalations(store.EscalationFilter{ComplaintID: out.Complaint.ID}) {
		up = up || (x.Level == domain.Ladder2 && x.Reason == "ack_no_fix")
	}
	if !up {
		t.Fatal("acknowledged-but-unfixed complaint was not escalated")
	}
}

func TestAdvisoryBlockedUntilReviewedInPilot(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Mode = config.ModePilot; c.AdvisoryReviewedBy = "" })
	v, l := 3.0, 0.0
	_, _ = h.e.AddWaterTest(h.ctx, domain.WaterTest{Source: domain.TestLabReport, WardID: ward, SampledAt: h.clk.Now(), Parameter: "E. coli", Value: &v, LimitValue: &l}, "p")
	_, _ = h.e.AddIllness(h.ctx, domain.IllnessReport{ReporterType: domain.KindClinic, WardID: ward, CasesDiarrhoea: 3}, "p")
	a := h.alert()
	if a.Level != domain.LevelWarning {
		t.Fatalf("level %v", a.Level)
	}
	if _, err := h.e.ApproveAdvisories(h.ctx, a.ID, "partner"); err != agents.ErrAdvisoryNotReviewed {
		t.Fatalf("unreviewed advisory not blocked: %v", err)
	}
}

func TestParseIllness(t *testing.T) {
	cases := map[string][3]int{
		"D 3 V 1 ORS 12":           {3, 1, 12},
		"दस्त 4 उल्टी 2":           {4, 2, -1},
		"diarrhoea: 2, vomiting 1": {2, 1, -1},
		"ors=40":                   {0, 0, 40},
	}
	for in, want := range cases {
		got, ok := agents.ParseIllness(in)
		ors := -1
		if got.ORS != nil {
			ors = *got.ORS
		}
		if !ok || got.Diarrhoea != want[0] || got.Vomiting != want[1] || ors != want[2] {
			t.Errorf("%q => %+v ok=%v", in, got, ok)
		}
	}
	if _, ok := agents.ParseIllness("hello"); ok {
		t.Error("free text parsed as counts")
	}
}

// TestConcurrentIntakeStress fires many complaints at one ward in parallel:
// exactly one active alert must result, with no deadlocks or lost writes.
func TestConcurrentIntakeStress(t *testing.T) {
	h := newHarness(t)
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := h.e.Intake(h.ctx, agents.Inbound{Channel: domain.ChannelWeb, Phone: fmt.Sprintf("+9100000010%02d", i),
				Text: "dirty smelly water", Point: near(float64(i%7)*20, float64(i%5)*20), Consent: ptrBool(true), ProviderMsgID: fmt.Sprint("web-", i)})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !strings.Contains(err.Error(), "could not serialize") {
			t.Fatal(err)
		}
	}
	as, _ := h.st.ListAlerts(h.ctx, store.AlertFilter{WardID: ward, Status: "active"})
	if len(as) != 1 || as[0].Level != domain.LevelWatch {
		t.Fatalf("active alerts = %d", len(as))
	}
	cnt, _ := h.st.CountComplaints(h.ctx, store.ComplaintFilter{})
	if cnt != n {
		t.Fatalf("complaints = %d, want %d", cnt, n)
	}
}

func ptrBool(b bool) *bool { return &b }

func TestLocationPinAttachesInsteadOfCreatingComplaint(t *testing.T) {
	h := newHarness(t)
	// Pin first, then the complaint text: one complaint, located by GPS.
	out, err := h.e.HandleMessage(h.ctx, agents.Message{Phone: "+910000000021", Point: near(10, 10), LocationText: "घर"})
	if err != nil || out.Handled != "location" {
		t.Fatalf("pin: %+v %v", out, err)
	}
	c := h.whatsapp("+910000000021", "नल से गंदा पानी आ रहा है", nil)
	if c.Complaint.LocationMethod != domain.LocGPS || c.Complaint.WardID != ward {
		t.Fatalf("kept pin not used: %+v", c.Complaint)
	}
	// Complaint first (no location), then the pin: the complaint gets located.
	c2 := h.whatsapp("+910000000022", "बदबूदार पानी आ रहा है", nil)
	if c2.Complaint.WardID != "" {
		t.Fatal("expected unlocated complaint")
	}
	out, err = h.e.HandleMessage(h.ctx, agents.Message{Phone: "+910000000022", Point: near(-20, 30)})
	if err != nil || out.Handled != "location" || out.Complaint == nil || out.Complaint.WardID != ward {
		t.Fatalf("late pin: %+v %v", out, err)
	}
	n, _ := h.st.CountComplaints(h.ctx, store.ComplaintFilter{})
	if n != 2 {
		t.Fatalf("pins created complaints: %d", n)
	}
}

func FuzzParseIllness(f *testing.F) {
	for _, s := range []string{"D 3 V 1 ORS 12", "दस्त 4 उल्टी 2", "ors=99999", "d -1", "D 99999999999"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, ok := agents.ParseIllness(s)
		if !ok {
			return
		}
		if c.Diarrhoea < 0 || c.Vomiting < 0 || c.Diarrhoea > 99999 || c.Vomiting > 99999 || (c.ORS != nil && (*c.ORS < 0 || *c.ORS > 99999)) {
			t.Fatalf("out-of-range counts from %q: %+v", s, c)
		}
	})
}

func TestOfficialAcknowledgesOnWhatsApp(t *testing.T) {
	h := newHarness(t)
	const offPhone = "+910000000501"
	enc, _ := h.e.Vault.Encrypt(offPhone)
	wa := domain.Official{Name: "WA AE", Role: "Assistant Engineer", WardIDs: []string{ward}, PhoneEnc: enc, PhoneHash: h.e.Vault.HashPhone(offPhone),
		LadderLevel: domain.Ladder1, PreferredChannel: "whatsapp", Active: true}
	if err := h.st.CreateOfficial(h.ctx, &wa); err != nil {
		t.Fatal(err)
	}
	out := h.whatsapp("+910000000031", "शिव मंदिर के पास गंदा पानी", nil)
	var mine, other domain.Escalation
	for _, x := range h.escalations(store.EscalationFilter{ComplaintID: out.Complaint.ID}) {
		if x.OfficialID == wa.ID {
			mine = x
		} else {
			other = x
		}
	}
	if mine.ID == 0 || other.ID == 0 || mine.Channel != "whatsapp" {
		t.Fatalf("expected escalations to both rung-1 officials: mine=%+v other=%+v", mine, other)
	}
	msgs, _ := h.st.ListMessages(h.ctx, store.MessageFilter{Entity: "escalation", EntityID: mine.ID})
	if len(msgs) != 1 || !strings.Contains(msgs[0].Body, "ACK "+mine.Ref) || !strings.Contains(string(msgs[0].Interactive), `"ack"`) {
		t.Fatalf("WhatsApp escalation lacks ACK instructions/button: %+v", msgs)
	}
	// Cannot acknowledge someone else's escalation.
	res, err := h.e.HandleMessage(h.ctx, agents.Message{Phone: offPhone, Text: "ACK " + other.Ref})
	if err != nil || res.Handled == "official_ack" {
		t.Fatalf("acknowledged another official's escalation: %+v %v", res, err)
	}
	if x, _ := h.st.GetEscalation(h.ctx, other.ID); x.AcknowledgedAt != nil {
		t.Fatal("other escalation acknowledged")
	}
	// Free text with the reference works, in any case.
	res, err = h.e.HandleMessage(h.ctx, agents.Message{Phone: offPhone, Text: "ok received " + strings.ToLower(mine.Ref)})
	if err != nil || res.Handled != "official_ack" {
		t.Fatalf("text ack: %+v %v", res, err)
	}
	if x, _ := h.st.GetEscalation(h.ctx, mine.ID); x.AcknowledgedAt == nil || x.AckVia != "whatsapp" {
		t.Fatalf("not acknowledged: %+v", x)
	}
	// Officials are never mistaken for residents filing complaints.
	n0, _ := h.st.CountComplaints(h.ctx, store.ComplaintFilter{})
	if _, err := h.e.HandleMessage(h.ctx, agents.Message{Phone: offPhone, Text: "गंदा पानी"}); err != nil {
		t.Fatal(err)
	}
	if n1, _ := h.st.CountComplaints(h.ctx, store.ComplaintFilter{}); n1 != n0 {
		t.Fatal("an official's message became a complaint")
	}
	// Button reply path.
	out2 := h.whatsapp("+910000000032", "राशन की दुकान के पास बदबूदार पानी", nil)
	for _, x := range h.escalations(store.EscalationFilter{ComplaintID: out2.Complaint.ID}) {
		if x.OfficialID == wa.ID {
			if r, err := h.e.HandleMessage(h.ctx, agents.Message{Phone: offPhone, ReplyID: "ack:" + itoa(x.ID)}); err != nil || r.Handled != "official_ack" {
				t.Fatalf("button ack: %+v %v", r, err)
			}
		}
	}
	if _, err := h.d.Run(h.ctx); err != nil {
		t.Fatal(err)
	}
}
