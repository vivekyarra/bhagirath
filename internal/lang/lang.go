// Package lang detects the language of short complaint texts and normalises
// them for keyword matching. It is deliberately dependency-free.
package lang

import (
	"strings"
	"unicode"
)

// Supported are the languages with full message templates. Other detected
// languages fall back to the ward's default language for outbound messages.
var Supported = []string{"en", "hi", "mr", "te"}

// Names maps codes to native display names.
var Names = map[string]string{
	"en": "English", "hi": "हिन्दी", "mr": "मराठी", "te": "తెలుగు", "ta": "தமிழ்", "bn": "বাংলা",
	"gu": "ગુજરાતી", "kn": "ಕನ್ನಡ", "ml": "മലയാളം", "pa": "ਪੰਜਾਬੀ", "or": "ଓଡ଼ିଆ",
}

// IsSupported reports whether templates exist for code.
func IsSupported(code string) bool {
	for _, s := range Supported {
		if s == code {
			return true
		}
	}
	return false
}

type script struct {
	lo, hi rune
	code   string
}

var scripts = []script{
	{0x0900, 0x097F, "hi"}, {0x0980, 0x09FF, "bn"}, {0x0A00, 0x0A7F, "pa"}, {0x0A80, 0x0AFF, "gu"},
	{0x0B00, 0x0B7F, "or"}, {0x0B80, 0x0BFF, "ta"}, {0x0C00, 0x0C7F, "te"}, {0x0C80, 0x0CFF, "kn"}, {0x0D00, 0x0D7F, "ml"},
}

// Marathi and Hindi share Devanagari; these frequent Marathi-only words
// tip the balance.
var marathiMarkers = []string{"आहे", "नाही", "येत", "आमच्या", "आमचे", "पाण्यात", "पाणी", "घाण", "गढूळ", "होत", "झाले", "दुर्गंधी", "उलट्या", "जुलाब", "कृपया", "मध्ये"}
var hindiMarkers = []string{"है", "नहीं", "हमारे", "पानी", "गंदा", "आ रहा", "बदबू", "उल्टी", "दस्त", "में", "कृपया", "रहा", "रही"}

// Romanised Hindi ("Hinglish") markers.
var hinglishMarkers = []string{"pani", "paani", "ganda", "gandā", "badbu", "nahi", "nahin", "aa raha", "aata", "hai", "ulti", "dast", "nali", "ghar", "mohalla", "bimar", "bacche", "kal se", "hamare", "gatar", "keede"}

// Detect returns an ISO-639-1 code for text ("" if there is nothing to go on).
func Detect(text string) string {
	counts := map[string]int{}
	latin := 0
	for _, r := range text {
		if r < 0x0900 {
			if unicode.IsLetter(r) {
				latin++
			}
			continue
		}
		for _, s := range scripts {
			if r >= s.lo && r <= s.hi {
				counts[s.code]++
				break
			}
		}
	}
	best, bestN := "", 0
	for code, n := range counts {
		if n > bestN || (n == bestN && code < best) {
			best, bestN = code, n
		}
	}
	if bestN > 0 && bestN >= latin/3 {
		if best == "hi" && score(text, marathiMarkers) > score(text, hindiMarkers) {
			return "mr"
		}
		return best
	}
	if latin == 0 {
		return ""
	}
	norm := " " + Normalize(text) + " "
	hits := 0
	for _, m := range hinglishMarkers {
		if strings.Contains(norm, " "+m+" ") || (strings.Contains(m, " ") && strings.Contains(norm, m)) {
			hits++
		}
	}
	if hits >= 2 {
		return "hi"
	}
	return "en"
}

func score(text string, markers []string) int {
	n := 0
	for _, m := range markers {
		n += strings.Count(text, m)
	}
	return n
}

// Normalize lowercases, maps punctuation to spaces and collapses whitespace.
// Indic combining marks are preserved (they are part of the word).
func Normalize(text string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r):
			b.WriteRune(r)
			space = false
		case r == '\u200d' || r == '\u200c': // ZWJ/ZWNJ inside Indic words
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// Pick returns lang if templates exist for it, else fallback, else "en".
func Pick(lang, fallback string) string {
	if IsSupported(lang) {
		return lang
	}
	if IsSupported(fallback) {
		return fallback
	}
	return "en"
}
