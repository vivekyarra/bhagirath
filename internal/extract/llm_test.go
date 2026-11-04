package extract

import (
	"context"
	"errors"
	"testing"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

type fakeLLM struct {
	reply string
	err   error
}

func (f fakeLLM) Complete(context.Context, string, string) (string, error) { return f.reply, f.err }
func (fakeLLM) Name() string                                               { return "fake" }

func TestLLMRefinesAndFallsBack(t *testing.T) {
	ctx := context.Background()
	good := LLM{Client: fakeLLM{reply: "Sure:\n```json\n{\"issue_type\":\"sewage_mixing\",\"mentions_illness\":true,\"diarrhoea\":true,\"landmark\":\"near the temple\",\"description_en\":\"Drain water entering supply; child has diarrhoea.\"}\n```"}}
	r, _ := good.Extract(ctx, "kuch gadbad hai paani mein", "")
	if r.IssueType != domain.IssueSewageMixing || !r.Diarrhoea || r.Extractor != "fake" || r.Landmark != "near the temple" {
		t.Fatalf("%+v", r)
	}
	for _, bad := range []LLM{
		{Client: fakeLLM{err: errors.New("timeout")}},
		{Client: fakeLLM{reply: "not json"}},
		{Client: fakeLLM{reply: `{"issue_type":"alien_invasion"}`}},
	} {
		r, err := bad.Extract(ctx, "गंदा पानी आ रहा है", "")
		if err != nil || r.IssueType != domain.IssueDirtyWater || r.Extractor == "fake" {
			t.Fatalf("fallback failed: %+v %v", r, err)
		}
	}
}
