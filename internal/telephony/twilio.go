package telephony

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // Twilio's request-signing scheme is HMAC-SHA1.
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Twilio is a Programmable Voice client.
type Twilio struct {
	AccountSID string
	AuthToken  string
	From       string
	BaseURL    string // default https://api.twilio.com
	HTTP       *http.Client
}

func (t *Twilio) client() *http.Client {
	if t.HTTP != nil {
		return t.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (t *Twilio) base() string {
	if t.BaseURL != "" {
		return strings.TrimRight(t.BaseURL, "/")
	}
	return "https://api.twilio.com"
}

// Call places an outbound call that executes inline TwiML (≤ 4000 chars) or
// fetches it from twimlURL. It returns the call SID.
func (t *Twilio) Call(ctx context.Context, to, twiml, twimlURL, statusCallback string) (string, error) {
	form := url.Values{"To": {to}, "From": {t.From}}
	if twimlURL != "" {
		form.Set("Url", twimlURL)
	} else {
		if len(twiml) > 4000 {
			return "", fmt.Errorf("twilio: inline TwiML too long (%d chars)", len(twiml))
		}
		form.Set("Twiml", twiml)
	}
	if statusCallback != "" {
		form.Set("StatusCallback", statusCallback)
		form.Add("StatusCallbackEvent", "completed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/2010-04-01/Accounts/%s/Calls.json", t.base(), t.AccountSID), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(t.AccountSID, t.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("twilio: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("twilio: HTTP %d: %.300s", resp.StatusCode, raw)
	}
	var out struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.SID == "" {
		return "", fmt.Errorf("twilio: unexpected response %.200s", raw)
	}
	return out.SID, nil
}

// FetchRecording downloads a recording (WAV) with account credentials.
// Only api.twilio.com URLs are fetched, so a forged webhook cannot make the
// server request arbitrary hosts.
func (t *Twilio) FetchRecording(ctx context.Context, recordingURL string) ([]byte, string, error) {
	u, err := url.Parse(recordingURL)
	if err != nil || u.Scheme != "https" || (u.Host != "api.twilio.com" && !strings.HasPrefix(recordingURL, t.base())) {
		return nil, "", fmt.Errorf("twilio: refusing to fetch recording from %q", recordingURL)
	}
	if !strings.HasSuffix(u.Path, ".wav") && !strings.HasSuffix(u.Path, ".mp3") {
		u.Path += ".wav"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.SetBasicAuth(t.AccountSID, t.AuthToken)
	resp, err := t.client().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("twilio recording: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "audio/wav"
	}
	return data, ct, err
}

// ValidateTwilioSignature verifies X-Twilio-Signature: base64(HMAC-SHA1(
// authToken, fullURL + sorted(key+value of POST params))).
func ValidateTwilioSignature(authToken, fullURL string, params url.Values, signature string) bool {
	if authToken == "" || signature == "" {
		return false
	}
	return hmac.Equal([]byte(TwilioSignature(authToken, fullURL, params)), []byte(signature))
}

// TwilioSignature computes the expected signature (also used by tests and
// the demo simulator).
func TwilioSignature(authToken, fullURL string, params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(fullURL)
	for _, k := range keys {
		vs := append([]string(nil), params[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			b.WriteString(k)
			b.WriteString(v)
		}
	}
	m := hmac.New(sha1.New, []byte(authToken))
	m.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}
