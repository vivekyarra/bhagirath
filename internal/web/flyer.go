package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"rsc.io/qr"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

// flyer renders a printable A5 outreach flyer for a ward: missed-call number,
// WhatsApp number and a QR code to the report form pre-set to that ward.
func (s *Server) flyer(w http.ResponseWriter, r *http.Request) {
	wards, _ := s.Engine.Store.ListWards(r.Context())
	var ward domain.Ward
	id := r.URL.Query().Get("ward")
	for _, x := range wards {
		if x.ID == id || (id == "" && ward.ID == "") {
			ward = x
		}
	}
	partner := ward.PartnerName
	if partner == "" {
		partner = s.Cfg.PartnerName
	}
	s.render(w, r, "flyer", map[string]any{"Title": "Flyer", "Ward": ward, "Wards": wards, "Partner": partner,
		"MissedCall": s.Cfg.MissedCallNumber, "WhatsApp": s.Cfg.WhatsAppNumber, "ReportURL": s.reportURL(ward.ID)})
}

func (s *Server) reportURL(wardID string) string {
	u := s.Cfg.BaseURL + "/report"
	if wardID != "" {
		u += "?ward=" + url.QueryEscape(wardID)
	}
	return u
}

// qrSVG serves a QR code (SVG) for the report form of a ward. Only this
// deployment's own report URL can be encoded, so it cannot be abused to
// generate codes pointing elsewhere.
func (s *Server) qrSVG(w http.ResponseWriter, r *http.Request) {
	ward := r.URL.Query().Get("ward")
	if ward != "" {
		if _, err := s.Engine.Store.GetWard(r.Context(), ward); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	code, err := qr.Encode(s.reportURL(ward), qr.M)
	if err != nil {
		http.Error(w, "qr failed", http.StatusInternalServerError)
		return
	}
	const quiet = 4
	n := code.Size
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="#fff"/><path fill="#000" d="`, n+2*quiet, n+2*quiet)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(b.String()))
}
