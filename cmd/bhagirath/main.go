// Command bhagirath runs the Bhagirath accountability loop.
//
//	bhagirath serve                    web server + scheduled agents
//	bhagirath migrate                  apply database migrations
//	bhagirath seed                     (demo) reset and seed fictional data
//	bhagirath export-metrics [-o f]    write pilot/METRICS.md from the database
//	bhagirath create-user NAME ROLE    create a dashboard user (password on stdin)
//	bhagirath backtest [flags]         replay outbreak timelines through the frozen rule
//	bhagirath rule                     print the rule version and SHA-256
//	bhagirath version
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/asr"
	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/demo"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/extract"
	"github.com/vivekyarra/bhagirath/internal/jobs"
	"github.com/vivekyarra/bhagirath/internal/llm"
	"github.com/vivekyarra/bhagirath/internal/metrics"
	"github.com/vivekyarra/bhagirath/internal/notify"
	"github.com/vivekyarra/bhagirath/internal/rule"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/store"
	"github.com/vivekyarra/bhagirath/internal/telephony"
	"github.com/vivekyarra/bhagirath/internal/web"
	"github.com/vivekyarra/bhagirath/internal/whatsapp"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bhagirath:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch cmd {
	case "version":
		fmt.Println("bhagirath", version)
		return nil
	case "rule":
		r, err := loadRule(envOr("BHAGIRATH_RULE_FILE", "config/alert_rule_v1.yaml"))
		if err != nil {
			return err
		}
		fmt.Printf("rule %s sha256 %s frozen-tag %s\n", r.Version, r.SHA256, rule.FrozenTag)
		return nil
	case "backtest":
		return runBacktest(args)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	app, err := build(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.store.Close()
	switch cmd {
	case "migrate":
		return nil // build already migrated
	case "seed":
		if !cfg.Demo() {
			return errors.New("seed only runs in demo mode (it deletes all data)")
		}
		return demo.Seed(ctx, app.engine, app.dispatcher)
	case "export-metrics":
		fs := flag.NewFlagSet("export-metrics", flag.ContinueOnError)
		out := fs.String("o", "pilot/METRICS.md", "output file")
		if err := fs.Parse(args); err != nil {
			return err
		}
		return exportMetrics(ctx, app, *out)
	case "create-user":
		if len(args) != 2 {
			return errors.New("usage: bhagirath create-user NAME admin|partner|judge|viewer  (password on stdin)")
		}
		pw, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		return ensureUser(ctx, app.store, args[0], strings.TrimSpace(pw), domain.Role(args[1]), app.engine.Clock.Now())
	case "serve":
		return serve(ctx, cfg, app)
	}
	return fmt.Errorf("unknown command %q", cmd)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func loadRule(path string) (*rule.Rule, error) {
	r, err := rule.Load(path)
	if err != nil {
		return nil, err
	}
	if err := r.CheckFrozen(); err != nil {
		return nil, err
	}
	return r, nil
}

type app struct {
	store      *store.Store
	engine     *agents.Engine
	dispatcher *notify.Dispatcher
	demoClock  *clock.Offset
	wa         *whatsapp.Client
	twilio     *telephony.Twilio
	exotel     *telephony.Exotel
	log        *slog.Logger
}

func build(ctx context.Context, cfg *config.Config) (*app, error) {
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	r, err := loadRule(cfg.RuleFile)
	if err != nil {
		return nil, err
	}
	ladder, err := config.LoadLadder(cfg.LadderFile)
	if err != nil {
		return nil, err
	}
	vault, err := secure.NewVault(cfg.SecretKey)
	if err != nil {
		return nil, err
	}
	st, err := store.OpenWithRetry(ctx, cfg.DatabaseURL, 90*time.Second)
	if err != nil {
		return nil, err
	}
	if err := st.Migrate(ctx); err != nil {
		st.Close()
		return nil, err
	}
	a := &app{store: st, log: log}
	var clk clock.Clock = clock.System{}
	if cfg.Demo() {
		a.demoClock = &clock.Offset{}
		if v, err := st.KVGet(ctx, "demo_clock_offset"); err == nil {
			if d, err := time.ParseDuration(v); err == nil {
				a.demoClock.Set(d)
			}
		}
		clk = a.demoClock
	}
	var ex extract.Extractor = extract.Rules{}
	if cfg.LLM.Enabled() {
		var c llm.Client
		switch cfg.LLM.Provider {
		case "anthropic":
			c = &llm.Anthropic{APIKey: cfg.LLM.APIKey, Model: envOr("LLM_MODEL", "claude-haiku-5-5"), BaseURL: cfg.LLM.BaseURL}
		default:
			c = &llm.OpenAICompatible{APIKey: cfg.LLM.APIKey, Model: cfg.LLM.Model, BaseURL: cfg.LLM.BaseURL}
		}
		ex = extract.LLM{Client: c}
	}
	var tr asr.Transcriber = asr.None{}
	if cfg.Sarvam.APIKey != "" {
		tr = &asr.Sarvam{APIKey: cfg.Sarvam.APIKey, Model: cfg.Sarvam.Model, BaseURL: cfg.Sarvam.BaseURL}
	}
	a.engine = &agents.Engine{Store: st, Clock: clk, Rule: r, Ladder: ladder, Vault: vault, Extractor: ex, ASR: tr, Cfg: cfg, Log: log}
	a.dispatcher = &notify.Dispatcher{Store: st, Vault: vault, Clock: clk, Cfg: cfg, Log: log}
	if cfg.WhatsApp.Enabled() {
		a.wa = &whatsapp.Client{Token: cfg.WhatsApp.Token, PhoneNumberID: cfg.WhatsApp.PhoneNumberID, APIVersion: cfg.WhatsApp.APIVersion}
		a.dispatcher.WhatsApp = a.wa
	}
	if cfg.Twilio.Enabled() {
		a.twilio = &telephony.Twilio{AccountSID: cfg.Twilio.AccountSID, AuthToken: cfg.Twilio.AuthToken, From: cfg.Twilio.FromNumber}
		a.dispatcher.Twilio = a.twilio
	}
	if cfg.Exotel.Enabled() {
		a.exotel = &telephony.Exotel{SID: cfg.Exotel.SID, APIKey: cfg.Exotel.APIKey, APIToken: cfg.Exotel.APIToken, Subdomain: cfg.Exotel.Subdomain,
			CallerID: cfg.Exotel.CallerID, AppID: cfg.Exotel.AppID}
		a.dispatcher.Exotel = a.exotel
	}
	if cfg.SMTP.Enabled() {
		a.dispatcher.Mailer = &notify.SMTPMailer{Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, User: cfg.SMTP.User, Password: cfg.SMTP.Password, From: cfg.SMTP.From}
	}
	if err := a.engine.SyncLadder(ctx, ladder); err != nil {
		st.Close()
		return nil, err
	}
	return a, nil
}

func ensureUser(ctx context.Context, st *store.Store, name, password string, role domain.Role, now time.Time) error {
	if !domain.Contains([]domain.Role{domain.RoleAdmin, domain.RolePartner, domain.RoleJudge, domain.RoleViewer}, role) {
		return fmt.Errorf("unknown role %q", role)
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	h, err := web.HashPassword(password)
	if err != nil {
		return err
	}
	return st.UpsertUser(ctx, domain.User{Username: name, PasswordHash: h, Role: role, CreatedAt: now})
}

func exportMetrics(ctx context.Context, a *app, out string) error {
	m, err := a.store.ComputeMetrics(ctx, true, time.Now().UTC())
	if err != nil {
		return err
	}
	n, agg, langs, err := metrics.ASR(ctx, a.store, true)
	if err != nil {
		return err
	}
	var wer *float64
	if agg != nil {
		wer = &agg.WER
	}
	md := metrics.Markdown(m, metrics.Targets(m, n, wer), a.engine.Cfg.BaseURL+" ("+string(a.engine.Cfg.Mode)+" mode)")
	md += metrics.ASRMarkdown(n, agg, langs)
	if out == "-" {
		_, err := os.Stdout.WriteString(md)
		return err
	}
	return os.WriteFile(out, []byte(md), 0o644)
}

func serve(ctx context.Context, cfg *config.Config, a *app) error {
	now := a.engine.Clock.Now()
	if cfg.AdminPassword != "" {
		if err := ensureUser(ctx, a.store, cfg.AdminUser, cfg.AdminPassword, domain.RoleAdmin, now); err != nil {
			return err
		}
	}
	if cfg.Demo() {
		pw := cfg.JudgePassword
		if pw == "" {
			pw = "water-judge-2026"
		}
		if err := ensureUser(ctx, a.store, cfg.JudgeUser, pw, domain.RoleJudge, now); err != nil {
			return err
		}
		wards, _ := a.store.ListWards(ctx)
		cs, _ := a.store.CountComplaints(ctx, store.ComplaintFilter{IncludeDuplicates: true})
		if cs == 0 || len(wards) == 0 {
			a.log.Info("seeding demo data")
			if err := demo.Seed(ctx, a.engine, a.dispatcher); err != nil {
				return fmt.Errorf("seed: %w", err)
			}
		}
	}
	sched := jobs.New(a.engine, a.dispatcher, a.log)
	srv := &web.Server{Engine: a.engine, Dispatcher: a.dispatcher, Scheduler: sched, Cfg: cfg, DemoClock: a.demoClock, WhatsApp: a.wa,
		Twilio: a.twilio, Exotel: a.exotel, Log: a.log, Version: version}
	if cfg.Demo() {
		srv.Seed = func(ctx context.Context) error { return demo.Seed(ctx, a.engine, a.dispatcher) }
	}
	bg, cancelBG := context.WithCancel(ctx)
	defer cancelBG()
	if err := srv.Start(bg, 8); err != nil {
		return err
	}
	var wg sync.WaitGroup
	sched.Start(bg, &wg)
	hs := &http.Server{Addr: cfg.Addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 16}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	a.log.Info("listening", "addr", cfg.Addr, "mode", cfg.Mode, "rule", a.engine.Rule.Version, "providers", cfg.Providers())
	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := hs.Shutdown(shutdown)
	cancelBG()
	srv.Stop()
	wg.Wait()
	return err
}
