// Package web serves the officials/partner dashboard, the public report form
// and impact page, the officials' evidence-pack links, and the provider
// webhooks (WhatsApp, Twilio, Exotel). Pages are server-rendered Go templates
// so they load fast on 2G phones; JavaScript is only used for the map and
// live countdown timers.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/config"
	"github.com/vivekyarra/bhagirath/internal/jobs"
	"github.com/vivekyarra/bhagirath/internal/notify"
	"github.com/vivekyarra/bhagirath/internal/secure"
	"github.com/vivekyarra/bhagirath/internal/telephony"
	"github.com/vivekyarra/bhagirath/internal/whatsapp"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server is the HTTP application.
type Server struct {
	Engine     *agents.Engine
	Dispatcher *notify.Dispatcher
	Scheduler  *jobs.Scheduler
	Cfg        *config.Config
	DemoClock  *clock.Offset // non-nil in demo mode
	WhatsApp   *whatsapp.Client
	Twilio     *telephony.Twilio
	Exotel     *telephony.Exotel
	Log        *slog.Logger
	Version    string
	Seed       func(ctx context.Context) error // demo reset

	tpl     *templates
	work    chan func(context.Context)
	wg      sync.WaitGroup
	limiter *rateLimiter
	impact  impactCache
	senders [64]sync.Mutex // per-sender ordering of chat messages
}

// senderLock serialises processing of messages from the same phone so a
// location pin and the complaint text that follows are handled in order.
func (s *Server) senderLock(phone string) *sync.Mutex {
	h := fnv.New32a()
	h.Write([]byte(phone))
	return &s.senders[h.Sum32()%uint32(len(s.senders))]
}

// Start prepares templates and background workers for webhook processing.
func (s *Server) Start(ctx context.Context, workers int) error {
	t, err := loadTemplates()
	if err != nil {
		return err
	}
	s.tpl = t
	s.limiter = newRateLimiter()
	s.work = make(chan func(context.Context), 1024)
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for f := range s.work {
				func() {
					defer func() {
						if r := recover(); r != nil {
							s.Log.Error("background task panicked", "panic", r, "stack", string(debug.Stack()))
						}
					}()
					tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
					defer cancel()
					f(tctx)
				}()
			}
		}()
	}
	return nil
}

// Stop drains background work.
func (s *Server) Stop() {
	close(s.work)
	s.wg.Wait()
}

// enqueue schedules webhook processing so providers get a fast 200.
func (s *Server) enqueue(f func(context.Context)) bool {
	select {
	case s.work <- f:
		return true
	default:
		return false
	}
}

// Handler builds the router with middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheStatic(http.FileServerFS(static))))
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.healthz)

	// Public
	mux.HandleFunc("GET /{$}", s.landing)
	mux.HandleFunc("GET /report", s.reportForm)
	mux.HandleFunc("POST /report", s.limit("report", 10, s.reportSubmit))
	mux.HandleFunc("GET /impact", s.impactPage)
	mux.HandleFunc("GET /flyer", s.flyer)
	mux.HandleFunc("GET /qr.svg", s.qrSVG)
	mux.HandleFunc("GET /api/v1/impact", s.impactJSON)
	mux.HandleFunc("GET /o/{token}", s.officialPage)
	mux.HandleFunc("POST /o/{token}/ack", s.limit("official", 30, s.officialAck))
	mux.HandleFunc("POST /o/{token}/fixed", s.limit("official", 30, s.officialFixed))

	// Auth
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.limit("login", 10, s.loginSubmit))
	mux.HandleFunc("POST /logout", s.logout)

	// Dashboard
	mux.HandleFunc("GET /app", s.auth("read", s.overview))
	mux.HandleFunc("GET /app/alerts/{id}", s.auth("read", s.alertPage))
	mux.HandleFunc("POST /app/alerts/{id}/approve", s.auth("operate", s.alertApprove))
	mux.HandleFunc("GET /app/complaints", s.auth("read", s.complaintsPage))
	mux.HandleFunc("GET /app/complaints/{ref}", s.auth("read", s.complaintPage))
	mux.HandleFunc("POST /app/complaints/{ref}/update", s.auth("operate", s.complaintUpdate))
	mux.HandleFunc("POST /app/complaints/{ref}/claim", s.auth("operate", s.complaintClaim))
	mux.HandleFunc("POST /app/closures/{id}/verify", s.auth("operate", s.closureVerify))
	mux.HandleFunc("GET /app/escalations", s.auth("read", s.escalationsPage))
	mux.HandleFunc("GET /app/escalations/{ref}", s.auth("read", s.escalationPage))
	mux.HandleFunc("POST /app/escalations/{ref}/ack", s.auth("operate", s.escalationAck))
	mux.HandleFunc("GET /app/review", s.auth("read", s.reviewPage))
	mux.HandleFunc("POST /app/review/{ref}", s.auth("operate", s.reviewSubmit))
	mux.HandleFunc("GET /app/volunteer", s.auth("operate", s.volunteerForm))
	mux.HandleFunc("POST /app/volunteer", s.auth("operate", s.volunteerSubmit))
	mux.HandleFunc("POST /app/volunteer/import", s.auth("operate", s.manualImport))
	mux.HandleFunc("GET /app/signals", s.auth("read", s.signalsPage))
	mux.HandleFunc("POST /app/signals/test", s.auth("operate", s.addTest))
	mux.HandleFunc("POST /app/signals/tests.csv", s.auth("operate", s.importTests))
	mux.HandleFunc("POST /app/signals/illness", s.auth("operate", s.addIllness))
	mux.HandleFunc("POST /app/signals/reporter", s.auth("operate", s.addSignalReporter))
	mux.HandleFunc("GET /app/outbox", s.auth("read", s.outboxPage))
	mux.HandleFunc("POST /app/outbox/{id}/answer", s.auth("simulate", s.outboxAnswer))
	mux.HandleFunc("GET /app/pilot", s.auth("read", s.pilotPage))
	mux.HandleFunc("POST /app/pilot/outreach", s.auth("operate", s.addOutreach))
	mux.HandleFunc("POST /app/pilot/baseline", s.auth("operate", s.addBaseline))
	mux.HandleFunc("GET /app/pilot/METRICS.md", s.auth("read", s.metricsMarkdown))
	mux.HandleFunc("GET /app/settings", s.auth("read", s.settingsPage))
	mux.HandleFunc("GET /app/media/{id}", s.auth("read", s.media))
	mux.HandleFunc("GET /api/v1/map", s.auth("read", s.mapJSON))
	mux.HandleFunc("GET /api/v1/events", s.auth("read", s.eventsJSON))

	// Demo simulator
	mux.HandleFunc("GET /app/demo", s.auth("simulate", s.demoPage))
	mux.HandleFunc("POST /app/demo/{action}", s.auth("simulate", s.demoAction))

	// Provider webhooks
	mux.HandleFunc("GET /webhooks/whatsapp", s.whatsappVerify)
	mux.HandleFunc("POST /webhooks/whatsapp", s.whatsappWebhook)
	mux.HandleFunc("POST /voice/twilio/incoming", s.twilioIncoming)
	mux.HandleFunc("POST /voice/twilio/ivr", s.twilioIVR)
	mux.HandleFunc("POST /voice/twilio/callback", s.twilioCallbackAnswer)
	mux.HandleFunc("POST /voice/twilio/status", s.twilioStatus)
	mux.HandleFunc("GET /voice/exotel/{step}", s.exotel)

	return s.recoverer(s.logRequests(securityHeaders(mux)))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.Engine.Store.Ping(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "ok %s rule=%s\n", s.Version, s.Engine.Rule.Version)
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(w, r)
	})
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "same-origin")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Permissions-Policy", "camera=(self), microphone=(self), geolocation=(self)")
		hd.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https://*.tile.openstreetmap.org; "+
			"script-src 'self'; style-src 'self' 'unsafe-inline'; media-src 'self' blob:; "+
			"connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			hd.Set("Strict-Transport-Security", "max-age=31536000")
		}
		h.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

func (s *Server) logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz" {
			return
		}
		// Paths only: query strings can carry tokens, so they are not logged.
		path := r.URL.Path
		if strings.HasPrefix(path, "/o/") {
			path = "/o/…"
		}
		s.Log.Info("http", "method", r.Method, "path", path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				s.Log.Error("panic", "panic", v, "stack", string(debug.Stack()))
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

// retry runs f up to 4 times with backoff. Every webhook path is idempotent
// (the provider message id is recorded in the same transaction as its
// effect), so a retry after a transient failure — a dropped database
// connection, a failover, a provider timeout — can never double-count.
func (s *Server) retry(ctx context.Context, what string, f func() error) {
	wait := 500 * time.Millisecond
	for attempt := 1; ; attempt++ {
		err := f()
		if err == nil || permanent(err) {
			if err != nil {
				s.Log.Warn(what+" rejected", "err", err)
			}
			return
		}
		if attempt == 4 {
			s.Log.Error(what+" failed after retries", "err", err)
			return
		}
		s.Log.Warn(what+" failed; retrying", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait *= 3
	}
}

func permanent(err error) bool {
	return errors.Is(err, agents.ErrEmpty) || errors.Is(err, secure.ErrInvalidPhone)
}
