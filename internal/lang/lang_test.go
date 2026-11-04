package lang

import "testing"

func TestDetect(t *testing.T) {
	cases := map[string]string{
		"हमारे घर में दो दिन से गंदा पानी आ रहा है":              "hi",
		"आमच्या घरी गढूळ पाणी येत आहे, खूप घाण वास":              "mr",
		"మా ఇంట్లో మురికి నీరు వస్తోంది":                         "te",
		"எங்கள் வீட்டில் அழுக்கு நீர் வருகிறது":                  "ta",
		"আমাদের বাড়িতে নোংরা জল আসছে":                           "bn",
		"Dirty yellow water is coming from the tap since Monday": "en",
		"hamare ghar me ganda pani aa raha hai":                  "hi",
		"Paani me badbu aa rahi hai, bacche bimar":               "hi",
		"":         "",
		"12345 !!": "",
	}
	for in, want := range cases {
		if got := Detect(in); got != want {
			t.Errorf("Detect(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize("  Ganda   PAANI!!! (since Monday)  "); got != "ganda paani since monday" {
		t.Fatalf("got %q", got)
	}
	if got := Normalize("गंदा पानी, बदबू।"); got != "गंदा पानी बदबू" {
		t.Fatalf("got %q", got)
	}
}

func TestPick(t *testing.T) {
	if Pick("ta", "hi") != "hi" || Pick("te", "hi") != "te" || Pick("xx", "yy") != "en" {
		t.Fatal("Pick fallback wrong")
	}
}
