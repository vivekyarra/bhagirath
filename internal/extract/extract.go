// Package extract turns a transcript into a structured complaint. The default
// extractor is deterministic and multilingual (keyword lexicon); an optional
// LLM extractor can refine it, but its output is always validated against the
// same enums and falls back to the deterministic result on any failure.
package extract

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/lang"
)

// Result is a structured reading of one complaint.
type Result struct {
	IssueType       domain.IssueType `json:"issue_type"`
	Severity        int              `json:"severity"`
	Language        string           `json:"language"`
	MentionsIllness bool             `json:"mentions_illness"`
	Diarrhoea       bool             `json:"diarrhoea"`
	Vomiting        bool             `json:"vomiting"`
	Landmark        string           `json:"landmark,omitempty"`
	DescriptionEN   string           `json:"description_en"`
	Cues            []string         `json:"cues,omitempty"`
	Extractor       string           `json:"extractor"`
	Confidence      float64          `json:"confidence"`
}

// Extractor extracts structure from text.
type Extractor interface {
	Extract(ctx context.Context, text string, hint domain.IssueType) (Result, error)
}

// Rules is the deterministic lexicon extractor.
type Rules struct{}

// priority breaks ties: the more dangerous reading wins.
var priority = []domain.IssueType{domain.IssueSewageMixing, domain.IssueDirtyWater, domain.IssueFoulSmell, domain.IssueLeak, domain.IssueNoWater}

func matches(norm string, cues []string) []string {
	padded := " " + norm + " "
	var out []string
	for _, c := range cues {
		cn := lang.Normalize(c)
		if cn == "" {
			continue
		}
		if isLatin(cn) {
			// Latin cues match on word boundaries ("ill" must not match "pill").
			if strings.Contains(padded, " "+cn+" ") || (len(cn) >= 5 && strings.Contains(padded, " "+cn)) {
				out = append(out, c)
			}
		} else if strings.Contains(norm, cn) {
			out = append(out, c)
		}
	}
	return out
}

func isLatin(s string) bool {
	for _, r := range s {
		if r > 0x024F {
			return false
		}
	}
	return true
}

// Extract classifies text. hint (e.g. an IVR menu choice or web-form button)
// is used when the text itself is inconclusive.
func (Rules) Extract(_ context.Context, text string, hint domain.IssueType) (Result, error) {
	norm := lang.Normalize(text)
	res := Result{Language: lang.Detect(text), Extractor: "rules"}
	scores := map[domain.IssueType]int{}
	for it, cues := range lexicon {
		m := matches(norm, cues)
		for _, c := range m {
			scores[it] += 1 + len([]rune(c))/8 // longer phrases are stronger evidence
		}
		res.Cues = append(res.Cues, m...)
	}
	ill := matches(norm, illnessCues)
	res.MentionsIllness = len(ill) > 0
	res.Diarrhoea = len(matches(norm, diarrhoeaCues)) > 0
	res.Vomiting = len(matches(norm, vomitingCues)) > 0
	res.Cues = append(res.Cues, ill...)

	best, bestScore := domain.IssueOther, 0
	for _, it := range priority {
		if scores[it] > bestScore {
			best, bestScore = it, scores[it]
		}
	}
	// Water-quality cues outrank "no water"/"leak" when both appear: "the leak
	// is letting drain water in" is a contamination complaint.
	if best == domain.IssueLeak || best == domain.IssueNoWater {
		for _, it := range priority[:3] {
			if scores[it] > 0 {
				best = it
				break
			}
		}
	}
	switch {
	case best != domain.IssueOther:
		res.Confidence = min(0.95, 0.55+0.1*float64(bestScore))
	case hint != "" && hint != domain.IssueOther:
		best, res.Confidence = hint, 0.6
	case res.MentionsIllness:
		best, res.Confidence = domain.IssueIllness, 0.6
	default:
		res.Confidence = 0.2
	}
	if hint != "" && hint != domain.IssueOther && best != hint && bestScore <= 1 {
		best = hint // an explicit menu choice beats a single weak cue
	}
	res.IssueType = best
	res.Severity = severity(best, res.MentionsIllness, len(matches(norm, urgencyCues)) > 0, len(matches(norm, durationCues)) > 0)
	sort.Strings(res.Cues)
	res.Cues = dedupe(res.Cues)
	res.DescriptionEN = Describe(res, "")
	return res, nil
}

func severity(it domain.IssueType, ill, urgent, persistent bool) int {
	s := 1
	switch it {
	case domain.IssueSewageMixing:
		s = 3
	case domain.IssueDirtyWater, domain.IssueFoulSmell, domain.IssueIllness:
		s = 2
	}
	if ill {
		s++
	}
	if urgent {
		s++
	}
	if persistent && s < 4 {
		s++
	}
	return max(1, min(5, s))
}

var issueLabels = map[domain.IssueType]string{
	domain.IssueDirtyWater:   "Dirty or discoloured tap water",
	domain.IssueFoulSmell:    "Foul-smelling tap water",
	domain.IssueSewageMixing: "Sewage/drain water mixing into drinking water",
	domain.IssueNoWater:      "No water supply",
	domain.IssueLeak:         "Leaking or broken water pipe",
	domain.IssueIllness:      "Residents report diarrhoea/vomiting",
	domain.IssueOther:        "Water problem (unclassified)",
}

// IssueLabel is the English label of an issue type.
func IssueLabel(it domain.IssueType) string {
	if l, ok := issueLabels[it]; ok {
		return l
	}
	return string(it)
}

// Describe writes the English one-line description used in evidence packs.
func Describe(r Result, landmark string) string {
	var b strings.Builder
	b.WriteString(IssueLabel(r.IssueType))
	if r.MentionsIllness && r.IssueType != domain.IssueIllness {
		var sym []string
		if r.Diarrhoea {
			sym = append(sym, "diarrhoea")
		}
		if r.Vomiting {
			sym = append(sym, "vomiting")
		}
		if len(sym) == 0 {
			sym = append(sym, "illness")
		}
		fmt.Fprintf(&b, "; household reports %s", strings.Join(sym, " and "))
	}
	if landmark != "" {
		fmt.Fprintf(&b, "; near %s", landmark)
	}
	if r.Language != "" && r.Language != "en" {
		fmt.Fprintf(&b, " (original in %s)", lang.Names[r.Language])
	}
	return b.String()
}

func dedupe(s []string) []string {
	out := s[:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}
