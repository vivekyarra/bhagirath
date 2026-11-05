package store

import (
	"context"
	"time"

	"github.com/vivekyarra/bhagirath/internal/domain"
)

// Metrics are the pilot impact numbers (spec Sections 3.2 and 14.4), computed
// directly from the database so every published number is reproducible.
type Metrics struct {
	GeneratedAt time.Time  `json:"generated_at"`
	FirstEvent  *time.Time `json:"first_event,omitempty"`
	LastEvent   *time.Time `json:"last_event,omitempty"`
	ActiveDays  int        `json:"active_days"`
	Wards       int        `json:"wards"`

	HouseholdsInformed   int `json:"households_informed"`
	RegisteredHouseholds int `json:"registered_households"`
	ConsentedHouseholds  int `json:"consented_households"`

	Complaints          int            `json:"complaints"`
	ComplaintsByChannel map[string]int `json:"complaints_by_channel"`
	ComplaintsByStatus  map[string]int `json:"complaints_by_status"`
	ComplaintsByIssue   map[string]int `json:"complaints_by_issue"`
	NonSmartphone       int            `json:"non_smartphone_complaints"`
	DistinctReporters   int            `json:"distinct_reporters"`
	AnonymousComplaints int            `json:"anonymous_complaints"`
	Duplicates          int            `json:"duplicates_linked"`
	Located             int            `json:"located_to_ward"`
	FollowupsAsked      int            `json:"followups_asked"`
	WardCorrections     int            `json:"ward_corrections"`
	Languages           map[string]int `json:"languages"`

	AlertsByPeak        map[string]int `json:"alerts_by_peak_level"`
	AlertsActive        int            `json:"alerts_active"`
	EligibleComplaints  int            `json:"eligible_complaints"`
	EscalatedComplaints int            `json:"escalated_complaints"`
	EscalationsSent     int            `json:"escalations_sent"`
	EscalationsAcked    int            `json:"escalations_acknowledged"`
	EscalationsTimedOut int            `json:"escalations_timed_out"`
	EscalationsDecided  int            `json:"escalations_decided"`
	MedianHoursToAck    *float64       `json:"median_hours_to_ack,omitempty"`
	GrievanceNumbers    int            `json:"grievance_numbers_tracked"`

	ClosureClaims      int      `json:"closure_claims"`
	VerifiedClosures   int      `json:"verified_closures"`
	ClosuresDenied     int      `json:"closures_denied"`
	ClosuresNoAnswer   int      `json:"closures_no_answer"`
	ReopenedComplaints int      `json:"reopened_complaints"`
	MedianDaysToFix    *float64 `json:"median_days_to_fix,omitempty"`
	BaselineMedianDays *float64 `json:"baseline_median_days,omitempty"`
	BaselineSamples    int      `json:"baseline_samples"`
	BaselineUnresolved int      `json:"baseline_unresolved"`

	AdvisoriesSent             int `json:"advisories_sent"`
	AdvisoriesDelivered        int `json:"advisories_delivered"`
	AdvisoryHouseholdsTargeted int `json:"advisory_households_targeted"`

	IllnessReports      int `json:"illness_reports"`
	IllnessContributors int `json:"illness_contributors"`
	SignalReporters     int `json:"signal_reporters_registered"`
	WaterTests          int `json:"water_tests"`
	WaterTestsFailed    int `json:"water_tests_failed"`

	Recordings         int `json:"recordings"`
	RecordingsReviewed int `json:"recordings_reviewed_with_asr"`

	MessagesByStatus map[string]int `json:"messages_by_status"`
}

// ComputeMetrics aggregates impact metrics. When excludeDemo is true, demo
// wards (fake seed data) are left out — pilot exports must use this.
func (s *Store) ComputeMetrics(ctx context.Context, excludeDemo bool, now time.Time) (Metrics, error) {
	m := Metrics{GeneratedAt: now, ComplaintsByChannel: map[string]int{}, ComplaintsByStatus: map[string]int{}, ComplaintsByIssue: map[string]int{},
		Languages: map[string]int{}, AlertsByPeak: map[string]int{}, MessagesByStatus: map[string]int{}}
	wf := func(col string) string {
		if !excludeDemo {
			return "TRUE"
		}
		return "NOT COALESCE((SELECT demo FROM ward WHERE ward.id = " + col + "), FALSE)"
	}
	scalar := func(dst *int, sql string, args ...any) error {
		return s.q.QueryRow(ctx, sql, args...).Scan(dst)
	}
	group := func(dst map[string]int, sql string) error {
		rows, err := s.q.Query(ctx, sql)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int
			if err := rows.Scan(&k, &n); err != nil {
				return err
			}
			dst[k] = n
		}
		return rows.Err()
	}
	cw := wf("c.ward_id")
	steps := []func() error{
		func() error { return scalar(&m.Wards, `SELECT count(*) FROM ward w WHERE `+wf("w.id")) },
		func() error {
			return scalar(&m.HouseholdsInformed, `SELECT COALESCE(sum(households),0) FROM outreach o WHERE `+wf("o.ward_id"))
		},
		func() error {
			return scalar(&m.RegisteredHouseholds, `SELECT count(*) FROM reporter r WHERE kind='resident' AND `+wf("r.ward_id"))
		},
		func() error {
			return scalar(&m.ConsentedHouseholds, `SELECT count(*) FROM reporter r WHERE kind='resident' AND consent_at IS NOT NULL AND `+wf("r.ward_id"))
		},
		func() error {
			return scalar(&m.Complaints, `SELECT count(*) FROM complaint c WHERE duplicate_of IS NULL AND status <> 'invalid' AND `+cw)
		},
		func() error {
			return group(m.ComplaintsByChannel, `SELECT channel, count(*) FROM complaint c WHERE duplicate_of IS NULL AND status <> 'invalid' AND `+cw+` GROUP BY 1`)
		},
		func() error {
			return group(m.ComplaintsByStatus, `SELECT status, count(*) FROM complaint c WHERE duplicate_of IS NULL AND `+cw+` GROUP BY 1`)
		},
		func() error {
			return group(m.ComplaintsByIssue, `SELECT issue_type, count(*) FROM complaint c WHERE duplicate_of IS NULL AND status <> 'invalid' AND `+cw+` GROUP BY 1`)
		},
		func() error {
			return group(m.Languages, `SELECT COALESCE(NULLIF(language,''),'unknown'), count(*) FROM complaint c WHERE duplicate_of IS NULL AND `+cw+` GROUP BY 1`)
		},
		func() error {
			return scalar(&m.NonSmartphone, `SELECT count(*) FROM complaint c WHERE duplicate_of IS NULL AND status <> 'invalid' AND channel IN ('missed_call','ivr','volunteer') AND `+cw)
		},
		func() error {
			return scalar(&m.DistinctReporters, `SELECT count(DISTINCT reporter_id) FROM complaint c WHERE duplicate_of IS NULL AND status <> 'invalid' AND `+cw)
		},
		func() error {
			return scalar(&m.AnonymousComplaints, `SELECT count(*) FROM complaint c WHERE reporter_id IS NULL AND duplicate_of IS NULL AND `+cw)
		},
		func() error {
			return scalar(&m.Duplicates, `SELECT count(*) FROM complaint c WHERE duplicate_of IS NOT NULL AND `+cw)
		},
		func() error {
			return scalar(&m.Located, `SELECT count(*) FROM complaint c WHERE duplicate_of IS NULL AND status <> 'invalid' AND ward_id IS NOT NULL AND `+cw)
		},
		func() error {
			return scalar(&m.FollowupsAsked, `SELECT count(*) FROM event_log c WHERE action='complaint.followup_asked' AND `+cw)
		},
		func() error {
			return scalar(&m.WardCorrections, `SELECT count(*) FROM event_log c WHERE action='complaint.ward_corrected' AND `+cw)
		},
		func() error {
			return group(m.AlertsByPeak, `SELECT peak_level, count(*) FROM alert c WHERE `+cw+` GROUP BY 1`)
		},
		func() error {
			return scalar(&m.AlertsActive, `SELECT count(*) FROM alert c WHERE status='active' AND `+cw)
		},
		func() error {
			return scalar(&m.EligibleComplaints, `SELECT count(*) FROM complaint c WHERE duplicate_of IS NULL AND status <> 'invalid' AND ward_id IS NOT NULL AND issue_type <> 'other' AND `+cw)
		},
		func() error {
			return scalar(&m.EscalatedComplaints, `SELECT count(DISTINCT e.complaint_id) FROM escalation e JOIN complaint c ON c.id=e.complaint_id WHERE c.duplicate_of IS NULL AND `+cw)
		},
		func() error { return scalar(&m.EscalationsSent, `SELECT count(*) FROM escalation c WHERE `+cw) },
		func() error {
			return scalar(&m.EscalationsAcked, `SELECT count(*) FROM escalation c WHERE acknowledged_at IS NOT NULL AND `+cw)
		},
		func() error {
			return scalar(&m.EscalationsTimedOut, `SELECT count(*) FROM escalation c WHERE outcome='timed_out' AND acknowledged_at IS NULL AND `+cw)
		},
		func() error {
			// Escalations whose acknowledgement window has ended (acknowledged or timed out) — the denominator for the ack rate.
			return scalar(&m.EscalationsDecided, `SELECT count(*) FROM escalation c WHERE (acknowledged_at IS NOT NULL OR (outcome='timed_out')) AND `+cw)
		},
		func() error {
			return scalar(&m.GrievanceNumbers, `SELECT count(*) FROM escalation c WHERE grievance_number <> '' AND `+cw)
		},
		func() error {
			return scalar(&m.ClosureClaims, `SELECT count(*) FROM closure x JOIN complaint c ON c.id=x.complaint_id WHERE `+cw)
		},
		func() error {
			return scalar(&m.VerifiedClosures, `SELECT count(*) FROM closure x JOIN complaint c ON c.id=x.complaint_id WHERE x.verified AND `+cw)
		},
		func() error {
			return scalar(&m.ClosuresDenied, `SELECT count(*) FROM closure x JOIN complaint c ON c.id=x.complaint_id WHERE x.callback_result='denied' AND `+cw)
		},
		func() error {
			return scalar(&m.ClosuresNoAnswer, `SELECT count(*) FROM closure x JOIN complaint c ON c.id=x.complaint_id WHERE x.callback_result='no_answer' AND `+cw)
		},
		func() error {
			return scalar(&m.ReopenedComplaints, `SELECT count(*) FROM complaint c WHERE reopen_count > 0 AND `+cw)
		},
		func() error {
			return scalar(&m.AdvisoriesSent, `SELECT count(*) FROM advisory x JOIN alert c ON c.id=x.alert_id WHERE `+cw)
		},
		func() error {
			return scalar(&m.AdvisoriesDelivered, `SELECT count(*) FROM advisory x JOIN alert c ON c.id=x.alert_id WHERE x.delivered AND `+cw)
		},
		func() error {
			return scalar(&m.AdvisoryHouseholdsTargeted, `SELECT count(DISTINCT x.reporter_id) FROM advisory x JOIN alert c ON c.id=x.alert_id WHERE `+cw)
		},
		func() error { return scalar(&m.IllnessReports, `SELECT count(*) FROM illness_report c WHERE `+cw) },
		func() error {
			return scalar(&m.IllnessContributors, `SELECT count(DISTINCT reporter_id) FROM illness_report c WHERE reporter_type <> 'resident' AND reporter_id IS NOT NULL AND `+cw)
		},
		func() error {
			return scalar(&m.SignalReporters, `SELECT count(*) FROM reporter c WHERE kind IN ('clinic','chemist','asha','hospital') AND `+cw)
		},
		func() error { return scalar(&m.WaterTests, `SELECT count(*) FROM water_test c WHERE `+cw) },
		func() error {
			return scalar(&m.WaterTestsFailed, `SELECT count(*) FROM water_test c WHERE is_contaminated AND `+cw)
		},
		func() error {
			return scalar(&m.Recordings, `SELECT count(*) FROM complaint c WHERE (audio_media_id IS NOT NULL OR transcript_asr <> '') AND `+cw)
		},
		func() error {
			return scalar(&m.RecordingsReviewed, `SELECT count(*) FROM complaint c WHERE transcript_asr <> '' AND transcript_status='reviewed' AND `+cw)
		},
		func() error {
			return group(m.MessagesByStatus, `SELECT status, count(*) FROM outbound_message GROUP BY 1`)
		},
	}
	for _, f := range steps {
		if err := f(); err != nil {
			return m, err
		}
	}
	var med *float64
	if err := s.q.QueryRow(ctx, `SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (acknowledged_at - sent_at))/3600)
		FROM escalation c WHERE acknowledged_at IS NOT NULL AND `+cw).Scan(&med); err != nil {
		return m, err
	}
	m.MedianHoursToAck = med
	med = nil
	if err := s.q.QueryRow(ctx, `SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (verified_fixed_at - created_at))/86400)
		FROM complaint c WHERE verified_fixed_at IS NOT NULL AND `+cw).Scan(&med); err != nil {
		return m, err
	}
	m.MedianDaysToFix = med
	med = nil
	if err := s.q.QueryRow(ctx, `SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY days_to_fix), count(*), count(*) FILTER (WHERE NOT resolved)
		FROM baseline c WHERE `+cw).Scan(&med, &m.BaselineSamples, &m.BaselineUnresolved); err != nil {
		return m, err
	}
	m.BaselineMedianDays = med
	if err := s.q.QueryRow(ctx, `SELECT min(at), max(at), count(DISTINCT (at AT TIME ZONE 'Asia/Kolkata')::date) FROM event_log c
		WHERE action IN ('complaint.created','escalation.sent','closure.verified','illness.created','water_test.created') AND `+cw).
		Scan(&m.FirstEvent, &m.LastEvent, &m.ActiveDays); err != nil {
		return m, err
	}
	return m, nil
}

// ProofEntry is one fix claim and what the resident said about it. It holds
// no personal data: no phone, name, transcript or location text.
type ProofEntry struct {
	Ref          string
	WardName     string
	IssueType    domain.IssueType
	Channel      string
	ReportedAt   time.Time
	ClaimedAt    time.Time
	DecidedAt    *time.Time
	Evidence     string
	Result       string
	VerifyVia    string
	DaysToVerify *float64
}

// ProofLog lists the most recent decided fix claims (verified and rejected),
// newest first — the public, checkable record of the accountability loop.
func (s *Store) ProofLog(ctx context.Context, excludeDemo bool, limit int) ([]ProofEntry, error) {
	demo := ""
	if excludeDemo {
		demo = " AND NOT w.demo"
	}
	rows, err := s.q.Query(ctx, `SELECT c.ref, w.name, c.issue_type, c.channel, c.created_at, x.claimed_at, x.decided_at, x.evidence_type,
			x.callback_result, x.callback_channel,
			CASE WHEN x.verified THEN EXTRACT(EPOCH FROM (x.decided_at - c.created_at))/86400 END
		FROM closure x JOIN complaint c ON c.id = x.complaint_id JOIN ward w ON w.id = c.ward_id
		WHERE x.decided_at IS NOT NULL`+demo+`
		ORDER BY x.decided_at DESC, x.id DESC LIMIT $1`, clampLimit(limit, 50, 500))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProofEntry
	for rows.Next() {
		var e ProofEntry
		if err := rows.Scan(&e.Ref, &e.WardName, &e.IssueType, &e.Channel, &e.ReportedAt, &e.ClaimedAt, &e.DecidedAt, &e.Evidence, &e.Result, &e.VerifyVia, &e.DaysToVerify); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
