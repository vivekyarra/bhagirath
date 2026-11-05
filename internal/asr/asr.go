// Package asr transcribes voice complaints. When no provider is configured
// (or a provider fails), the audio goes to the human transcription queue —
// the spec's volunteer fallback — and nothing is lost.
package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// ErrUnavailable means no machine transcription is possible; queue for a human.
var ErrUnavailable = errors.New("speech recognition unavailable")

// Transcript is a machine transcription.
type Transcript struct {
	Text     string `json:"text"`
	Language string `json:"language"` // ISO-639-1
	Provider string `json:"provider"`
}

// Transcriber converts audio to text.
type Transcriber interface {
	Transcribe(ctx context.Context, audio []byte, contentType, langHint string) (Transcript, error)
	Name() string
}

// None always defers to humans.
type None struct{}

// Transcribe implements Transcriber.
func (None) Transcribe(context.Context, []byte, string, string) (Transcript, error) {
	return Transcript{}, ErrUnavailable
}

// Name implements Transcriber.
func (None) Name() string { return "none" }

// Sarvam calls Sarvam AI's speech-to-text REST API (Saarika models; 10+ Indian
// languages with automatic language identification).
type Sarvam struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// Name implements Transcriber.
func (s *Sarvam) Name() string { return "sarvam:" + s.Model }

var sarvamCodes = map[string]string{"hi": "hi-IN", "en": "en-IN", "mr": "mr-IN", "te": "te-IN", "ta": "ta-IN", "bn": "bn-IN",
	"gu": "gu-IN", "kn": "kn-IN", "ml": "ml-IN", "pa": "pa-IN", "or": "od-IN"}

// Transcribe implements Transcriber.
func (s *Sarvam) Transcribe(ctx context.Context, audio []byte, contentType, langHint string) (Transcript, error) {
	if s.APIKey == "" {
		return Transcript{}, ErrUnavailable
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	ext := map[string]string{"audio/ogg": "ogg", "audio/mpeg": "mp3", "audio/wav": "wav", "audio/x-wav": "wav", "audio/mp4": "m4a", "audio/aac": "aac", "audio/webm": "webm"}[baseType(contentType)]
	if ext == "" {
		ext = "ogg"
	}
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="audio.%s"`, ext)}
	h["Content-Type"] = []string{contentType}
	fw, err := mw.CreatePart(h)
	if err != nil {
		return Transcript{}, err
	}
	if _, err := fw.Write(audio); err != nil {
		return Transcript{}, err
	}
	_ = mw.WriteField("model", s.Model)
	code := sarvamCodes[langHint]
	if code == "" {
		code = "unknown" // automatic language identification
	}
	_ = mw.WriteField("language_code", code)
	if err := mw.Close(); err != nil {
		return Transcript{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/speech-to-text", &body)
	if err != nil {
		return Transcript{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("api-subscription-key", s.APIKey)
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Transcript{}, fmt.Errorf("sarvam: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return Transcript{}, fmt.Errorf("sarvam: HTTP %d: %.200s", resp.StatusCode, raw)
	}
	var out struct {
		Transcript   string `json:"transcript"`
		LanguageCode string `json:"language_code"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Transcript{}, fmt.Errorf("sarvam: decode: %w", err)
	}
	lang := langHint
	for k, v := range sarvamCodes {
		if v == out.LanguageCode {
			lang = k
		}
	}
	return Transcript{Text: out.Transcript, Language: lang, Provider: s.Name()}, nil
}

func baseType(ct string) string {
	for i, r := range ct {
		if r == ';' {
			return ct[:i]
		}
	}
	return ct
}
