package web_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/asr"
	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/demo"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/extract"
	"github.com/vivekyarra/bhagirath/internal/jobs"
	"github.com/vivekyarra/bhagirath/internal/notify"
	"github.com/vivekyarra/bhagirath/internal/rule"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
	"github.com/vivekyarra/bhagirath/internal/store/storetest"
	"github.com/vivekyarra/bhagirath/internal/web"
)

type env struct {
	t   *testing.T
	srv *httptest.Server
	s   *web.Server
	st  *store.Store
	c   *http.Client
}

func setup(t *testing.T, mode config.Mode) *env {
	t.Helper()
	web.BcryptCost = 4
	st := storetest.New(t)
	t.Setenv("BHAGIRATH_MODE", string(mode))
	t.Setenv("BHAGIRATH_SECRET_KEY", strings.Repeat("k", 32))
	t.Setenv("BHAGIRATH_ADMIN_PASSWORD", "admin-password-123")
	t.Setenv("BHAGIRATH_LADDER_FILE", "../../config/ladder.example.yaml")
	t.Setenv("WHATSAPP_APP_SECRET", "appsecret")
	t.Setenv("WHATSAPP_VERIFY_TOKEN", "verify-me")
	if mode == config.ModePilot {
		t.Setenv("BHAGIRATH_LADDER_FILE", "../../config/ladder.example.yaml.copy")
	}
	cfg, err := config.Load()
	if err != nil && mode == config.ModeDemo {
		t.Fatal(err)
	}
	if cfg == nil { // pilot: bypass the ladder-file guard for tests
		t.Setenv("BHAGIRATH_LADDER_FILE", "x.yaml")
		cfg, err = config.Load()
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg.LadderFile = "../../config/ladder.example.yaml"
	r, _ := rule.Load("../../config/alert_rule_v1.yaml")
	l, _ := config.LoadLadder(cfg.LadderFile)
	v, _ := secure.NewVault(cfg.SecretKey)
	oc := &clock.Offset{}
	log := slog.New(slog.DiscardHandler)
	e := &agents.Engine{Store: st, Clock: oc, Rule: r, Ladder: l, Vault: v, Extractor: extract.Rules{}, ASR: asr.None{}, Cfg: cfg, Log: log}
	ctx := context.Background()
	if err := e.SyncLadder(ctx, l); err != nil {
		t.Fatal(err)
	}
	d := &notify.Dispatcher{Store: st, Vault: v, Clock: oc, Cfg: cfg, Log: log}
	hash, _ := web.HashPassword("judge-password")
	_ = st.UpsertUser(ctx, domain.User{Username: "judge", PasswordHash: hash, Role: domain.RoleJudge})
	hash, _ = web.HashPassword("admin-password-123")
	_ = st.UpsertUser(ctx, domain.User{Username: "admin", PasswordHash: hash, Role: domain.RoleAdmin})
	hash, _ = web.HashPassword("viewer-password")
	_ = st.UpsertUser(ctx, domain.User{Username: "viewer", PasswordHash: hash, Role: domain.RoleViewer})
	s := &web.Server{Engine: e, Dispatcher: d, Scheduler: jobs.New(e, d, log), Cfg: cfg, Log: log, Version: "test",
		Seed: func(ctx context.Context) error { return demo.Seed(ctx, e, d) }}
	if mode == config.ModeDemo {
		s.DemoClock = oc
	}
	if err := s.Start(ctx, 2); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	cfg.BaseURL = srv.URL
	t.Cleanup(func() { srv.Close(); s.Stop() })
	jar, _ := cookiejar.New(nil)
	return &env{t: t, srv: srv, s: s, st: st, c: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

var csrfRe = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func (e *env) get(path string) (int, string) {
	e.t.Helper()
	resp, err := e.c.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (e *env) post(path string, form url.Values) (int, string, string) {
	e.t.Helper()
	resp, err := e.c.PostForm(e.srv.URL+path, form)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header.Get("Location")
}

func (e *env) login(user, pw string) string {
	e.t.Helper()
	_, body := e.get("/login")
	m := csrfRe.FindStringSubmatch(body)
	code, _, _ := e.post("/login", url.Values{"username": {user}, "password": {pw}, "csrf": {m[1]}, "next": {"/app"}})
	if code != http.StatusSeeOther {
		e.t.Fatalf("login %s: %d", user, code)
	}
	_, body = e.get("/app")
	return csrfRe.FindStringSubmatch(body)[1]
}

func TestPublicPagesAndHealth(t *testing.T) {
	e := setup(t, config.ModeDemo)
	for _, p := range []string{"/", "/report", "/report?lang=te", "/impact", "/api/v1/impact", "/login", "/healthz", "/static/app.css", "/static/leaflet/leaflet.js"} {
		if code, _ := e.get(p); code != 200 {
			t.Errorf("%s = %d", p, code)
		}
	}
	if code, _ := e.get("/app"); code != http.StatusSeeOther {
		t.Errorf("/app without login = %d, want redirect", code)
	}
	if code, _ := e.get("/api/v1/map"); code != http.StatusUnauthorized {
		t.Errorf("/api/v1/map without login = %d", code)
	}
}

func TestWebFormIntakeAndValidation(t *testing.T) {
	e := setup(t, config.ModeDemo)
	_, body := e.get("/report?lang=hi")
	tok := csrfRe.FindStringSubmatch(body)[1]
	code, out, _ := e.post("/report", url.Values{"csrf": {tok}, "issue": {"sewage_mixing"}, "text": {"नाली का पानी मिल रहा है शिव मंदिर के पास"}, "phone": {"98765 43210"}, "consent": {"on"}, "lang": {"hi"}})
	if code != 200 || !strings.Contains(out, "BG-") {
		t.Fatalf("submit: %d", code)
	}
	cs, _ := e.st.ListComplaints(context.Background(), store.ComplaintFilter{})
	if len(cs) != 1 || cs[0].WardID != demo.Urban || cs[0].Channel != domain.ChannelWeb {
		t.Fatalf("complaint: %+v", cs)
	}
	if code, _, _ := e.post("/report", url.Values{"csrf": {"forged"}, "text": {"x"}}); code != 400 {
		t.Errorf("forged csrf accepted: %d", code)
	}
	if code, _, _ := e.post("/report", url.Values{"csrf": {tok}, "text": {"x"}, "phone": {"not-a-phone"}}); code != 400 {
		t.Errorf("bad phone accepted: %d", code)
	}
	if code, _, _ := e.post("/report", url.Values{"csrf": {tok}, "issue": {"<script>"}}); code != 400 {
		t.Errorf("bad issue accepted: %d", code)
	}
	n, _ := e.st.CountComplaints(context.Background(), store.ComplaintFilter{})
	if n != 1 {
		t.Errorf("invalid submissions created complaints: %d", n)
	}
}

func TestDashboardAuthCSRFAndRoles(t *testing.T) {
	e := setup(t, config.ModeDemo)
	tok := e.login("viewer", "viewer-password")
	for _, p := range []string{"/app", "/app/complaints", "/app/escalations", "/app/review", "/app/signals", "/app/outbox", "/app/pilot", "/app/settings", "/api/v1/map"} {
		if code, _ := e.get(p); code != 200 {
			t.Errorf("viewer GET %s = %d", p, code)
		}
	}
	if code, _, _ := e.post("/app/signals/test", url.Values{"csrf": {tok}, "ward": {demo.Urban}, "parameter": {"pH"}, "source": {"lab_report"}}); code != http.StatusForbidden {
		t.Errorf("viewer could write: %d", code)
	}
	if code, _ := e.get("/app/demo"); code != http.StatusForbidden {
		t.Errorf("viewer could open simulator: %d", code)
	}
	e2 := setup(t, config.ModeDemo)
	tok = e2.login("judge", "judge-password")
	if code, _, _ := e2.post("/app/signals/test", url.Values{"csrf": {"wrong"}, "ward": {demo.Urban}, "parameter": {"pH"}, "source": {"lab_report"}}); code != http.StatusForbidden {
		t.Errorf("missing CSRF accepted: %d", code)
	}
	if code, _, loc := e2.post("/app/signals/test", url.Values{"csrf": {tok}, "ward": {demo.Urban}, "parameter": {"E. coli"}, "value": {"5"}, "limit": {"0"}, "source": {"lab_report"}}); code != http.StatusSeeOther || !strings.Contains(loc, "ok=") {
		t.Errorf("judge write in demo: %d %s", code, loc)
	}
	as, _ := e2.st.ListAlerts(context.Background(), store.AlertFilter{Status: "active"})
	if len(as) != 1 || as[0].Level != domain.LevelWatch {
		t.Fatalf("value above limit should have raised a Watch alert: %+v", as)
	}
	if code, _, _ := e2.post("/login", url.Values{"username": {"judge"}, "password": {"wrong"}, "csrf": {"x"}}); code == http.StatusSeeOther {
		t.Error("login with bad csrf/password succeeded")
	}
}

func TestGuidedStoryOverHTTP(t *testing.T) {
	e := setup(t, config.ModeDemo)
	tok := e.login("judge", "judge-password")
	if code, _, loc := e.post("/app/demo/reset", url.Values{"csrf": {tok}}); code != http.StatusSeeOther || strings.Contains(loc, "Could+not") {
		t.Fatalf("reset: %d %s", code, loc)
	}
	for _, step := range []string{"story-missed-call", "story-neighbours", "story-test", "story-clinic", "story-timeout", "story-fake-fix", "story-real-fix"} {
		code, _, loc := e.post("/app/demo/"+step, url.Values{"csrf": {tok}})
		if code != http.StatusSeeOther || strings.Contains(loc, "Could+not") {
			t.Fatalf("%s: %d %s", step, code, loc)
		}
	}
	m, _ := e.st.ComputeMetrics(context.Background(), false, e.s.Engine.Clock.Now())
	if m.ReopenedComplaints < 2 || m.VerifiedClosures < 2 || m.AlertsByPeak["critical"] != 1 || m.EscalationsTimedOut == 0 { // seed history + story
		t.Fatalf("story metrics: reopened=%d verified=%d peaks=%v timeouts=%d", m.ReopenedComplaints, m.VerifiedClosures, m.AlertsByPeak, m.EscalationsTimedOut)
	}
	// Every dashboard page renders after the story.
	for _, p := range []string{"/app", "/app/complaints", "/app/escalations?show=all", "/app/review", "/app/outbox", "/app/pilot", "/app/demo", "/app/pilot/METRICS.md", "/impact"} {
		if code, body := e.get(p); code != 200 || strings.Contains(body, "<no value>") {
			t.Errorf("%s = %d", p, code)
		}
	}
}

func TestOfficialLinkAndSimulatorGuard(t *testing.T) {
	e := setup(t, config.ModeDemo)
	ctx := context.Background()
	res, err := e.s.Engine.Intake(ctx, agents.Inbound{Channel: domain.ChannelWeb, Text: "गंदा पानी शिव मंदिर", Phone: "+910000000777", Consent: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Dispatcher.Run(ctx); err != nil {
		t.Fatal(err)
	}
	msgs, _ := e.st.ListMessages(ctx, store.MessageFilter{Kind: "escalation"})
	if len(msgs) == 0 || msgs[0].Link == "" {
		t.Fatal("no escalation link")
	}
	link := strings.TrimPrefix(msgs[0].Link, e.srv.URL)
	code, body := e.get(link)
	if code != 200 || !strings.Contains(body, res.Complaint.Ref) {
		t.Fatalf("official page: %d", code)
	}
	if code, _ := e.get("/o/" + strings.Repeat("A", 43)); code != 404 {
		t.Errorf("forged token = %d", code)
	}
	tok := csrfRe.FindStringSubmatch(body)[1]
	if code, _, _ := e.post(link+"/ack", url.Values{"csrf": {tok}, "grievance": {"CMH-1"}}); code != http.StatusSeeOther {
		t.Fatalf("ack = %d", code)
	}
	es, _ := e.st.ListEscalations(ctx, store.EscalationFilter{ComplaintID: res.Complaint.ID})
	if es[0].AcknowledgedAt == nil || es[0].GrievanceNumber != "CMH-1" {
		t.Fatalf("ack not stored: %+v", es[0])
	}
	// Claim without evidence is refused.
	if code, _, loc := e.post(link+"/fixed", url.Values{"csrf": {tok}, "complaint_id": {itoa(res.Complaint.ID)}}); code != http.StatusSeeOther || !strings.Contains(loc, "Not+saved") {
		t.Fatalf("evidence-less claim: %d %s", code, loc)
	}
}

func TestWhatsAppWebhookSecurity(t *testing.T) {
	e := setup(t, config.ModeDemo)
	if code, body := e.get("/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=verify-me&hub.challenge=42"); code != 200 || body != "42" {
		t.Fatalf("verify: %d %q", code, body)
	}
	if code, _ := e.get("/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=nope&hub.challenge=42"); code != 403 {
		t.Fatal("wrong verify token accepted")
	}
	payload := `{"entry":[{"changes":[{"value":{"messages":[{"from":"910000000555","id":"wamid.T1","timestamp":"1762300000","type":"text","text":{"body":"शिव मंदिर के पास गंदा पानी"}}]}}]}]}`
	resp, _ := e.c.Post(e.srv.URL+"/webhooks/whatsapp", "application/json", strings.NewReader(payload))
	if resp.StatusCode != 401 {
		t.Fatalf("unsigned webhook = %d", resp.StatusCode)
	}
	mac := hmac.New(sha256.New, []byte("appsecret"))
	mac.Write([]byte(payload))
	req, _ := http.NewRequest("POST", e.srv.URL+"/webhooks/whatsapp", strings.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := e.c.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("signed webhook: %v %d", err, resp.StatusCode)
	}
	e.s.Stop() // drain background processing
	n, _ := e.st.CountComplaints(context.Background(), store.ComplaintFilter{})
	if n != 1 {
		t.Fatalf("webhook complaints = %d", n)
	}
	e.s.Start(context.Background(), 1) //nolint:errcheck // restart so Cleanup's Stop works
}

func TestPilotModeHidesSimulator(t *testing.T) {
	e := setup(t, config.ModePilot)
	tok := e.login("admin", "admin-password-123")
	if code, _, _ := e.post("/app/demo/story-missed-call", url.Values{"csrf": {tok}}); code != http.StatusForbidden {
		t.Fatalf("simulator ran in pilot mode: %d", code)
	}
	if code, _ := e.get("/"); code != 200 {
		t.Fatal("landing")
	}
}

func ptr[T any](v T) *T { return &v }

func itoa(n int64) string { return fmtI(n) }

func fmtI(n int64) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func TestRateLimitCannotBeSpoofed(t *testing.T) {
	e := setup(t, config.ModeDemo)
	codes := map[int]int{}
	for i := 0; i < 15; i++ {
		req, _ := http.NewRequest("POST", e.srv.URL+"/login", strings.NewReader("username=x&password=y"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", "10.0.0."+itoa(int64(i))+", 203.0.113.9")
		resp, err := e.c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		codes[resp.StatusCode]++
	}
	if codes[http.StatusTooManyRequests] == 0 {
		t.Fatalf("rotating a spoofed X-Forwarded-For prefix bypassed the login limiter: %v", codes)
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := setup(t, config.ModeDemo)
	resp, err := e.c.Get(e.srv.URL + "/report")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if resp.Header.Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	if strings.Contains(resp.Header.Get("Content-Security-Policy"), "unsafe-eval") {
		t.Error("CSP allows eval")
	}
}

func TestRecordingsNeedOperateRole(t *testing.T) {
	e := setup(t, config.ModeDemo)
	ctx := context.Background()
	id, err := e.st.SaveMedia(ctx, domain.Media{SHA256: "a", ContentType: "audio/wav", Data: demo.SilentWAV(100), Kind: "audio"})
	if err != nil {
		t.Fatal(err)
	}
	e.login("viewer", "viewer-password")
	if code, _ := e.get("/app/media/" + itoa(id)); code != http.StatusForbidden {
		t.Fatalf("viewer played a recording: %d", code)
	}
	e2 := setup(t, config.ModeDemo)
	id2, _ := e2.st.SaveMedia(ctx, domain.Media{SHA256: "a", ContentType: "audio/wav", Data: demo.SilentWAV(100), Kind: "audio"})
	e2.login("judge", "judge-password")
	if code, _ := e2.get("/app/media/" + itoa(id2)); code != 200 {
		t.Fatalf("demo judge could not play a recording: %d", code)
	}
}
