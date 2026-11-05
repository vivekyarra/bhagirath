package telephony

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTwiMLEscapes(t *testing.T) {
	doc := (&TwiML{}).Add(
		Say{Voice: "Polly.Aditi", Language: "hi-IN", Text: `गंदा पानी <script>& "x"`},
		Gather{Action: "/voice/x?a=1&b=2", NumDigits: 1, Timeout: 8, Says: []Say{{Text: "Press 1"}}},
		Record{Action: "/r", MaxLength: 120, FinishOnKey: "#", PlayBeep: true},
		Hangup{},
	).String()
	for _, want := range []string{`<Say voice="Polly.Aditi" language="hi-IN">गंदा पानी &lt;script&gt;&amp; &#34;x&#34;</Say>`,
		`<Gather action="/voice/x?a=1&amp;b=2" numDigits="1" timeout="8"><Say>Press 1</Say></Gather>`,
		`<Record action="/r" maxLength="120" finishOnKey="#" playBeep="true"></Record>`, `<Hangup></Hangup>`} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s in\n%s", want, doc)
		}
	}
	if Digits("2") != 2 || Digits("") != 0 || Digits("x") != 0 {
		t.Error("Digits")
	}
}

func TestTwilioSignatureKnownVector(t *testing.T) {
	// Example from Twilio's security documentation.
	params := url.Values{"CallSid": {"CA1234567890ABCDE"}, "Caller": {"+12349013030"}, "Digits": {"1234"}, "From": {"+12349013030"}, "To": {"+18005551212"}}
	u := "https://mycompany.com/myapp.php?foo=1&bar=2"
	sig := TwilioSignature("12345", u, params)
	if sig != "0/KCTR6DLpKmkAf8muzZqo1nDgQ=" {
		t.Fatalf("signature = %s", sig)
	}
	if !ValidateTwilioSignature("12345", u, params, sig) || ValidateTwilioSignature("12345", u+"x", params, sig) || ValidateTwilioSignature("", u, params, sig) {
		t.Fatal("validation wrong")
	}
}

func TestTwilioCallAndRecordingGuard(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		if u != "AC1" || p != "tok" {
			w.WriteHeader(401)
			return
		}
		_ = r.ParseForm()
		form = r.PostForm
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"sid":"CA9"}`))
	}))
	defer srv.Close()
	tw := &Twilio{AccountSID: "AC1", AuthToken: "tok", From: "+15550001111", BaseURL: srv.URL}
	sid, err := tw.Call(context.Background(), "+910000000001", "<Response/>", "", "")
	if err != nil || sid != "CA9" || form.Get("To") != "+910000000001" || form.Get("Twiml") != "<Response/>" {
		t.Fatalf("call: %v %v %v", sid, err, form)
	}
	if _, _, err := tw.FetchRecording(context.Background(), "https://evil.example.com/rec"); err == nil {
		t.Fatal("fetched recording from foreign host (SSRF)")
	}
	if _, err := tw.Call(context.Background(), "+91", strings.Repeat("x", 4001), "", ""); err == nil {
		t.Fatal("oversized TwiML accepted")
	}
}

func TestExotel(t *testing.T) {
	q, _ := url.ParseQuery(`CallSid=abc&CallFrom=09876543210&CallTo=08000000000&digits="1"&RecordingUrl=https%3A%2F%2Fs3.exotel%2Fr.mp3&CustomField=closure%3A5`)
	p := ParseExotelPassthru(q)
	if p.CallSid != "abc" || p.From != "09876543210" || p.Digits != "1" || p.CustomField != "closure:5" || p.RecordingURL != "https://s3.exotel/r.mp3" {
		t.Fatalf("%+v", p)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Call":{"Sid":"exo123","Status":"in-progress"}}`))
	}))
	defer srv.Close()
	e := &Exotel{SID: "s", APIKey: "k", APIToken: "t", CallerID: "080", AppID: "9", BaseURL: srv.URL}
	sid, err := e.Connect(context.Background(), "+910000000001", "x", "")
	if err != nil || sid != "exo123" {
		t.Fatalf("connect: %v %v", sid, err)
	}
}
