// Package demo seeds a deployment with FICTIONAL data so judges can explore a
// realistic history and then run the guided story themselves. All wards are
// marked demo, all phone numbers are +9100000xxxxx (not real), and pilot
// metric exports exclude demo wards.
package demo

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/asr"
	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/notify"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// Wards used by the seed and the guided story.
const (
	Urban   = "demo-urban-12"
	Village = "demo-village-3"
)

// Phone returns a clearly fake demo number.
func Phone(n int) string { return fmt.Sprintf("+9100000%05d", n) }

// scriptedASR stands in for a speech provider in the demo: it "recognises"
// pre-recorded fake clips with realistic errors so the review queue and the
// WER measurement can be demonstrated without real resident audio.
type scriptedASR map[int]string

func (s scriptedASR) Transcribe(_ context.Context, audio []byte, _, _ string) (asr.Transcript, error) {
	if t, ok := s[len(audio)]; ok {
		return asr.Transcript{Text: t, Language: "hi", Provider: "demo-scripted"}, nil
	}
	return asr.Transcript{}, asr.ErrUnavailable
}
func (scriptedASR) Name() string { return "demo-scripted" }

// SilentWAV returns a valid mono 8 kHz WAV of the given length (a stand-in
// for a recording; no real voice is ever stored in the repository).
func SilentWAV(ms int) []byte {
	n := 8 * ms
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + n))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(8000))
	w(uint32(8000))
	w(uint16(1))
	w(uint16(8))
	b.WriteString("data")
	w(uint32(n))
	b.Write(bytes.Repeat([]byte{128}, n))
	return b.Bytes()
}

// RepairPhoto returns a small PNG standing in for a repair photo.
func RepairPhoto() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(40 + x), G: uint8(90 + y), B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func near(base domain.Point, dyM, dxM float64) *domain.Point {
	return &domain.Point{Lat: base.Lat + dyM/111_195, Lon: base.Lon + dxM/102_550}
}

// Seed wipes the database and builds about ten days of fictional history.
func Seed(ctx context.Context, live *agents.Engine, d *notify.Dispatcher) error {
	st := live.Store
	if err := st.ResetAll(ctx); err != nil {
		return err
	}
	if err := live.SyncLadder(ctx, live.Ladder); err != nil {
		return err
	}
	now := live.Clock.Now()
	clk := clock.NewFixed(now.Add(-10 * 24 * time.Hour).Truncate(time.Hour))
	clipA, clipB := SilentWAV(2100), SilentWAV(3400)
	e := *live
	e.Clock = clk
	e.ASR = scriptedASR{
		len(clipA): "पंचायत भवन के पास हैंडपंप का पानी पीला और गंदा आ रहा है बच्चों को दस्त",
		len(clipB): "गटर का पानी नल में मिल रहा है आंगनवाडी के पास बहुत बदबू",
	}
	dd := *d
	dd.Clock = clk
	step := func(h float64) { clk.Advance(time.Duration(h * float64(time.Hour))) }
	ok := func(_ any, err error) error { return err }
	t := true

	// Demo signal reporters (a clinic and a chemist with ORS history).
	if _, err := e.RegisterSignalReporter(ctx, Phone(901), domain.KindClinic, "Demo Primary Health Clinic", Urban, "hi", "seed"); err != nil {
		return err
	}
	chem, err := e.RegisterSignalReporter(ctx, Phone(902), domain.KindChemist, "Demo Chemist", Urban, "hi", "seed")
	if err != nil {
		return err
	}
	if _, err := e.RegisterSignalReporter(ctx, Phone(903), domain.KindASHA, "Demo ASHA worker", Village, "hi", "seed"); err != nil {
		return err
	}

	urbanSchool := domain.Point{Lat: 22.7178, Lon: 75.8620}
	steps := []func() error{
		// Village: a stale complaint via the volunteer line (no smartphone), escalated through the CM Helpline.
		func() error {
			return ok(e.Intake(ctx, agents.Inbound{Channel: domain.ChannelVolunteer, Phone: Phone(301), Text: "हैंडपंप के पास पानी में कीड़े और मिट्टी आ रही है",
				WardHint: Village, Consent: &t, HasSmartphone: ptr(false), Actor: "volunteer:demo", ProviderMsgID: "seed-1"}))
		},
		func() error { step(30); return nil },
		// Urban: a WhatsApp complaint near the school, later fixed and verified.
		func() error {
			return ok(e.HandleMessage(ctx, agents.Message{Phone: Phone(201), Text: "स्कूल के पास नल से पीला पानी आ रहा है", Point: near(urbanSchool, 20, 10), ProviderMsgID: "seed-2"}))
		},
		func() error {
			return ok(e.HandleMessage(ctx, agents.Message{Phone: Phone(201), Text: "हाँ", ProviderMsgID: "seed-3"}))
		},
		func() error { step(5); return nil },
		// Two missed-call recordings: one with a (scripted) machine transcript, one awaiting a human.
		func() error {
			return ok(e.Intake(ctx, agents.Inbound{Channel: domain.ChannelMissedCall, Phone: Phone(302), Audio: clipA, AudioType: "audio/wav", Consent: &t, ProviderMsgID: "seed-4"}))
		},
		func() error {
			return ok(e.Intake(ctx, agents.Inbound{Channel: domain.ChannelMissedCall, Phone: Phone(303), Audio: SilentWAV(1500), AudioType: "audio/wav", Consent: &t, ProviderMsgID: "seed-5"}))
		},
		func() error { step(20); return nil },
		func() error {
			return ok(e.Intake(ctx, agents.Inbound{Channel: domain.ChannelIVR, Phone: Phone(304), Audio: clipB, AudioType: "audio/wav", Consent: &t, ProviderMsgID: "seed-6"}))
		},
		func() error { step(26); return nil },
		// Urban official claims a fix; the resident says NO (fake closure caught), then a real fix is verified.
		func() error { return claimAndAnswer(ctx, &e, Phone(201), false) },
		func() error { step(30); return nil },
		func() error { return claimAndAnswer(ctx, &e, Phone(201), true) },
		// A web-form complaint from the urban ward (leak; not a water-quality signal).
		func() error {
			return ok(e.Intake(ctx, agents.Inbound{Channel: domain.ChannelWeb, Text: "Pipeline leaking on the main road near the ration shop for 3 days",
				Point: &domain.Point{Lat: 22.7192, Lon: 75.8578}, Phone: Phone(202), Consent: &t, ProviderMsgID: "seed-7"}))
		},
	}
	for _, f := range steps {
		if err := f(); err != nil {
			return err
		}
		if _, err := e.ProcessTimeouts(ctx); err != nil {
			return err
		}
		if _, err := e.ProcessCallbacks(ctx); err != nil {
			return err
		}
		if _, err := dd.Run(ctx); err != nil {
			return err
		}
	}
	// Chemist ORS history: a steady baseline (no spike), reported daily.
	for i := 0; i < 9; i++ {
		units := 8 + i%3
		at := now.Add(-time.Duration(9-i) * 24 * time.Hour)
		if _, err := live.AddIllness(ctx, domain.IllnessReport{ReporterType: domain.KindChemist, ReporterID: &chem.ID, WardID: Urban, ReportedAt: at,
			ORSUnitsSold: &units, Notes: "demo daily ORS report"}, "seed"); err != nil {
			return err
		}
	}
	// The partner recorded the CM Helpline number for the village grievance.
	escs, err := st.ListEscalations(ctx, store.EscalationFilter{WardID: Village, Limit: 50})
	if err != nil {
		return err
	}
	for _, x := range escs {
		if x.Channel == "grievance" && x.Outcome == domain.OutcomePending {
			_ = live.Acknowledge(ctx, x.ID, "grievance_portal", "CMH-DEMO-0001", "partner:demo")
			break
		}
	}
	// Pilot logs (fictional): outreach and baseline interviews.
	for _, o := range []domain.Outreach{
		{WardID: Urban, Households: 120, Method: "door_to_door", Note: "demo flyer distribution"},
		{WardID: Urban, Households: 80, Method: "whatsapp_group", Note: "demo RWA group"},
		{WardID: Village, Households: 95, Method: "meeting", Note: "demo gram sabha announcement"},
	} {
		o.Date, o.LoggedBy, o.CreatedAt = now.Add(-8*24*time.Hour), "seed", now
		if err := st.CreateOutreach(ctx, o); err != nil {
			return err
		}
	}
	for _, b := range []domain.Baseline{
		{WardID: Urban, Source: "resident_interview", DaysToFix: 21, Resolved: true, Note: "demo"},
		{WardID: Urban, Source: "resident_interview", DaysToFix: 45, Resolved: false, Note: "demo: never fixed"},
		{WardID: Village, Source: "partner", DaysToFix: 30, Resolved: true, Note: "demo"},
		{WardID: Urban, Source: "helpline_data", DaysToFix: 14, Resolved: true, Note: "demo"},
	} {
		b.CreatedAt = now
		if err := st.CreateBaseline(ctx, b); err != nil {
			return err
		}
	}
	if _, err := live.EvaluateAll(ctx); err != nil {
		return err
	}
	if _, err := live.ProcessTimeouts(ctx); err != nil {
		return err
	}
	_, err = d.Run(ctx)
	return err
}

func ptr[T any](v T) *T { return &v }

func claimAndAnswer(ctx context.Context, e *agents.Engine, phone string, yes bool) error {
	st := e.Store
	cs, err := st.ListComplaints(ctx, store.ComplaintFilter{WardID: Urban, Statuses: []domain.ComplaintStatus{domain.StatusEscalated, domain.StatusReopened, domain.StatusOpen}, Limit: 20})
	if err != nil {
		return err
	}
	for _, c := range cs {
		if c.IssueType == domain.IssueLeak {
			continue
		}
		if escs, err := st.ListEscalations(ctx, store.EscalationFilter{ComplaintID: c.ID, Outcomes: []domain.EscalationOutcome{domain.OutcomePending}}); err == nil && len(escs) > 0 {
			_ = e.Acknowledge(ctx, escs[0].ID, "link", "", "official:Demo Assistant Engineer")
		}
		photo, err := st.SaveMedia(ctx, domain.Media{SHA256: "demo", ContentType: "image/png", Data: RepairPhoto(), Kind: "photo", CreatedAt: e.Clock.Now()})
		if err != nil {
			return err
		}
		cl, err := e.ClaimFixed(ctx, agents.Claim{ComplaintID: c.ID, ClaimedBy: "Demo Assistant Engineer", PhotoMediaID: &photo, Note: "demo: pipe joint replaced"})
		if err != nil {
			return err
		}
		_, err = e.HandleMessage(ctx, agents.Message{Phone: phone, ReplyID: fmt.Sprintf("cb:%d:%s", cl.ID, map[bool]string{true: "yes", false: "no"}[yes])})
		return err
	}
	return nil
}
