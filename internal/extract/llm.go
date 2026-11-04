package extract

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/llm"
)

// LLM refines the deterministic extraction with a language model. Any error,
// timeout or invalid output falls back to the rules result unchanged.
type LLM struct {
	Client llm.Client
	Rules  Rules
}

const systemPrompt = `You read short complaints from Indian residents about their drinking-water supply.
They may be in Hindi, English, Hinglish, Marathi, Telugu or another Indian language, and may be noisy speech transcripts.
Reply with ONE JSON object and nothing else:
{"issue_type": one of ["dirty_water","foul_smell","sewage_mixing","no_water","leak","illness","other"],
 "mentions_illness": true|false, "diarrhoea": true|false, "vomiting": true|false,
 "landmark": "nearby place named by the resident, verbatim, or empty",
 "description_en": "one neutral English sentence, max 30 words, no names or phone numbers"}
Prefer sewage_mixing over dirty_water over foul_smell when several apply. Never invent details.`

type llmOut struct {
	IssueType       string `json:"issue_type"`
	MentionsIllness bool   `json:"mentions_illness"`
	Diarrhoea       bool   `json:"diarrhoea"`
	Vomiting        bool   `json:"vomiting"`
	Landmark        string `json:"landmark"`
	DescriptionEN   string `json:"description_en"`
}

// Extract implements Extractor.
func (l LLM) Extract(ctx context.Context, text string, hint domain.IssueType) (Result, error) {
	base, _ := l.Rules.Extract(ctx, text, hint)
	if l.Client == nil || strings.TrimSpace(text) == "" {
		return base, nil
	}
	reply, err := l.Client.Complete(ctx, systemPrompt, text)
	if err != nil {
		base.Extractor = "rules (llm error)"
		return base, nil
	}
	var out llmOut
	if err := json.Unmarshal([]byte(llm.JSONObject(reply)), &out); err != nil {
		base.Extractor = "rules (llm invalid)"
		return base, nil
	}
	it := domain.IssueType(out.IssueType)
	if !domain.Contains(domain.IssueTypes, it) {
		base.Extractor = "rules (llm invalid)"
		return base, nil
	}
	res := base
	res.Extractor = l.Client.Name()
	res.IssueType = it
	res.MentionsIllness = out.MentionsIllness || base.MentionsIllness
	res.Diarrhoea = out.Diarrhoea || base.Diarrhoea
	res.Vomiting = out.Vomiting || base.Vomiting
	res.Landmark = clip(out.Landmark, 80)
	res.Severity = max(base.Severity, severity(it, res.MentionsIllness, false, false))
	res.Confidence = max(base.Confidence, 0.8)
	if d := clip(out.DescriptionEN, 240); d != "" {
		res.DescriptionEN = d
	} else {
		res.DescriptionEN = Describe(res, "")
	}
	return res, nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
