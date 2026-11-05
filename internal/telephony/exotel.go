package telephony

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Exotel is a client for Exotel's Call API (Indian virtual numbers).
//
// Missed-call intake: point the ExoPhone's "Passthru" applet at
// /voice/exotel/missed?key=<EXOTEL_WEBHOOK_KEY>; Bhagirath then places a
// free callback that runs the configured flow (EXOTEL_APP_ID), whose Passthru
// applets call /voice/exotel/* to record consent and the complaint. See
// docs/TELEPHONY.md for the flow layout.
type Exotel struct {
	SID       string
	APIKey    string
	APIToken  string
	Subdomain string // e.g. api.exotel.com or api.in.exotel.com
	CallerID  string // the ExoPhone
	AppID     string
	HTTP      *http.Client
	BaseURL   string // override for tests
}

// Connect places a call from the ExoPhone to `to` that runs the flow, passing
// customField through to every Passthru request.
func (e *Exotel) Connect(ctx context.Context, to, customField, statusCallback string) (string, error) {
	base := e.BaseURL
	if base == "" {
		base = "https://" + e.Subdomain
	}
	form := url.Values{
		"From":        {to},
		"CallerId":    {e.CallerID},
		"Url":         {fmt.Sprintf("http://my.exotel.com/%s/exoml/start_voice/%s", e.SID, e.AppID)},
		"CustomField": {customField},
		"CallType":    {"trans"},
	}
	if statusCallback != "" {
		form.Set("StatusCallback", statusCallback)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/Accounts/%s/Calls/connect.json", base, e.SID), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(e.APIKey, e.APIToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := e.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("exotel: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("exotel: HTTP %d: %.300s", resp.StatusCode, raw)
	}
	return extractSid(string(raw)), nil
}

func extractSid(s string) string {
	i := strings.Index(s, `"Sid"`)
	if i < 0 {
		return ""
	}
	rest := s[i+5:]
	a := strings.Index(rest, `"`)
	if a < 0 {
		return ""
	}
	rest = rest[a+1:]
	b := strings.Index(rest, `"`)
	if b < 0 {
		return ""
	}
	return rest[:b]
}

// ExotelPassthru is the subset of Passthru parameters Bhagirath uses.
type ExotelPassthru struct {
	CallSid      string
	From         string
	To           string
	Digits       string
	RecordingURL string
	CustomField  string
}

// ParseExotelPassthru reads Passthru query parameters (Exotel sends GET).
func ParseExotelPassthru(q url.Values) ExotelPassthru {
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := q.Get(k); v != "" {
				return v
			}
		}
		return ""
	}
	return ExotelPassthru{
		CallSid:      first("CallSid"),
		From:         first("CallFrom", "From"),
		To:           first("CallTo", "To"),
		Digits:       strings.Trim(first("digits", "Digits"), `"`),
		RecordingURL: first("RecordingUrl", "RecordingURL"),
		CustomField:  first("CustomField"),
	}
}
