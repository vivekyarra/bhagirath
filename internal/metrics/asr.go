package metrics

import (
	"context"
	"sort"

	"github.com/vivekyarra/bhagirath/internal/asr"
	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// LanguageRates is speech-recognition accuracy for one language.
type LanguageRates struct {
	Lang  string
	N     int
	Rates asr.ErrorRates
}

// ASR measures speech recognition on every recording whose machine transcript
// was corrected by a human: total, pooled rates, and per-language rates.
// Demo wards are excluded when excludeDemo is set.
func ASR(ctx context.Context, st *store.Store, excludeDemo bool) (int, *asr.ErrorRates, []LanguageRates, error) {
	cs, err := st.ListComplaints(ctx, store.ComplaintFilter{TranscriptStatus: domain.TranscriptReviewed, Limit: 20000, IncludeDuplicates: true})
	if err != nil {
		return 0, nil, nil, err
	}
	demo := map[string]bool{}
	if excludeDemo {
		ws, err := st.ListWards(ctx)
		if err != nil {
			return 0, nil, nil, err
		}
		for _, w := range ws {
			demo[w.ID] = w.Demo
		}
	}
	var all []asr.ErrorRates
	byLang := map[string][]asr.ErrorRates{}
	for _, c := range cs {
		if c.TranscriptASR == "" || demo[c.WardID] {
			continue
		}
		r := asr.Compare(c.Transcript, c.TranscriptASR)
		all = append(all, r)
		byLang[c.Language] = append(byLang[c.Language], r)
	}
	if len(all) == 0 {
		return 0, nil, nil, nil
	}
	agg := asr.Aggregate(all)
	var langs []LanguageRates
	for l, rs := range byLang {
		langs = append(langs, LanguageRates{Lang: l, N: len(rs), Rates: asr.Aggregate(rs)})
	}
	sort.Slice(langs, func(i, j int) bool {
		return langs[i].N > langs[j].N || (langs[i].N == langs[j].N && langs[i].Lang < langs[j].Lang)
	})
	return len(all), &agg, langs, nil
}
