package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/demo"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// The guided story mirrors the demo-video script (spec Section 17).
type storyStep struct {
	Action, Title, Detail string
}

var story = []storyStep{
	{"story-missed-call", "1 · An elderly resident gives a missed call", "No smartphone. Bhagirath calls back free; she records a Hindi complaint naming a landmark (Shiv Mandir). It is transcribed, classified, mapped to the ward and escalated to the Assistant Engineer."},
	{"story-neighbours", "2 · Two neighbours report on WhatsApp", "Three households within 300 m in a few hours → the frozen rule raises a WATCH alert with a plain-language reason."},
	{"story-test", "3 · A field test finds coliform", "Complaint cluster + failed test → WARNING. Level-2 engineer and the health officer get evidence packs; household advisories wait for partner sign-off."},
	{"story-clinic", "4 · A clinic reports 5 diarrhoea cases", "All three signals → CRITICAL. Every rung, including the commissioner and partner, is notified with a 24-hour deadline."},
	{"story-timeout", "5 · Nobody responds for 2 days", "The clock jumps 49 hours: unacknowledged escalations time out and automatically climb the ladder."},
	{"story-fake-fix", "6 · The engineer marks it fixed — the resident says NO", "A photo is uploaded, Bhagirath calls the resident, she presses 2 → the complaint is reopened and escalated one level up. This is exactly the Indore failure, caught."},
	{"story-real-fix", "7 · A real fix, verified", "A passing retest is uploaded, the resident presses 1 → verified fixed. Only now does it count as closed."},
}

func (s *Server) demoPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	done := map[string]bool{}
	for _, st := range story {
		if v, err := s.Engine.Store.KVGet(ctx, "demo:done:"+st.Action); err == nil && v == "1" {
			done[st.Action] = true
		}
	}
	var offset time.Duration
	if s.DemoClock != nil {
		offset = s.DemoClock.Elapsed()
	}
	wards, _ := s.Engine.Store.ListWards(ctx)
	s.render(w, r, "demo", map[string]any{"Title": "Demo simulator", "Story": story, "Done": done, "Offset": offset, "Wards": wards,
		"Phones": []string{demo.Phone(101), demo.Phone(102), demo.Phone(103), demo.Phone(104), demo.Phone(105)}, "Clinic": demo.Phone(901)})
}

func (s *Server) demoAction(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.Demo() {
		http.Error(w, "the simulator is only available in demo mode", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	action := r.PathValue("action")
	to, msg, err := s.runDemo(ctx, r, action)
	if err != nil {
		redirectOK(w, r, "/app/demo", "Could not run: "+err.Error())
		return
	}
	s.invalidateImpact()
	if strings.HasPrefix(action, "story-") {
		_ = s.Engine.Store.KVSet(ctx, "demo:done:"+action, "1", s.Engine.Clock.Now())
	}
	if s.Scheduler != nil {
		_ = s.Scheduler.RunAll(ctx)
	} else {
		_, _ = s.Dispatcher.Run(ctx)
	}
	redirectOK(w, r, to, msg)
}

func (s *Server) advance(ctx context.Context, d time.Duration) error {
	if s.DemoClock == nil {
		return fmt.Errorf("clock control unavailable")
	}
	s.DemoClock.Advance(d)
	return s.Engine.Store.KVSet(ctx, "demo_clock_offset", s.DemoClock.Elapsed().String(), s.Engine.Clock.Now())
}

func (s *Server) storyComplaint(ctx context.Context) (domain.Complaint, error) {
	ref, err := s.Engine.Store.KVGet(ctx, "demo:story:complaint")
	if err != nil {
		return domain.Complaint{}, fmt.Errorf("run step 1 first")
	}
	return s.Engine.Store.GetComplaintByRef(ctx, ref)
}

func (s *Server) runDemo(ctx context.Context, r *http.Request, action string) (string, string, error) {
	e := s.Engine
	t := true
	shiv := domain.Point{Lat: 22.7205, Lon: 75.8590}
	at := func(dy, dx float64) *domain.Point {
		return &domain.Point{Lat: shiv.Lat + dy/111_195, Lon: shiv.Lon + dx/102_550}
	}
	switch action {
	case "story-missed-call":
		res, err := e.Intake(ctx, agents.Inbound{Channel: domain.ChannelMissedCall, Phone: demo.Phone(101), Consent: &t, Language: "hi",
			Text:          "शिव मंदिर के पास हमारे नल से दो हफ्ते से बदबूदार और गंदा पानी आ रहा है, पोते को दस्त हो रहे हैं",
			ProviderMsgID: fmt.Sprintf("sim-call-%d", e.Clock.Now().UnixNano()), Actor: "simulator:ivr"})
		if err != nil {
			return "", "", err
		}
		_ = e.Store.KVSet(ctx, "demo:story:complaint", res.Complaint.Ref, e.Clock.Now())
		return "/app/complaints/" + res.Complaint.Ref, "Missed call → callback → Hindi recording → complaint " + res.Complaint.Ref + " (transcript simulated; live deployments use speech recognition).", nil
	case "story-neighbours":
		for i, txt := range []string{"मंदिर वाली गली में नल का पानी काला और बदबूदार है", "Gutter water is mixing with our tap water near the temple"} {
			p := at(float64(90+60*i), float64(-70+150*i))
			if _, err := e.HandleMessage(ctx, agents.Message{Phone: demo.Phone(102 + i), Text: txt, Point: p, ProviderMsgID: fmt.Sprintf("sim-wa-%d-%d", i, e.Clock.Now().UnixNano()), Actor: "simulator:whatsapp"}); err != nil {
				return "", "", err
			}
			_, _ = e.HandleMessage(ctx, agents.Message{Phone: demo.Phone(102 + i), Text: "YES", ProviderMsgID: fmt.Sprintf("sim-wa-yes-%d-%d", i, e.Clock.Now().UnixNano())})
		}
		return s.alertURL(ctx, demo.Urban), "Two WhatsApp complaints with location pins. The rule found a 3-household cluster.", nil
	case "story-test":
		v, l := 23.0, 0.0
		if _, err := e.AddWaterTest(ctx, domain.WaterTest{Source: domain.TestFieldKit, WardID: demo.Urban, Point: at(30, 20), SampledAt: e.Clock.Now().Add(-2 * time.Hour),
			Parameter: "Total coliform", Value: &v, LimitValue: &l, Unit: "MPN/100 ml", Note: "demo H2S/coliform field kit"}, "simulator:partner"); err != nil {
			return "", "", err
		}
		return s.alertURL(ctx, demo.Urban), "Coliform-positive field test recorded. Two signals of three → Warning.", nil
	case "story-clinic":
		if _, err := e.HandleMessage(ctx, agents.Message{Phone: demo.Phone(901), Text: "D 5 V 3", ProviderMsgID: fmt.Sprintf("sim-clinic-%d", e.Clock.Now().UnixNano()), Actor: "simulator:clinic"}); err != nil {
			return "", "", err
		}
		return s.alertURL(ctx, demo.Urban), "Clinic WhatsApp \"D 5 V 3\" parsed as 5 diarrhoea and 3 vomiting cases → all three signals → Critical.", nil
	case "story-timeout", "advance":
		h := 49
		if v, err := strconv.Atoi(r.FormValue("hours")); err == nil && v > 0 && v <= 24*14 {
			h = v
		}
		if err := s.advance(ctx, time.Duration(h)*time.Hour); err != nil {
			return "", "", err
		}
		n, err := e.ProcessTimeouts(ctx)
		if err != nil {
			return "", "", err
		}
		if _, err := e.ProcessCallbacks(ctx); err != nil {
			return "", "", err
		}
		return "/app/escalations?show=all", fmt.Sprintf("Clock advanced %d h. %d escalations timed out and moved up the ladder.", h, n), nil
	case "story-fake-fix", "story-real-fix":
		c, err := s.storyComplaint(ctx)
		if err != nil {
			return "", "", err
		}
		claim := agents.Claim{ComplaintID: c.ID, ClaimedBy: "Demo Assistant Engineer (simulated)"}
		if action == "story-fake-fix" {
			data := demo.RepairPhoto()
			sum := sha256.Sum256(data)
			id, err := e.Store.SaveMedia(ctx, domain.Media{SHA256: hex.EncodeToString(sum[:]), ContentType: "image/png", Data: data, Kind: "photo", CreatedAt: e.Clock.Now()})
			if err != nil {
				return "", "", err
			}
			claim.PhotoMediaID, claim.Note = &id, "Line flushed (simulated claim)"
		} else {
			v, l := 0.0, 0.0
			t, err := e.AddWaterTest(ctx, domain.WaterTest{Source: domain.TestLabReport, WardID: demo.Urban, Point: c.Point, SampledAt: e.Clock.Now(), Parameter: "Total coliform",
				Value: &v, LimitValue: &l, Unit: "MPN/100 ml", Note: "demo retest after pipe replacement"}, "simulator:lab")
			if err != nil {
				return "", "", err
			}
			claim.RetestID, claim.Note = &t.ID, "Leaking sewer crossing replaced (simulated)"
		}
		cl, err := e.ClaimFixed(ctx, claim)
		if err != nil {
			return "", "", err
		}
		// The resident answers the verification call.
		yes := action == "story-real-fix"
		if err := e.RecordCallback(ctx, cl.ID, yes, "voice", "simulator:resident"); err != nil {
			return "", "", err
		}
		if yes {
			return "/app/complaints/" + c.Ref, "Retest passed and the resident pressed 1 → verified fixed.", nil
		}
		return "/app/complaints/" + c.Ref, "Official claimed a fix; the resident pressed 2 → fake closure caught, reopened and escalated one level up.", nil
	case "whatsapp":
		m := agents.Message{Phone: r.FormValue("phone"), Text: strings.TrimSpace(r.FormValue("text")), ProviderMsgID: fmt.Sprintf("sim-%d", e.Clock.Now().UnixNano()), Actor: "simulator:whatsapp"}
		if lat, lon := optFloat(r.FormValue("lat")), optFloat(r.FormValue("lon")); lat != nil && lon != nil {
			m.Point = &domain.Point{Lat: *lat, Lon: *lon}
		}
		out, err := e.HandleMessage(ctx, m)
		if err != nil {
			return "", "", err
		}
		if out.Complaint != nil {
			return "/app/complaints/" + out.Complaint.Ref, "WhatsApp message handled as " + out.Handled, nil
		}
		return "/app/outbox", "WhatsApp message handled as " + out.Handled + " " + out.Detail, nil
	case "missed-call":
		consent := r.FormValue("consent") == "on"
		in := agents.Inbound{Channel: domain.ChannelMissedCall, Phone: r.FormValue("phone"), Text: strings.TrimSpace(r.FormValue("text")), Language: r.FormValue("lang"),
			Consent: &consent, ProviderMsgID: fmt.Sprintf("sim-call-%d", e.Clock.Now().UnixNano()), Actor: "simulator:ivr"}
		if in.Text == "" {
			in.Audio, in.AudioType = demo.SilentWAV(1800), "audio/wav" // goes to the human transcription queue
		}
		res, err := e.Intake(ctx, in)
		if err != nil {
			return "", "", err
		}
		return "/app/complaints/" + res.Complaint.Ref, "Missed-call complaint " + res.Complaint.Ref + " recorded.", nil
	case "reset":
		if s.Seed == nil {
			return "", "", fmt.Errorf("reset not configured")
		}
		if s.DemoClock != nil {
			s.DemoClock.Set(0)
		}
		if err := s.Seed(ctx); err != nil {
			return "", "", err
		}
		return "/app/demo", "Demo data reset. Start the story from step 1.", nil
	}
	return "", "", fmt.Errorf("unknown action %q", action)
}

func (s *Server) alertURL(ctx context.Context, ward string) string {
	as, err := s.Engine.Store.ListAlerts(ctx, store.AlertFilter{WardID: ward, Status: "active", Limit: 1})
	if err != nil || len(as) == 0 {
		return "/app"
	}
	return fmt.Sprintf("/app/alerts/%d", as[0].ID)
}
