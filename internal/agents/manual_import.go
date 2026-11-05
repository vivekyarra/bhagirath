package agents

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vivekyarra/bhagirath/internal/clock"
	"github.com/vivekyarra/bhagirath/internal/domain"
)

// ImportResult summarises a manual-log import.
type ImportResult struct {
	Imported, Duplicates int
	Errors               []string
}

// ManualLogColumns documents the CSV accepted by ImportManualLog.
const ManualLogColumns = "date,phone,channel,language,issue,text,landmark,ward_id,consent,smartphone"

// ImportManualLog migrates complaints recorded by hand (volunteer phone line,
// WhatsApp group, paper register) before the software went live, keeping
// their original dates. Each row is idempotent: re-importing the same file
// does not create duplicates.
//
// Columns: date (YYYY-MM-DD or YYYY-MM-DD HH:MM, IST), phone (optional),
// channel (volunteer|whatsapp|missed_call|web; default volunteer), language,
// issue (optional issue type), text, landmark, ward_id, consent (yes/no),
// smartphone (yes/no/blank).
func (e *Engine) ImportManualLog(ctx context.Context, r io.Reader, actor string) (ImportResult, error) {
	var out ImportResult
	cr := csv.NewReader(io.LimitReader(r, 8<<20))
	cr.FieldsPerRecord = -1
	recs, err := cr.ReadAll()
	if err != nil {
		return out, fmt.Errorf("read CSV: %w", err)
	}
	if len(recs) < 2 {
		return out, fmt.Errorf("CSV needs a header row (%s) and at least one data row", ManualLogColumns)
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, need := range []string{"date", "text"} {
		if _, ok := col[need]; !ok {
			return out, fmt.Errorf("CSV is missing the %q column (expected %s)", need, ManualLogColumns)
		}
	}
	get := func(rec []string, k string) string {
		if i, ok := col[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	yesNo := func(v string) *bool {
		switch strings.ToLower(v) {
		case "yes", "y", "1", "true", "haan", "हाँ":
			t := true
			return &t
		case "no", "n", "0", "false", "nahi", "नहीं":
			f := false
			return &f
		}
		return nil
	}
	for i, rec := range recs[1:] {
		line := i + 2
		at, err := parseManualDate(get(rec, "date"))
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("row %d: %v", line, err))
			continue
		}
		ch := domain.Channel(strings.ToLower(get(rec, "channel")))
		if ch == "" {
			ch = domain.ChannelVolunteer
		}
		if !domain.Contains(domain.Channels, ch) {
			out.Errors = append(out.Errors, fmt.Sprintf("row %d: unknown channel %q", line, ch))
			continue
		}
		issue := domain.IssueType(get(rec, "issue"))
		if issue != "" && !domain.Contains(domain.IssueTypes, issue) {
			out.Errors = append(out.Errors, fmt.Sprintf("row %d: unknown issue %q", line, issue))
			continue
		}
		in := Inbound{Channel: ch, Phone: get(rec, "phone"), Text: get(rec, "text"), LocationText: get(rec, "landmark"), WardHint: get(rec, "ward_id"),
			IssueHint: issue, Language: get(rec, "language"), At: at, Actor: actor, Backfill: true,
			ProviderMsgID: fmt.Sprintf("manual:%s:%s:%s", at.Format(time.RFC3339), hashShort(get(rec, "phone")), hashShort(get(rec, "text")))}
		if in.Phone != "" {
			in.Consent = yesNo(get(rec, "consent"))
			if in.Consent == nil {
				f := false
				in.Consent = &f // no recorded consent → no callbacks
			}
			in.HasSmartphone = yesNo(get(rec, "smartphone"))
		}
		res, err := e.Intake(ctx, in)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("row %d: %v", line, err))
			continue
		}
		if res.Duplicate {
			out.Duplicates++
		} else {
			out.Imported++
		}
	}
	return out, nil
}

func parseManualDate(v string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02", "02/01/2006 15:04", "02/01/2006"} {
		if t, err := time.ParseInLocation(layout, v, clock.IST); err == nil {
			if layout == "2006-01-02" || layout == "02/01/2006" {
				t = t.Add(12 * time.Hour)
			}
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("date %q not understood (use YYYY-MM-DD or YYYY-MM-DD HH:MM, IST)", v)
}

func hashShort(s string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}
