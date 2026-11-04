// Package whatsapp is a client for the Meta WhatsApp Cloud API: webhook
// verification and parsing, message sending and media download.
package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client sends messages from one business phone number.
type Client struct {
	Token         string
	PhoneNumberID string
	APIVersion    string
	BaseURL       string // default https://graph.facebook.com
	HTTP          *http.Client
}

// MaxMediaBytes bounds downloaded voice notes/photos.
const MaxMediaBytes = 16 << 20

func (c *Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://graph.facebook.com"
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Button is an interactive reply button (max 3, title ≤ 20 chars).
type Button struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// SendText sends a plain text message and returns the message id.
func (c *Client) SendText(ctx context.Context, to, body string) (string, error) {
	return c.send(ctx, map[string]any{
		"messaging_product": "whatsapp", "recipient_type": "individual", "to": strings.TrimPrefix(to, "+"),
		"type": "text", "text": map[string]any{"body": truncate(body, 4096), "preview_url": true},
	})
}

// SendButtons sends an interactive message with reply buttons.
func (c *Client) SendButtons(ctx context.Context, to, body string, buttons []Button) (string, error) {
	var bs []map[string]any
	for _, b := range buttons {
		bs = append(bs, map[string]any{"type": "reply", "reply": map[string]string{"id": b.ID, "title": truncate(b.Title, 20)}})
	}
	return c.send(ctx, map[string]any{
		"messaging_product": "whatsapp", "recipient_type": "individual", "to": strings.TrimPrefix(to, "+"), "type": "interactive",
		"interactive": map[string]any{"type": "button", "body": map[string]string{"text": truncate(body, 1024)}, "action": map[string]any{"buttons": bs}},
	})
}

// SendTemplate sends a pre-approved template (required outside the 24-hour
// customer-service window) with positional body parameters.
func (c *Client) SendTemplate(ctx context.Context, to, name, language string, params []string) (string, error) {
	var ps []map[string]string
	for _, p := range params {
		ps = append(ps, map[string]string{"type": "text", "text": truncate(p, 1024)})
	}
	tpl := map[string]any{"name": name, "language": map[string]string{"code": language}}
	if len(ps) > 0 {
		tpl["components"] = []map[string]any{{"type": "body", "parameters": ps}}
	}
	return c.send(ctx, map[string]any{"messaging_product": "whatsapp", "to": strings.TrimPrefix(to, "+"), "type": "template", "template": tpl})
}

func (c *Client) send(ctx context.Context, payload map[string]any) (string, error) {
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/%s/%s/messages", c.base(), c.APIVersion, c.PhoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("whatsapp: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("whatsapp: HTTP %d: %.300s", resp.StatusCode, raw)
	}
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Messages) == 0 {
		return "", fmt.Errorf("whatsapp: unexpected response %.200s", raw)
	}
	return out.Messages[0].ID, nil
}

// GetMedia downloads a media object (voice note, image) by id.
func (c *Client) GetMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s/%s", c.base(), c.APIVersion, mediaID), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("whatsapp media lookup: HTTP %d", resp.StatusCode)
	}
	var meta struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
		FileSize int    `json:"file_size"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&meta); err != nil {
		return nil, "", err
	}
	if meta.FileSize > MaxMediaBytes {
		return nil, "", fmt.Errorf("whatsapp media too large: %d bytes", meta.FileSize)
	}
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.URL, nil)
	if err != nil {
		return nil, "", err
	}
	req2.Header.Set("Authorization", "Bearer "+c.Token)
	resp2, err := c.http().Do(req2)
	if err != nil {
		return nil, "", err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("whatsapp media download: HTTP %d", resp2.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp2.Body, MaxMediaBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > MaxMediaBytes {
		return nil, "", fmt.Errorf("whatsapp media too large")
	}
	return data, meta.MimeType, nil
}

// VerifySignature checks the X-Hub-Signature-256 header ("sha256=<hex>").
func VerifySignature(appSecret string, body []byte, header string) bool {
	if appSecret == "" || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	m := hmac.New(sha256.New, []byte(appSecret))
	m.Write(body)
	return hmac.Equal(got, m.Sum(nil))
}

// Inbound is one message received from a user.
type Inbound struct {
	ID        string
	From      string // E.164 without "+"
	Name      string
	At        time.Time
	Type      string // text | audio | location | interactive | button | image | other
	Text      string
	MediaID   string
	MediaMime string
	Lat, Lon  float64
	ReplyID   string // interactive button id
}

// Status is a delivery receipt for a message we sent.
type Status struct {
	ID     string
	Status string // sent | delivered | read | failed
	At     time.Time
}

type webhook struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Messages []struct {
					ID        string `json:"id"`
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
					Audio struct {
						ID       string `json:"id"`
						MimeType string `json:"mime_type"`
					} `json:"audio"`
					Image struct {
						ID       string `json:"id"`
						MimeType string `json:"mime_type"`
						Caption  string `json:"caption"`
					} `json:"image"`
					Location struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
						Name      string  `json:"name"`
						Address   string  `json:"address"`
					} `json:"location"`
					Interactive struct {
						Type        string `json:"type"`
						ButtonReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"button_reply"`
					} `json:"interactive"`
					Button struct {
						Payload string `json:"payload"`
						Text    string `json:"text"`
					} `json:"button"`
				} `json:"messages"`
				Statuses []struct {
					ID        string `json:"id"`
					Status    string `json:"status"`
					Timestamp string `json:"timestamp"`
				} `json:"statuses"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// Parse decodes a webhook body into messages and delivery statuses.
func Parse(body []byte) ([]Inbound, []Status, error) {
	var w webhook
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, nil, err
	}
	var ins []Inbound
	var sts []Status
	for _, e := range w.Entry {
		for _, ch := range e.Changes {
			names := map[string]string{}
			for _, c := range ch.Value.Contacts {
				names[c.WaID] = c.Profile.Name
			}
			for _, m := range ch.Value.Messages {
				in := Inbound{ID: m.ID, From: m.From, Name: names[m.From], At: unix(m.Timestamp), Type: m.Type}
				switch m.Type {
				case "text":
					in.Text = m.Text.Body
				case "audio":
					in.MediaID, in.MediaMime = m.Audio.ID, m.Audio.MimeType
				case "image":
					in.MediaID, in.MediaMime, in.Text = m.Image.ID, m.Image.MimeType, m.Image.Caption
				case "location":
					in.Lat, in.Lon = m.Location.Latitude, m.Location.Longitude
					in.Text = strings.TrimSpace(m.Location.Name + " " + m.Location.Address)
				case "interactive":
					in.ReplyID, in.Text = m.Interactive.ButtonReply.ID, m.Interactive.ButtonReply.Title
				case "button":
					in.ReplyID, in.Text = m.Button.Payload, m.Button.Text
				default:
					in.Type = "other"
				}
				ins = append(ins, in)
			}
			for _, s := range ch.Value.Statuses {
				sts = append(sts, Status{ID: s.ID, Status: s.Status, At: unix(s.Timestamp)})
			}
		}
	}
	return ins, sts, nil
}

func unix(s string) time.Time {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
