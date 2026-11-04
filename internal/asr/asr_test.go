package asr

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	e := Compare("गंदा पानी आ रहा है", "गंदा पानी आ रहे है")
	if e.RefWords != 5 || e.WordEdits != 1 || math.Abs(e.WER-0.2) > 1e-9 {
		t.Fatalf("%+v", e)
	}
	if e := Compare("same text", "Same, text!"); e.WER != 0 || e.CER != 0 {
		t.Fatalf("normalisation: %+v", e)
	}
	if e := Compare("a b c", ""); e.WER != 1 {
		t.Fatalf("deletion: %+v", e)
	}
	agg := Aggregate([]ErrorRates{Compare("a b", "a x"), Compare("c d e f", "c d e f")})
	if agg.RefWords != 6 || agg.WordEdits != 1 {
		t.Fatalf("aggregate %+v", agg)
	}
}

func TestSarvamClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-subscription-key") != "k" || r.URL.Path != "/speech-to-text" {
			w.WriteHeader(401)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			w.WriteHeader(400)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(400)
			return
		}
		b, _ := io.ReadAll(f)
		if string(b) != "OGGDATA" || r.FormValue("language_code") != "unknown" || r.FormValue("model") != "saarika:v2.5" {
			w.WriteHeader(422)
			return
		}
		_, _ = w.Write([]byte(`{"request_id":"x","transcript":"गंदा पानी","language_code":"hi-IN"}`))
	}))
	defer srv.Close()
	s := &Sarvam{APIKey: "k", Model: "saarika:v2.5", BaseURL: srv.URL}
	tr, err := s.Transcribe(context.Background(), []byte("OGGDATA"), "audio/ogg; codecs=opus", "")
	if err != nil || tr.Text != "गंदा पानी" || tr.Language != "hi" {
		t.Fatalf("%+v %v", tr, err)
	}
	bad := &Sarvam{APIKey: "wrong", Model: "m", BaseURL: srv.URL}
	if _, err := bad.Transcribe(context.Background(), []byte("x"), "audio/ogg", "hi"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401, got %v", err)
	}
	if _, err := (None{}).Transcribe(context.Background(), nil, "", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatal("None must be unavailable")
	}
}
