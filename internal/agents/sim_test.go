package agents_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// TestRandomizedSimulation drives the whole system with thousands of random
// resident, official, lab, clinic and clock actions and checks invariants
// against the database after every step. Seeds are fixed for
// reproducibility; set BHAGIRATH_SIM_STEPS / BHAGIRATH_SIM_SEEDS to go longer.
func TestRandomizedSimulation(t *testing.T) {
	steps, seeds := 400, 3
	if v, err := strconv.Atoi(os.Getenv("BHAGIRATH_SIM_STEPS")); err == nil {
		steps = v
	}
	if v, err := strconv.Atoi(os.Getenv("BHAGIRATH_SIM_SEEDS")); err == nil {
		seeds = v
	}
	for seed := 1; seed <= seeds; seed++ {
		t.Run(fmt.Sprint("seed", seed), func(t *testing.T) { simulate(t, uint64(seed), steps) })
	}
}

var simTexts = []string{
	"शिव मंदिर के पास गंदा पानी आ रहा है", "नल से बदबूदार पानी, बच्चों को उल्टी दस्त", "नाली का पानी पीने के पानी में मिल रहा है",
	"dirty yellow water from tap", "sewage smell in drinking water near the school", "pani nahi aa raha", "pipe leakage near ration shop",
	"राशन की दुकान के पास काला पानी", "पानी की टंकी के पास बदबू", "hello", "गंदा पानी",
}

func simulate(t *testing.T, seed uint64, steps int) {
	h := newHarness(t)
	ctx := h.ctx
	rng := rand.New(rand.NewPCG(seed, seed*7919))
	phones := make([]string, 25)
	for i := range phones {
		phones[i] = fmt.Sprintf("+9100000%05d", 5000+i)
	}
	clinic := "+910000009001"
	if _, err := h.e.RegisterSignalReporter(ctx, clinic, domain.KindClinic, "Sim Clinic", ward, "hi", "sim"); err != nil {
		t.Fatal(err)
	}
	var log []string
	fail := func(format string, a ...any) {
		t.Helper()
		tail := log
		if len(tail) > 15 {
			tail = tail[len(tail)-15:]
		}
		t.Fatalf("seed %d after %d steps: %s\nlast actions:\n  %s", seed, len(log), fmt.Sprintf(format, a...), strings.Join(tail, "\n  "))
	}
	for step := 0; step < steps; step++ {
		var act string
		var err error
		switch r := rng.IntN(100); {
		case r < 30:
			p := phones[rng.IntN(len(phones))]
			var pt *domain.Point
			if rng.IntN(2) == 0 {
				pt = near(float64(rng.IntN(500)), float64(rng.IntN(500)))
			}
			act = "complaint " + p
			_, err = h.e.HandleMessage(ctx, agents.Message{Phone: p, Text: simTexts[rng.IntN(len(simTexts))], Point: pt, ProviderMsgID: fmt.Sprint("sim-", seed, "-", step)})
		case r < 38:
			p := phones[rng.IntN(len(phones))]
			ans := []string{"yes", "no", "हाँ", "1", "2", "राशन की दुकान", "school"}[rng.IntN(7)]
			act = "reply " + p + " " + ans
			_, err = h.e.HandleMessage(ctx, agents.Message{Phone: p, Text: ans, ProviderMsgID: fmt.Sprint("sim-r-", seed, "-", step)})
		case r < 44:
			v, lim := float64(rng.IntN(3)), 0.0
			param := []string{"E. coli", "Total coliform"}[rng.IntN(2)]
			act = fmt.Sprintf("test %s=%v", param, v)
			_, err = h.e.AddWaterTest(ctx, domain.WaterTest{Source: domain.TestFieldKit, WardID: ward, SampledAt: h.clk.Now(), Parameter: param, Value: &v, LimitValue: &lim}, "sim")
		case r < 50:
			msg := fmt.Sprintf("D %d V %d ORS %d", rng.IntN(4), rng.IntN(3), 5+rng.IntN(20))
			act = "clinic " + msg
			_, err = h.e.HandleMessage(ctx, agents.Message{Phone: clinic, Text: msg, ProviderMsgID: fmt.Sprint("sim-c-", seed, "-", step)})
		case r < 58:
			es, _ := h.st.ListEscalations(ctx, store.EscalationFilter{Outcomes: []domain.EscalationOutcome{domain.OutcomePending}, Limit: 50})
			if len(es) > 0 {
				x := es[rng.IntN(len(es))]
				act = "ack " + x.Ref
				err = h.e.Acknowledge(ctx, x.ID, "link", "", "sim")
				if err == agents.ErrClosed {
					err = nil
				}
			}
		case r < 70:
			cs, _ := h.st.ListComplaints(ctx, store.ComplaintFilter{Statuses: []domain.ComplaintStatus{domain.StatusEscalated, domain.StatusReopened}, Limit: 50})
			if len(cs) > 0 {
				c := cs[rng.IntN(len(cs))]
				photo, _ := h.st.SaveMedia(ctx, domain.Media{SHA256: "s", ContentType: "image/png", Data: []byte{1}, Kind: "photo", CreatedAt: h.clk.Now()})
				act = "claim " + c.Ref
				_, err = h.e.ClaimFixed(ctx, agents.Claim{ComplaintID: c.ID, ClaimedBy: "sim official", PhotoMediaID: &photo})
			}
		case r < 80:
			cls, _ := h.st.ListClosures(ctx, true, 50)
			if len(cls) > 0 {
				cl := cls[rng.IntN(len(cls))]
				yes := rng.IntN(2) == 0
				act = fmt.Sprintf("callback closure %d yes=%v", cl.ID, yes)
				err = h.e.RecordCallback(ctx, cl.ID, yes, "voice", "sim")
				if err == agents.ErrAlreadyDecided {
					err = nil
				}
			}
		case r < 84:
			as, _ := h.st.ListAlerts(ctx, store.AlertFilter{Status: "active"})
			for _, a := range as {
				if a.Level >= domain.LevelWarning {
					act = fmt.Sprintf("approve advisories alert %d", a.ID)
					_, err = h.e.ApproveAdvisories(ctx, a.ID, "sim partner")
				}
			}
		default:
			d := time.Duration(1+rng.IntN(30)) * time.Hour
			act = "advance " + d.String()
			h.clk.Advance(d)
			if _, err = h.e.ProcessTimeouts(ctx); err == nil {
				if _, err = h.e.ProcessCallbacks(ctx); err == nil {
					_, err = h.e.EvaluateAll(ctx)
				}
			}
		}
		if act == "" {
			continue
		}
		log = append(log, act)
		if err != nil {
			fail("action %q failed: %v", act, err)
		}
		if _, err := h.d.Run(ctx); err != nil {
			fail("dispatcher: %v", err)
		}
		checkInvariants(t, h, fail)
	}
}

func checkInvariants(t *testing.T, h *harness, fail func(string, ...any)) {
	t.Helper()
	ctx := h.ctx
	q := func(sql string) int {
		var n int
		if err := h.st.QueryRowForTest(ctx, sql).Scan(&n); err != nil {
			fail("invariant query failed: %v\n%s", err, sql)
		}
		return n
	}
	checks := map[string]string{
		"verified complaint without a confirmed, verified closure": `SELECT count(*) FROM complaint c WHERE status='verified_fixed'
			AND NOT EXISTS (SELECT 1 FROM closure x WHERE x.complaint_id=c.id AND x.verified AND x.callback_result='confirmed')`,
		"verified complaint without verified_fixed_at": `SELECT count(*) FROM complaint WHERE status='verified_fixed' AND verified_fixed_at IS NULL`,
		"claimed_fixed complaint without exactly one undecided closure": `SELECT count(*) FROM complaint c WHERE status='claimed_fixed'
			AND (SELECT count(*) FROM closure x WHERE x.complaint_id=c.id AND x.decided_at IS NULL) <> 1`,
		"undecided closure on a complaint that is not claimed_fixed": `SELECT count(*) FROM closure x JOIN complaint c ON c.id=x.complaint_id
			WHERE x.decided_at IS NULL AND c.status <> 'claimed_fixed'`,
		"closure without evidence": `SELECT count(*) FROM closure WHERE evidence_media_id IS NULL AND retest_id IS NULL`,
		"reopen count differs from rejected closures": `SELECT count(*) FROM complaint c WHERE reopen_count <>
			(SELECT count(*) FROM closure x WHERE x.complaint_id=c.id AND x.callback_result IN ('denied','no_answer'))`,
		"open escalation on a verified or invalid complaint": `SELECT count(*) FROM escalation e JOIN complaint c ON c.id=e.complaint_id
			WHERE e.outcome IN ('pending','acknowledged') AND c.status IN ('verified_fixed','invalid')`,
		"open escalation on a resolved alert": `SELECT count(*) FROM escalation e JOIN alert a ON a.id=e.alert_id
			WHERE e.outcome IN ('pending','acknowledged') AND a.status='resolved'`,
		"escalation deadline not after it was sent": `SELECT count(*) FROM escalation WHERE deadline_at <= sent_at`,
		"more than one active alert in a ward":      `SELECT count(*) FROM (SELECT ward_id FROM alert WHERE status='active' GROUP BY ward_id HAVING count(*) > 1) x`,
		"advisory to a household without consent":   `SELECT count(*) FROM advisory a JOIN reporter r ON r.id=a.reporter_id WHERE r.consent_at IS NULL`,
		"advisory for an alert below Warning":       `SELECT count(*) FROM advisory WHERE level NOT IN ('warning','critical')`,
		"message stuck in the outbox (demo mode)":   `SELECT count(*) FROM outbound_message WHERE status IN ('queued','failed')`,
		"phone number in plain text in the outbox":  `SELECT count(*) FROM outbound_message WHERE body ~ '0000\d{5}' OR recipient_label ~ '0000\d{5}'`,
		"complaint escalated without a ward":        `SELECT count(*) FROM complaint WHERE status='escalated' AND ward_id IS NULL`,
		"declined reporter still holds a number":    `SELECT count(*) FROM reporter WHERE consent_declined AND phone_enc IS NOT NULL AND consent_at IS NULL`,
		"peak level below current level": `SELECT count(*) FROM alert WHERE
			(CASE peak_level WHEN 'watch' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END) < (CASE level WHEN 'watch' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END)`,
	}
	for name, sql := range checks {
		if n := q(sql); n != 0 {
			fail("invariant violated (%d rows): %s", n, name)
		}
	}
	// The active alert always equals what the frozen rule says right now.
	w, _ := h.st.GetWard(ctx, ward)
	snap, err := h.e.Snapshot(ctx, h.st, w, h.clk.Now())
	if err != nil {
		fail("snapshot: %v", err)
	}
	want := h.e.Rule.Evaluate(snap, h.clk.Now()).Level
	as, _ := h.st.ListAlerts(ctx, store.AlertFilter{WardID: ward, Status: "active"})
	got := domain.LevelNone
	if len(as) > 0 {
		got = as[0].Level
	}
	if got != want {
		fail("active alert level %v but the rule says %v", got, want)
	}
}
