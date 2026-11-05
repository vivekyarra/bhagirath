package extract

import (
	"context"
	"testing"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

func TestRulesExtractMultilingual(t *testing.T) {
	cases := []struct {
		text string
		want domain.IssueType
		ill  bool
		lang string
	}{
		{"हमारे घर में दो दिन से गंदा पानी आ रहा है", domain.IssueDirtyWater, false, "hi"},
		{"नल से बदबूदार पानी आ रहा है, बच्चों को उल्टी दस्त हो रहे हैं", domain.IssueFoulSmell, true, "hi"},
		{"नाली का पानी पीने के पानी में मिल रहा है", domain.IssueSewageMixing, false, "hi"},
		{"pani me badbu aa rahi hai aur nali ka pani mix ho raha hai", domain.IssueSewageMixing, false, "hi"},
		{"Yellow muddy water from the tap since Monday, my son has loose motions", domain.IssueDirtyWater, true, "en"},
		{"Three days no water supply in our lane", domain.IssueNoWater, false, "en"},
		{"Pipeline burst near the school, water leaking on road", domain.IssueLeak, false, "en"},
		{"Pipe leakage is letting drain water into the supply", domain.IssueSewageMixing, false, "en"},
		{"आमच्या घरी गढूळ पाणी येत आहे", domain.IssueDirtyWater, false, "mr"},
		{"మా ఇంట్లో మురికి నీరు వస్తోంది, పిల్లలకు వాంతులు", domain.IssueDirtyWater, true, "te"},
		{"எங்கள் தெருவில் சாக்கடை நீர் கலக்கிறது", domain.IssueSewageMixing, false, "ta"},
		{"घर में सब बीमार हैं, दस्त लग रहे हैं", domain.IssueIllness, true, "hi"},
		{"hello I want to talk", domain.IssueOther, false, "en"},
	}
	for _, c := range cases {
		r, err := Rules{}.Extract(context.Background(), c.text, "")
		if err != nil {
			t.Fatal(err)
		}
		if r.IssueType != c.want || r.MentionsIllness != c.ill || r.Language != c.lang {
			t.Errorf("%q => %s ill=%v lang=%s (cues %v); want %s ill=%v lang=%s", c.text, r.IssueType, r.MentionsIllness, r.Language, r.Cues, c.want, c.ill, c.lang)
		}
		if r.Severity < 1 || r.Severity > 5 || r.DescriptionEN == "" {
			t.Errorf("%q bad severity/description: %+v", c.text, r)
		}
	}
}

func TestWordBoundaries(t *testing.T) {
	r, _ := Rules{}.Extract(context.Background(), "I need a pill for my headache, also the tap is dripping", "")
	if r.MentionsIllness {
		t.Fatal(`"pill" matched illness cue "ill"`)
	}
}

func TestHintUsedWhenTextInconclusive(t *testing.T) {
	r, _ := Rules{}.Extract(context.Background(), "please help us", domain.IssueFoulSmell)
	if r.IssueType != domain.IssueFoulSmell {
		t.Fatalf("hint ignored: %s", r.IssueType)
	}
	r, _ = Rules{}.Extract(context.Background(), "गंदा पानी और बदबू, नाली का पानी मिल रहा है", domain.IssueNoWater)
	if r.IssueType != domain.IssueSewageMixing {
		t.Fatalf("strong text overridden by hint: %s", r.IssueType)
	}
}

func TestSeverityEscalates(t *testing.T) {
	low, _ := Rules{}.Extract(context.Background(), "water pipe leaking", "")
	high, _ := Rules{}.Extract(context.Background(), "sewage in water for weeks, children vomiting, everyone sick", "")
	if !(low.Severity < high.Severity && high.Severity == 5) {
		t.Fatalf("severity low=%d high=%d", low.Severity, high.Severity)
	}
}

func TestDescribe(t *testing.T) {
	r, _ := Rules{}.Extract(context.Background(), "नल से बदबूदार पानी, बच्चों को उल्टी", "")
	d := Describe(r, "Shiv Mandir")
	want := "Foul-smelling tap water; household reports vomiting; near Shiv Mandir (original in हिन्दी)"
	if d != want {
		t.Fatalf("got %q", d)
	}
}

func FuzzRulesNeverPanics(f *testing.F) {
	f.Add("गंदा पानी")
	f.Add("\x00\xff\xfe")
	f.Fuzz(func(t *testing.T, s string) {
		r, err := Rules{}.Extract(context.Background(), s, "")
		if err != nil || r.Severity < 1 || r.Severity > 5 || !domain.Contains(domain.IssueTypes, r.IssueType) {
			t.Fatalf("bad result %+v %v", r, err)
		}
	})
}
