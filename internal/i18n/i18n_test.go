package i18n

import (
	"strings"
	"testing"

	"github.com/vivekyarra/bhagirath/internal/lang"
)

func TestEveryMessageInEveryLanguage(t *testing.T) {
	for _, k := range Keys() {
		en := messages[k]["en"]
		for _, l := range lang.Supported {
			s, ok := messages[k][l]
			if !ok || strings.TrimSpace(s) == "" {
				t.Errorf("%s missing %s", k, l)
				continue
			}
			// Same placeholders in every translation.
			for _, ph := range []string{"{ref}", "{ward}", "{partner}", "{issue}", "{how}", "{summary}"} {
				if strings.Contains(en, ph) != strings.Contains(s, ph) {
					t.Errorf("%s/%s placeholder %s mismatch", k, l, ph)
				}
			}
		}
	}
}

func TestAdvisoryLabelAndSafetyContent(t *testing.T) {
	for _, l := range lang.Supported {
		s := T("advisory", l, Vars{"ward": "W", "partner": "P", "how": "H"})
		if strings.ContainsAny(s, "{}") {
			t.Errorf("%s: unrendered placeholder: %s", l, s)
		}
		if !strings.Contains(s, "1") || !strings.Contains(s, "ORS") || !strings.Contains(s, "P") {
			t.Errorf("%s: advisory missing boil-1-minute / ORS / partner label", l)
		}
	}
	en := T("advisory", "en", Vars{"ward": "W", "partner": "P", "how": "H"})
	for _, must := range []string{"at least 1 minute", "ORS", "blood in stool", "not able to drink", "very sleepy", "very little urine",
		"precautionary safety message from Bhagirath and P", "It is not an official government order."} {
		if !strings.Contains(en, must) {
			t.Errorf("English advisory missing %q", must)
		}
	}
}

func TestFallback(t *testing.T) {
	if T("ivr_thanks", "ta", nil) != messages["ivr_thanks"]["hi"] {
		t.Error("unsupported language should fall back to Hindi")
	}
	if T("nope", "en", nil) != "nope" {
		t.Error("unknown key")
	}
	if Issue("leak", "te") != "పైపు లీక్" {
		t.Error("issue label")
	}
}
