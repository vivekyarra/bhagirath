package web

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

const sessionCookie = "bh_session"
const sessionTTL = 12 * time.Hour

type session struct {
	User string
	Role domain.Role
	Exp  time.Time
}

type ctxKey struct{}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, u domain.User) {
	exp := time.Now().Add(sessionTTL)
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%s|%d", u.Username, u.Role, exp.Unix())))
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: payload + "." + s.Engine.Vault.Sign("session:"+payload), Path: "/",
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode, Expires: exp})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func (s *Server) readSession(r *http.Request) (session, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return session{}, false
	}
	payload, sig, ok := strings.Cut(c.Value, ".")
	if !ok || !s.Engine.Vault.Verify("session:"+payload, sig) {
		return session{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return session{}, false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return session{}, false
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return session{}, false
	}
	return session{User: parts[0], Role: domain.Role(parts[1]), Exp: time.Unix(exp, 0)}, true
}

func currentSession(ctx context.Context) session {
	s, _ := ctx.Value(ctxKey{}).(session)
	return s
}

// can applies role permissions. In demo deployments the judge account may
// also operate (approve advisories, review transcripts) on fictional data.
func (s *Server) can(sess session, action string) bool {
	if sess.Role.Can(action) {
		return true
	}
	return s.Cfg.Demo() && sess.Role == domain.RoleJudge && action == "operate"
}

// auth guards dashboard routes and enforces CSRF on state-changing requests.
func (s *Server) auth(action string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.readSession(r)
		if !ok {
			if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") {
				http.Redirect(w, r, "/login?next="+urlQueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
				return
			}
			http.Error(w, "login required", http.StatusUnauthorized)
			return
		}
		if !s.can(sess, action) {
			http.Error(w, "your role cannot do this", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !s.validCSRF(r, sess) {
				http.Error(w, "invalid or missing form token; reload the page and try again", http.StatusForbidden)
				return
			}
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	}
}

// csrfToken binds a token to the session (stateless double-submit).
func (s *Server) csrfToken(sess session) string {
	return s.Engine.Vault.Sign(fmt.Sprintf("csrf:%s:%d", sess.User, sess.Exp.Unix()))
}

func (s *Server) validCSRF(r *http.Request, sess session) bool {
	tok := r.Header.Get("X-CSRF-Token")
	if tok == "" {
		if err := r.ParseMultipartForm(16 << 20); err != nil && err != http.ErrNotMultipart {
			return false
		}
		tok = r.FormValue("csrf")
	}
	return tok != "" && hmac.Equal([]byte(tok), []byte(s.csrfToken(sess)))
}

// publicCSRF protects unauthenticated forms (report, official actions) with a
// short-lived token signed over the form name and an hour bucket.
func (s *Server) publicCSRF(form string) string {
	return s.Engine.Vault.Sign(fmt.Sprintf("pub:%s:%d", form, time.Now().Unix()/3600))
}

func (s *Server) validPublicCSRF(form, tok string) bool {
	now := time.Now().Unix() / 3600
	for _, b := range []int64{now, now - 1} {
		if hmac.Equal([]byte(tok), []byte(s.Engine.Vault.Sign(fmt.Sprintf("pub:%s:%d", form, b)))) {
			return true
		}
	}
	return false
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login", map[string]any{"Next": safeNext(r.URL.Query().Get("next")), "CSRF": s.publicCSRF("login")})
}

func (s *Server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.FormValue("next"))
	if !s.validPublicCSRF("login", r.FormValue("csrf")) {
		s.render(w, r, "login", map[string]any{"Error": "The form expired. Please try again.", "Next": next, "CSRF": s.publicCSRF("login")})
		return
	}
	u, err := s.Engine.Store.GetUser(r.Context(), strings.TrimSpace(r.FormValue("username")))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(r.FormValue("password"))) != nil {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, r, "login", map[string]any{"Error": "Wrong username or password.", "Next": next, "CSRF": s.publicCSRF("login")})
		return
	}
	s.issueSession(w, r, u)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func safeNext(n string) string {
	if !strings.HasPrefix(n, "/") || strings.HasPrefix(n, "//") || strings.Contains(n, "\\") {
		return "/app"
	}
	return n
}

// rateLimiter is a small fixed-window limiter keyed by client IP and bucket.
type rateLimiter struct {
	mu   sync.Mutex
	hits map[string]int
	win  int64
}

func newRateLimiter() *rateLimiter { return &rateLimiter{hits: map[string]int{}} }

func (l *rateLimiter) allow(key string, perMinute int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := time.Now().Unix() / 60
	if w != l.win {
		l.win, l.hits = w, map[string]int{}
	}
	l.hits[key]++
	return l.hits[key] <= perMinute
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) limit(bucket string, perMinute int, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.allow(bucket+"|"+clientIP(r), perMinute) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "too many requests; please wait a minute", http.StatusTooManyRequests)
			return
		}
		h(w, r)
	}
}

// BcryptCost is the password hashing cost (lowered only in tests).
var BcryptCost = 12

// HashPassword hashes a password for storage.
func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), BcryptCost)
	return string(b), err
}
