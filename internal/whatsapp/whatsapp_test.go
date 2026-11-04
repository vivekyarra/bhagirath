package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sample = `{"object":"whatsapp_business_account","entry":[{"id":"1","changes":[{"field":"messages","value":{
 "messaging_product":"whatsapp","metadata":{"display_phone_number":"15550000000","phone_number_id":"123"},
 "contacts":[{"profile":{"name":"Asha"},"wa_id":"910000000001"}],
 "messages":[
  {"from":"910000000001","id":"wamid.A","timestamp":"1762300000","type":"audio","audio":{"mime_type":"audio/ogg; codecs=opus","id":"MEDIA1"}},
  {"from":"910000000001","id":"wamid.B","timestamp":"1762300001","type":"text","text":{"body":"गंदा पानी"}},
  {"from":"910000000001","id":"wamid.C","timestamp":"1762300002","type":"location","location":{"latitude":22.72,"longitude":75.86,"name":"Shiv Mandir"}},
  {"from":"910000000001","id":"wamid.D","timestamp":"1762300003","type":"interactive","interactive":{"type":"button_reply","button_reply":{"id":"cb:42:yes","title":"Yes"}}},
  {"from":"910000000001","id":"wamid.E","timestamp":"1762300004","type":"sticker","sticker":{}}],
 "statuses":[{"id":"wamid.OUT","status":"delivered","timestamp":"1762300005","recipient_id":"910000000001"}]}}]}]}`

func TestParse(t *testing.T) {
	ins, sts, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(ins) != 5 || len(sts) != 1 {
		t.Fatalf("got %d msgs %d statuses", len(ins), len(sts))
	}
	if ins[0].Type != "audio" || ins[0].MediaID != "MEDIA1" || ins[0].Name != "Asha" || ins[0].At.Unix() != 1762300000 {
		t.Errorf("audio: %+v", ins[0])
	}
	if ins[1].Text != "गंदा पानी" || ins[2].Lat != 22.72 || ins[3].ReplyID != "cb:42:yes" || ins[4].Type != "other" {
		t.Errorf("parsed wrong: %+v", ins)
	}
	if sts[0].Status != "delivered" || sts[0].ID != "wamid.OUT" {
		t.Errorf("status: %+v", sts[0])
	}
	if _, _, err := Parse([]byte("{bad")); err == nil {
		t.Error("bad json accepted")
	}
}

func TestVerifySignature(t *testing.T) {
	body := []byte(sample)
	m := hmac.New(sha256.New, []byte("secret"))
	m.Write(body)
	sig := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if !VerifySignature("secret", body, sig) {
		t.Fatal("valid signature rejected")
	}
	if VerifySignature("secret", append(body, ' '), sig) || VerifySignature("other", body, sig) || VerifySignature("", body, sig) || VerifySignature("secret", body, "md5=x") {
		t.Fatal("invalid signature accepted")
	}
}

func TestSendAndMedia(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.URL.Path == "/v21.0/123/messages":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &got)
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.X"}]}`))
		case r.URL.Path == "/v21.0/MEDIA1":
			_, _ = w.Write([]byte(`{"url":"` + "http://" + r.Host + `/blob","mime_type":"audio/ogg","file_size":4}`))
		case r.URL.Path == "/blob":
			_, _ = w.Write([]byte("OGG!"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := &Client{Token: "tok", PhoneNumberID: "123", APIVersion: "v21.0", BaseURL: srv.URL}
	id, err := c.SendButtons(context.Background(), "+910000000001", "Fixed?", []Button{{ID: "cb:1:yes", Title: "Yes, fixed"}, {ID: "cb:1:no", Title: "No, still a problem here"}})
	if err != nil || id != "wamid.X" {
		t.Fatalf("send: %v %v", id, err)
	}
	if got["to"] != "910000000001" || got["type"] != "interactive" {
		t.Fatalf("payload: %v", got)
	}
	title := got["interactive"].(map[string]any)["action"].(map[string]any)["buttons"].([]any)[1].(map[string]any)["reply"].(map[string]any)["title"].(string)
	if len([]rune(title)) > 20 || !strings.HasSuffix(title, "…") {
		t.Fatalf("button title not truncated: %q", title)
	}
	data, mime, err := c.GetMedia(context.Background(), "MEDIA1")
	if err != nil || string(data) != "OGG!" || mime != "audio/ogg" {
		t.Fatalf("media: %q %q %v", data, mime, err)
	}
	bad := &Client{Token: "nope", PhoneNumberID: "123", APIVersion: "v21.0", BaseURL: srv.URL}
	if _, err := bad.SendText(context.Background(), "+91", "x"); err == nil {
		t.Fatal("401 not reported")
	}
}
