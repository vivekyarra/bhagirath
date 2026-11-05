package agents

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
	"github.com/vivekyarra/bhagirath/internal/store"
)

// AddWaterTest stores a water test (manual entry, CSV import, field kit) and
// re-evaluates the ward.
func (e *Engine) AddWaterTest(ctx context.Context, t domain.WaterTest, actor string) (domain.WaterTest, error) {
	if !domain.Contains(domain.TestSources, t.Source) {
		return t, fmt.Errorf("unknown test source %q", t.Source)
	}
	if strings.TrimSpace(t.Parameter) == "" || t.WardID == "" || t.SampledAt.IsZero() {
		return t, errors.New("ward, parameter and sample date are required")
	}
	if t.SampledAt.After(e.now().Add(time.Hour)) {
		return t, errors.New("sample date is in the future")
	}
	if t.Value != nil && t.LimitValue != nil && !t.IsContaminated && *t.Value > *t.LimitValue {
		t.IsContaminated = true // a value above its limit is a failure, whatever the form said
	}
	t.CreatedAt, t.CreatedBy = e.now(), actor
	err := e.Store.Tx(ctx, func(tx *store.Store) error {
		if err := tx.CreateWaterTest(ctx, &t); err != nil {
			return err
		}
		e.event(ctx, tx, actor, "water_test.created", "water_test", t.Ref, t.WardID, map[string]any{"parameter": t.Parameter, "contaminated": t.IsContaminated, "source": t.Source})
		return nil
	})
	if err != nil {
		return t, err
	}
	_, err = e.EvaluateWard(ctx, t.WardID)
	return t, err
}

// AddIllness stores an aggregate illness report and re-evaluates the ward.
func (e *Engine) AddIllness(ctx context.Context, r domain.IllnessReport, actor string) (domain.IllnessReport, error) {
	return e.addIllness(ctx, r, actor, "", "")
}

// ErrDuplicateDelivery means a provider retried a message already stored.
var ErrDuplicateDelivery = errors.New("message already processed")

// addIllness optionally records a provider message id in the same
// transaction, so a retry after a failure is processed rather than dropped.
func (e *Engine) addIllness(ctx context.Context, r domain.IllnessReport, actor, provider, msgID string) (domain.IllnessReport, error) {
	if !r.ReporterType.IllnessSource() || r.WardID == "" {
		return r, errors.New("ward and a valid reporter type are required")
	}
	if r.CasesDiarrhoea < 0 || r.CasesVomiting < 0 || r.CasesDiarrhoea > 10000 || r.CasesVomiting > 10000 || (r.ORSUnitsSold != nil && (*r.ORSUnitsSold < 0 || *r.ORSUnitsSold > 100000)) {
		return r, errors.New("counts out of range")
	}
	if r.ReportedAt.IsZero() {
		r.ReportedAt = e.now()
	}
	if r.ReportedAt.After(e.now().Add(time.Hour)) {
		return r, errors.New("report date is in the future")
	}
	r.CreatedAt = e.now()
	err := e.Store.Tx(ctx, func(tx *store.Store) error {
		if msgID != "" {
			fresh, err := tx.MarkInboundSeen(ctx, provider, msgID, e.now())
			if err != nil {
				return err
			}
			if !fresh {
				return ErrDuplicateDelivery
			}
		}
		if err := tx.CreateIllness(ctx, &r); err != nil {
			return err
		}
		e.event(ctx, tx, actor, "illness.created", "illness_report", r.Ref, r.WardID, map[string]any{"type": r.ReporterType, "d": r.CasesDiarrhoea, "v": r.CasesVomiting, "ors": r.ORSUnitsSold})
		return nil
	})
	if err != nil {
		return r, err
	}
	_, err = e.EvaluateWard(ctx, r.WardID)
	return r, err
}

// IllnessCounts is a parsed clinic/chemist message.
type IllnessCounts struct {
	Diarrhoea, Vomiting int
	ORS                 *int
}

var countRe = regexp.MustCompile(`(?i)(ors|d|v|दस्त|उल्टी|जुलाब|उलटी|विरेचन|विరేచనాలు|వాంతులు|diarrh?o?ea|vomit(?:ing)?)\s*[:=\-]?\s*(\d{1,5})`)

// ParseIllness reads messages like "D 3 V 1 ORS 12", "दस्त 4 उल्टी 2",
// "diarrhoea: 2, vomiting 1". It returns false if nothing was found.
func ParseIllness(text string) (IllnessCounts, bool) {
	var out IllnessCounts
	found := false
	for _, m := range countRe.FindAllStringSubmatch(text, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		key := strings.ToLower(m[1])
		switch {
		case key == "ors":
			out.ORS = &n
		case key == "d" || strings.HasPrefix(key, "diarrh") || key == "दस्त" || key == "जुलाब" || key == "विरेचन" || key == "విరేచనాలు":
			out.Diarrhoea = n
		default:
			out.Vomiting = n
		}
		found = true
	}
	return out, found
}
