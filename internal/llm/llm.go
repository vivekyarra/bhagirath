// Package llm is a minimal client for chat-completion style models, used only
// for extraction and translation of complaint text. Alert decisions never
// depend on it (see package rule).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client completes a prompt and returns plain text.
type Client interface {
	Complete(ctx context.Context, system, user string) (string, error)
	Name() string
}

// Anthropic calls the Anthropic Messages API.
type Anthropic struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// Name implements Client.
func (a *Anthropic) Name() string { return "anthropic:" + a.Model }

// Complete implements Client.
func (a *Anthropic) Complete(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      a.Model,
		"max_tokens": 600,
		"system":     system,
		"messages":   []map[string]string{{"role": "user", "content": user}},
	})
	base := a.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	raw, err := post(ctx, a.HTTP, base+"/v1/messages", body, map[string]string{
		"x-api-key": a.APIKey, "anthropic-version": "2023-06-01",
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String(), nil
}

// OpenAICompatible calls any /chat/completions endpoint (OpenAI, Sarvam,
// a gateway provided by the hackathon IDE, a local model server, ...).
type OpenAICompatible struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// Name implements Client.
func (o *OpenAICompatible) Name() string { return "openai:" + o.Model }

// Complete implements Client.
func (o *OpenAICompatible) Complete(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       o.Model,
		"temperature": 0,
		"messages":    []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
	})
	base := strings.TrimRight(o.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	raw, err := post(ctx, o.HTTP, base+"/chat/completions", body, map[string]string{"Authorization": "Bearer " + o.APIKey})
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm: empty response")
	}
	return out.Choices[0].Message.Content, nil
}

func post(ctx context.Context, client *http.Client, url string, body []byte, headers map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("llm: HTTP %d: %.200s", resp.StatusCode, raw)
	}
	return raw, nil
}

// JSONObject extracts the first {...} object from model output (models
// sometimes wrap JSON in prose or code fences).
func JSONObject(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
