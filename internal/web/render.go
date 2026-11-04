package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/extract"
	"github.com/vivekyarra/bhagirath/internal/i18n"
	"github.com/vivekyarra/bhagirath/internal/lang"
)

type templates struct {
	pages map[string]*template.Template
}

var funcs = template.FuncMap{
	"ist": func(t any) string {
		switch v := t.(type) {
		case time.Time:
			if v.IsZero() {
				return "—"
			}
			return v.In(clock.IST).Format("2 Jan, 3:04 PM")
		case *time.Time:
			if v == nil {
				return "—"
			}
			return v.In(clock.IST).Format("2 Jan, 3:04 PM")
		}
		return ""
	},
	"date":    func(t time.Time) string { return t.In(clock.IST).Format("2 Jan 2006") },
	"rfc3339": func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
	"issue":   func(it domain.IssueType) string { return extract.IssueLabel(it) },
	"issueIn": func(it domain.IssueType, l string) string { return i18n.Issue(string(it), l) },
	"label":   func(s any) string { return strings.ReplaceAll(fmt.Sprint(s), "_", " ") },
	"title": func(s any) string {
		x := strings.ReplaceAll(fmt.Sprint(s), "_", " ")
		if x == "" {
			return x
		}
		return strings.ToUpper(x[:1]) + x[1:]
	},
	"langName": func(code string) string {
		if n, ok := lang.Names[code]; ok {
			return n
		}
		return code
	},
	"t": func(key, l string) string { return i18n.T(key, l, nil) },
	"pct": func(n, d int) string {
		if d == 0 {
			return "—"
		}
		return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(d))
	},
	"f1": func(p *float64) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprintf("%.1f", *p)
	},
	"json": func(v any) template.JS {
		b, _ := json.Marshal(v)
		return template.JS(b)
	},
	"add": func(a, b int) int { return a + b },
	"hours": func(d time.Duration) string {
		h := d.Hours()
		if math.Abs(h) < 48 {
			return fmt.Sprintf("%.0f h", h)
		}
		return fmt.Sprintf("%.1f days", h/24)
	},
	"deref": func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	},
	"lower": strings.ToLower,
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return fmt.Sprintf("%d %s", n, one)
		}
		return fmt.Sprintf("%d %s", n, many)
	},
	"mul100": func(f float64) float64 { return 100 * f },
	"derefF": func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return *p
	},
	"yes":      func(b *bool) bool { return b != nil && *b },
	"list":     func(xs ...string) []string { return xs },
	"slice2":   func(a, b []domain.Complaint) [][]domain.Complaint { return [][]domain.Complaint{a, b} },
	"query":    url.QueryEscape,
	"contains": strings.Contains,
	"seq": func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	},
}

func loadTemplates() (*templates, error) {
	layout, err := templateFS.ReadFile("templates/layout.html")
	if err != nil {
		return nil, err
	}
	names, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	t := &templates{pages: map[string]*template.Template{}}
	for _, n := range names {
		base := strings.TrimSuffix(path.Base(n), ".html")
		if base == "layout" {
			continue
		}
		body, err := templateFS.ReadFile(n)
		if err != nil {
			return nil, err
		}
		tp, err := template.New("layout").Funcs(funcs).Parse(string(layout))
		if err != nil {
			return nil, err
		}
		if _, err := tp.Parse(string(body)); err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		t.pages[base] = tp
	}
	return t, nil
}

// pageData is available to every template as .
type pageData struct {
	Page    string
	Title   string
	User    string
	Role    domain.Role
	CanOp   bool
	CanSim  bool
	CSRF    string
	Demo    bool
	Now     time.Time
	Version string
	Lang    string
	Langs   []string
	Flash   string
	Data    map[string]any
	Path    string
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	tp, ok := s.tpl.pages[page]
	if !ok {
		http.Error(w, "unknown page "+page, http.StatusInternalServerError)
		return
	}
	sess, logged := s.readSession(r)
	pd := pageData{Page: page, Demo: s.Cfg.Demo(), Now: s.Engine.Clock.Now(), Version: s.Version, Data: data, Path: r.URL.Path,
		Lang: pickLang(r), Langs: lang.Supported, Flash: r.URL.Query().Get("ok")}
	if t, ok := data["Title"].(string); ok {
		pd.Title = t
	}
	if logged {
		pd.User, pd.Role, pd.CSRF = sess.User, sess.Role, s.csrfToken(sess)
		pd.CanOp, pd.CanSim = s.can(sess, "operate"), s.can(sess, "simulate")
	}
	var buf bytes.Buffer
	if err := tp.ExecuteTemplate(&buf, "layout", pd); err != nil {
		s.Log.Error("render", "page", page, "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = buf.WriteTo(w)
}

func pickLang(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); lang.IsSupported(l) {
		return l
	}
	if c, err := r.Cookie("bh_lang"); err == nil && lang.IsSupported(c.Value) {
		return c.Value
	}
	al := r.Header.Get("Accept-Language")
	for _, l := range lang.Supported {
		if strings.HasPrefix(al, l) {
			return l
		}
	}
	return "hi"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func urlQueryEscape(s string) string { return url.QueryEscape(s) }

func redirectOK(w http.ResponseWriter, r *http.Request, to, msg string) {
	sep := "?"
	if strings.Contains(to, "?") {
		sep = "&"
	}
	http.Redirect(w, r, to+sep+"ok="+url.QueryEscape(msg), http.StatusSeeOther)
}
