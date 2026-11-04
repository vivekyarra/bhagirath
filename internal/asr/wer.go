package asr

import (
	"strings"

	"github.com/vivekyarra/bhagirath/internal/lang"
)

// ErrorRates compares a machine transcript (hypothesis) with the human-
// corrected transcript (reference). Both are normalised first.
type ErrorRates struct {
	RefWords  int     `json:"ref_words"`
	WordEdits int     `json:"word_edits"`
	WER       float64 `json:"wer"`
	RefChars  int     `json:"ref_chars"`
	CharEdits int     `json:"char_edits"`
	CER       float64 `json:"cer"`
}

// Compare computes word and character error rates.
func Compare(reference, hypothesis string) ErrorRates {
	rw := strings.Fields(lang.Normalize(reference))
	hw := strings.Fields(lang.Normalize(hypothesis))
	rc := []rune(strings.Join(rw, ""))
	hc := []rune(strings.Join(hw, ""))
	e := ErrorRates{RefWords: len(rw), WordEdits: levenshtein(rw, hw), RefChars: len(rc), CharEdits: levenshtein(rc, hc)}
	if e.RefWords > 0 {
		e.WER = float64(e.WordEdits) / float64(e.RefWords)
	}
	if e.RefChars > 0 {
		e.CER = float64(e.CharEdits) / float64(e.RefChars)
	}
	return e
}

func levenshtein[T comparable](a, b []T) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// Aggregate pools error rates (micro-average: total edits / total reference).
func Aggregate(rs []ErrorRates) ErrorRates {
	var t ErrorRates
	for _, r := range rs {
		t.RefWords += r.RefWords
		t.WordEdits += r.WordEdits
		t.RefChars += r.RefChars
		t.CharEdits += r.CharEdits
	}
	if t.RefWords > 0 {
		t.WER = float64(t.WordEdits) / float64(t.RefWords)
	}
	if t.RefChars > 0 {
		t.CER = float64(t.CharEdits) / float64(t.RefChars)
	}
	return t
}
