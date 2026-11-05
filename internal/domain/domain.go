// Package domain defines Bhagirath's core records. Enumerations mirror the
// CHECK constraints in migrations/ and are validated at every boundary.
package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// Channel is how a complaint reached Bhagirath.
type Channel string

// Intake channels.
const (
	ChannelMissedCall Channel = "missed_call"
	ChannelIVR        Channel = "ivr"
	ChannelWhatsApp   Channel = "whatsapp"
	ChannelWeb        Channel = "web"
	ChannelVolunteer  Channel = "volunteer"
)

// Channels lists every intake channel.
var Channels = []Channel{ChannelMissedCall, ChannelIVR, ChannelWhatsApp, ChannelWeb, ChannelVolunteer}

// NonSmartphone reports whether the channel works on a basic feature phone.
func (c Channel) NonSmartphone() bool {
	return c == ChannelMissedCall || c == ChannelIVR || c == ChannelVolunteer
}

// IssueType classifies a complaint.
type IssueType string

// Issue types.
const (
	IssueDirtyWater   IssueType = "dirty_water"
	IssueFoulSmell    IssueType = "foul_smell"
	IssueSewageMixing IssueType = "sewage_mixing"
	IssueNoWater      IssueType = "no_water"
	IssueLeak         IssueType = "leak"
	IssueIllness      IssueType = "illness"
	IssueOther        IssueType = "other"
)

// IssueTypes lists every issue type.
var IssueTypes = []IssueType{IssueDirtyWater, IssueFoulSmell, IssueSewageMixing, IssueNoWater, IssueLeak, IssueIllness, IssueOther}

// ComplaintStatus is the lifecycle state of a complaint.
type ComplaintStatus string

// Complaint statuses.
const (
	StatusOpen          ComplaintStatus = "open"
	StatusEscalated     ComplaintStatus = "escalated"
	StatusClaimedFixed  ComplaintStatus = "claimed_fixed"
	StatusVerifiedFixed ComplaintStatus = "verified_fixed"
	StatusReopened      ComplaintStatus = "reopened"
	StatusInvalid       ComplaintStatus = "invalid"
)

// ComplaintStatuses lists every status.
var ComplaintStatuses = []ComplaintStatus{StatusOpen, StatusEscalated, StatusClaimedFixed, StatusVerifiedFixed, StatusReopened, StatusInvalid}

// Closed reports whether the complaint needs no further action.
func (s ComplaintStatus) Closed() bool { return s == StatusVerifiedFixed || s == StatusInvalid }

// LocationMethod records how a complaint's ward was resolved.
type LocationMethod string

// Location methods, from most to least precise.
const (
	LocGPS        LocationMethod = "gps"
	LocLandmark   LocationMethod = "landmark"
	LocRegistered LocationMethod = "registered_address"
	LocManual     LocationMethod = "manual"
	LocWardOnly   LocationMethod = "ward_only"
	LocUnresolved LocationMethod = "unresolved"
)

// TranscriptStatus tracks the speech-to-text review workflow.
type TranscriptStatus string

// Transcript states.
const (
	TranscriptNone        TranscriptStatus = "none"         // text intake, nothing to transcribe
	TranscriptNeedsReview TranscriptStatus = "needs_review" // audio awaiting human transcript/correction
	TranscriptASR         TranscriptStatus = "asr"          // machine transcript, not yet reviewed
	TranscriptReviewed    TranscriptStatus = "reviewed"     // human-verified
)

// ReporterKind is who a reporter is.
type ReporterKind string

// Reporter kinds. Clinics, chemists, ASHAs and hospitals send illness counts.
const (
	KindResident  ReporterKind = "resident"
	KindClinic    ReporterKind = "clinic"
	KindChemist   ReporterKind = "chemist"
	KindASHA      ReporterKind = "asha"
	KindHospital  ReporterKind = "hospital"
	KindVolunteer ReporterKind = "volunteer"
)

// ReporterKinds lists every reporter kind.
var ReporterKinds = []ReporterKind{KindResident, KindClinic, KindChemist, KindASHA, KindHospital, KindVolunteer}

// IllnessSource reports whether this kind may submit illness counts.
func (k ReporterKind) IllnessSource() bool {
	return k == KindClinic || k == KindChemist || k == KindASHA || k == KindHospital || k == KindResident
}

// TestSource is where a water test came from.
type TestSource string

// Test sources.
const (
	TestJJMPublic   TestSource = "jjm_public"
	TestLabReport   TestSource = "lab_report"
	TestFieldKit    TestSource = "field_test_kit"
	TestPartnerTest TestSource = "partner_test"
)

// TestSources lists every source.
var TestSources = []TestSource{TestJJMPublic, TestLabReport, TestFieldKit, TestPartnerTest}

// Level is an alert level. The zero value means "no alert".
type Level int

// Alert levels, ordered.
const (
	LevelNone Level = iota
	LevelWatch
	LevelWarning
	LevelCritical
)

func (l Level) String() string {
	switch l {
	case LevelWatch:
		return "watch"
	case LevelWarning:
		return "warning"
	case LevelCritical:
		return "critical"
	default:
		return "none"
	}
}

// Title is the display form ("Warning").
func (l Level) Title() string {
	switch l {
	case LevelWatch:
		return "Watch"
	case LevelWarning:
		return "Warning"
	case LevelCritical:
		return "Critical"
	default:
		return "None"
	}
}

// ParseLevel parses a stored level.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "none", "":
		return LevelNone, nil
	case "watch":
		return LevelWatch, nil
	case "warning":
		return LevelWarning, nil
	case "critical":
		return LevelCritical, nil
	}
	return LevelNone, fmt.Errorf("unknown level %q", s)
}

// MarshalJSON encodes the level as its name.
func (l Level) MarshalJSON() ([]byte, error) { return json.Marshal(l.String()) }

// UnmarshalJSON decodes a level name.
func (l *Level) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseLevel(s)
	*l = v
	return err
}

// LadderLevel is a rung on the escalation ladder.
type LadderLevel string

// Ladder rungs (Section 10.1 of the spec).
const (
	Ladder1       LadderLevel = "1" // AE/JE water supply
	Ladder2       LadderLevel = "2" // Executive Engineer
	Ladder3       LadderLevel = "3" // Commissioner / BDO / CEO ZP
	LadderHealth  LadderLevel = "H" // District Health Officer / CMHO
	LadderPartner LadderLevel = "P" // Councillor / Sarpanch + partner NGO
)

// LadderLevels lists rungs in display order.
var LadderLevels = []LadderLevel{Ladder1, Ladder2, Ladder3, LadderHealth, LadderPartner}

// Next is the rung a timed-out escalation moves to. Health and partner rungs
// escalate to the commissioner; the commissioner is the top of the ladder.
func (l LadderLevel) Next() (LadderLevel, bool) {
	switch l {
	case Ladder1:
		return Ladder2, true
	case Ladder2, LadderHealth:
		return Ladder3, true
	}
	return "", false
}

// Rank orders engineering rungs for "one level up" decisions.
func (l LadderLevel) Rank() int {
	switch l {
	case Ladder1:
		return 1
	case Ladder2:
		return 2
	case Ladder3:
		return 3
	case LadderHealth:
		return 2
	}
	return 0
}

// EscalationOutcome is the state of a single escalation.
type EscalationOutcome string

// Escalation outcomes.
const (
	OutcomePending      EscalationOutcome = "pending"
	OutcomeAcknowledged EscalationOutcome = "acknowledged"
	OutcomeTimedOut     EscalationOutcome = "timed_out"
	OutcomeResolved     EscalationOutcome = "resolved"
	OutcomeClosed       EscalationOutcome = "closed" // parent alert/complaint resolved before deadline
)

// CallbackResult is the complainant's answer to "was it fixed?".
type CallbackResult string

// Callback results.
const (
	CallbackPending   CallbackResult = "pending"
	CallbackConfirmed CallbackResult = "confirmed"
	CallbackDenied    CallbackResult = "denied"
	CallbackNoAnswer  CallbackResult = "no_answer"
)

// EvidenceType of a closure claim.
type EvidenceType string

// Evidence types.
const (
	EvidencePhoto  EvidenceType = "photo"
	EvidenceRetest EvidenceType = "retest"
	EvidenceBoth   EvidenceType = "both"
)

// Contains reports whether v is one of allowed.
func Contains[T comparable](allowed []T, v T) bool { return slices.Contains(allowed, v) }

// Point is a WGS84 coordinate.
type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Valid reports whether the point is a plausible coordinate (not 0,0).
func (p Point) Valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180 && !(p.Lat == 0 && p.Lon == 0)
}

// Landmark is a named place used to resolve spoken locations.
type Landmark struct {
	Name    string   `json:"name" yaml:"name"`
	Aliases []string `json:"aliases,omitempty" yaml:"aliases"`
	Point   Point    `json:"point" yaml:"point"`
}

// Ward is a pilot area (urban ward or village).
type Ward struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	CityOrBlock     string     `json:"city_or_block"`
	District        string     `json:"district"`
	State           string     `json:"state"`
	Polygon         [][]Point  `json:"polygon"` // outer ring first; holes after
	Centroid        Point      `json:"centroid"`
	DefaultLanguage string     `json:"default_language"`
	Landmarks       []Landmark `json:"landmarks"`
	PartnerName     string     `json:"partner_name"`
	Demo            bool       `json:"demo"`
}

// Reporter is a resident or a signal contributor. Phone numbers are held
// only as a keyed hash and as ciphertext (see package secure).
type Reporter struct {
	ID                int64        `json:"-"`
	Ref               string       `json:"ref"`
	PhoneHash         string       `json:"-"`
	PhoneEnc          []byte       `json:"-"`
	Kind              ReporterKind `json:"kind"`
	Label             string       `json:"label,omitempty"` // organisation label for clinics, never a patient name
	PreferredLanguage string       `json:"preferred_language"`
	PreferredChannel  string       `json:"preferred_channel"` // whatsapp | voice
	WardID            string       `json:"ward_id,omitempty"`
	Home              *Point       `json:"-"`
	HasSmartphone     *bool        `json:"has_smartphone,omitempty"`
	ConsentAt         *time.Time   `json:"consent_at,omitempty"`
	ConsentAskedAt    *time.Time   `json:"-"`
	ConsentDeclined   bool         `json:"-"`
	CreatedAt         time.Time    `json:"created_at"`
}

// CanContact reports whether Bhagirath may call or message this reporter.
func (r *Reporter) CanContact() bool { return r != nil && r.ConsentAt != nil && len(r.PhoneEnc) > 0 }

// Complaint is one report of a water problem.
type Complaint struct {
	ID               int64            `json:"-"`
	Ref              string           `json:"ref"`
	ReporterID       *int64           `json:"-"`
	ReporterRef      string           `json:"reporter_ref,omitempty"`
	Channel          Channel          `json:"channel"`
	ProviderMsgID    string           `json:"-"`
	AudioMediaID     *int64           `json:"-"`
	Transcript       string           `json:"transcript"`
	TranscriptASR    string           `json:"transcript_asr,omitempty"`
	TranscriptStatus TranscriptStatus `json:"transcript_status"`
	Language         string           `json:"language"`
	IssueType        IssueType        `json:"issue_type"`
	Severity         int              `json:"severity"`
	DescriptionEN    string           `json:"description_en"`
	Point            *Point           `json:"point,omitempty"`
	LocationText     string           `json:"location_text"`
	LocationMethod   LocationMethod   `json:"location_method"`
	WardID           string           `json:"ward_id,omitempty"`
	Status           ComplaintStatus  `json:"status"`
	DuplicateOf      *int64           `json:"-"`
	NeedsFollowup    bool             `json:"needs_followup"`
	Extraction       json.RawMessage  `json:"extraction,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	VerifiedFixedAt  *time.Time       `json:"verified_fixed_at,omitempty"`
	ReopenCount      int              `json:"reopen_count"`
}

// WaterTest is one parameter of one water sample.
type WaterTest struct {
	ID             int64      `json:"-"`
	Ref            string     `json:"ref"`
	Source         TestSource `json:"source"`
	Point          *Point     `json:"point,omitempty"`
	WardID         string     `json:"ward_id"`
	SampledAt      time.Time  `json:"sampled_at"`
	Parameter      string     `json:"parameter"`
	Value          *float64   `json:"value,omitempty"`
	Unit           string     `json:"unit"`
	LimitValue     *float64   `json:"limit_value,omitempty"`
	IsContaminated bool       `json:"is_contaminated"`
	EvidenceMedia  *int64     `json:"-"`
	EvidenceURL    string     `json:"evidence_url,omitempty"`
	Note           string     `json:"note,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CreatedBy      string     `json:"created_by"`
}

// IllnessReport is an aggregate count (never a patient record).
type IllnessReport struct {
	ID             int64        `json:"-"`
	Ref            string       `json:"ref"`
	ReporterType   ReporterKind `json:"reporter_type"`
	ReporterID     *int64       `json:"-"`
	ReporterRef    string       `json:"reporter_ref,omitempty"`
	WardID         string       `json:"ward_id"`
	Point          *Point       `json:"point,omitempty"`
	ReportedAt     time.Time    `json:"reported_at"`
	CasesDiarrhoea int          `json:"cases_diarrhoea"`
	CasesVomiting  int          `json:"cases_vomiting"`
	ORSUnitsSold   *int         `json:"ors_units_sold,omitempty"`
	Notes          string       `json:"notes,omitempty"`
	ComplaintID    *int64       `json:"-"` // set when derived from a resident complaint
	CreatedAt      time.Time    `json:"created_at"`
}

// Alert is the output of the frozen rule for one ward.
type Alert struct {
	ID                 int64           `json:"id"`
	WardID             string          `json:"ward_id"`
	Level              Level           `json:"level"`
	PeakLevel          Level           `json:"peak_level"`
	RuleVersion        string          `json:"rule_version"`
	RuleSHA256         string          `json:"rule_sha256"`
	TriggeredAt        time.Time       `json:"triggered_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	ResolvedAt         *time.Time      `json:"resolved_at,omitempty"`
	Signals            json.RawMessage `json:"signals"`
	ReasonText         string          `json:"reason_text"`
	ReasonLocal        string          `json:"reason_local,omitempty"`
	Status             string          `json:"status"` // active | resolved
	AdvisoryApprovedAt *time.Time      `json:"advisory_approved_at,omitempty"`
	AdvisoryApprovedBy string          `json:"advisory_approved_by,omitempty"`
}

// AlertTransition records each level change for audit and metrics.
type AlertTransition struct {
	ID         int64           `json:"id"`
	AlertID    int64           `json:"alert_id"`
	At         time.Time       `json:"at"`
	FromLevel  Level           `json:"from_level"`
	ToLevel    Level           `json:"to_level"`
	ReasonText string          `json:"reason_text"`
	Signals    json.RawMessage `json:"signals"`
}

// Official is a person on the escalation ladder. Contacts come only from
// public sources or the partner and are kept out of the repository.
type Official struct {
	ID               int64       `json:"id"`
	Name             string      `json:"name"`
	Role             string      `json:"role"`
	Department       string      `json:"department"`
	WardIDs          []string    `json:"ward_ids"`
	PhoneEnc         []byte      `json:"-"`
	PhoneHash        string      `json:"-"`
	Email            string      `json:"email,omitempty"`
	LadderLevel      LadderLevel `json:"ladder_level"`
	GrievanceChannel string      `json:"grievance_channel,omitempty"` // e.g. "CM Helpline 181" when no direct contact
	PreferredChannel string      `json:"preferred_channel"`           // email | whatsapp | grievance
	Language         string      `json:"language"`
	Active           bool        `json:"active"`
}

// Escalation is one evidence pack sent to one official with a deadline.
type Escalation struct {
	ID              int64             `json:"id"`
	Ref             string            `json:"ref"`
	AlertID         *int64            `json:"alert_id,omitempty"`
	ComplaintID     *int64            `json:"complaint_id,omitempty"`
	OfficialID      int64             `json:"official_id"`
	Level           LadderLevel       `json:"level"`
	WardID          string            `json:"ward_id"`
	SentAt          time.Time         `json:"sent_at"`
	Channel         string            `json:"channel"`
	TokenHash       string            `json:"-"`
	DeadlineAt      time.Time         `json:"deadline_at"`
	AcknowledgedAt  *time.Time        `json:"acknowledged_at,omitempty"`
	AckVia          string            `json:"ack_via,omitempty"`
	GrievanceNumber string            `json:"grievance_number,omitempty"`
	Outcome         EscalationOutcome `json:"outcome"`
	ParentID        *int64            `json:"parent_id,omitempty"`
	Reason          string            `json:"reason"` // new_alert | level_up | complaint | timeout | reopened
	ClosedAt        *time.Time        `json:"closed_at,omitempty"`
}

// Closure is an official's "fixed" claim and its verification.
type Closure struct {
	ID               int64          `json:"id"`
	ComplaintID      int64          `json:"complaint_id"`
	ClaimedAt        time.Time      `json:"claimed_at"`
	ClaimedBy        string         `json:"claimed_by"`
	EscalationID     *int64         `json:"escalation_id,omitempty"`
	EvidenceType     EvidenceType   `json:"evidence_type"`
	EvidenceMediaID  *int64         `json:"-"`
	RetestID         *int64         `json:"-"`
	Note             string         `json:"note"`
	CallbackAttempts int            `json:"callback_attempts"`
	NextCallbackAt   *time.Time     `json:"next_callback_at,omitempty"`
	CallbackAt       *time.Time     `json:"callback_at,omitempty"`
	CallbackResult   CallbackResult `json:"callback_result"`
	CallbackChannel  string         `json:"callback_channel,omitempty"`
	Verified         bool           `json:"verified"`
	DecidedAt        *time.Time     `json:"decided_at,omitempty"`
}

// Advisory is one precautionary message to one household.
type Advisory struct {
	ID         int64     `json:"id"`
	AlertID    int64     `json:"alert_id"`
	ReporterID int64     `json:"-"`
	Level      Level     `json:"level"`
	Language   string    `json:"language"`
	Channel    string    `json:"channel"`
	SentAt     time.Time `json:"sent_at"`
	Delivered  bool      `json:"delivered"`
	MessageID  *int64    `json:"-"`
	Text       string    `json:"text"`
}

// MessageStatus is the delivery state of an outbound message.
type MessageStatus string

// Message statuses. "simulated" means the deployment runs without a provider
// (demo mode) and the message was recorded but not sent to a real person.
const (
	MsgQueued    MessageStatus = "queued"
	MsgSent      MessageStatus = "sent"
	MsgDelivered MessageStatus = "delivered"
	MsgFailed    MessageStatus = "failed"
	MsgSimulated MessageStatus = "simulated"
)

// OutboundMessage is the outbox: every message Bhagirath sends is recorded here
// first, then delivered by a channel adapter with retries.
type OutboundMessage struct {
	ID             int64           `json:"id"`
	Kind           string          `json:"kind"` // escalation | callback | advisory | followup | consent | ack | partner
	Channel        string          `json:"channel"`
	RecipientType  string          `json:"recipient_type"` // reporter | official
	RecipientID    int64           `json:"recipient_id"`
	RecipientLabel string          `json:"recipient_label"`
	Language       string          `json:"language"`
	Subject        string          `json:"subject,omitempty"`
	Body           string          `json:"body"`
	Link           string          `json:"link,omitempty"`
	Interactive    json.RawMessage `json:"interactive,omitempty"`
	Status         MessageStatus   `json:"status"`
	Attempts       int             `json:"attempts"`
	ProviderID     string          `json:"provider_id,omitempty"`
	Error          string          `json:"error,omitempty"`
	Entity         string          `json:"entity,omitempty"`
	EntityID       int64           `json:"entity_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	NextAttemptAt  time.Time       `json:"next_attempt_at"`
	SentAt         *time.Time      `json:"sent_at,omitempty"`
}

// Media is a stored blob (voice note, repair photo, lab report).
type Media struct {
	ID          int64      `json:"id"`
	SHA256      string     `json:"sha256"`
	ContentType string     `json:"content_type"`
	Size        int        `json:"size"`
	Data        []byte     `json:"-"`
	Kind        string     `json:"kind"` // audio | photo | document
	CreatedAt   time.Time  `json:"created_at"`
	DeleteAfter *time.Time `json:"delete_after,omitempty"`
}

// Event is an append-only audit record.
type Event struct {
	ID       int64           `json:"id"`
	At       time.Time       `json:"at"`
	Actor    string          `json:"actor"`
	Action   string          `json:"action"`
	Entity   string          `json:"entity"`
	EntityID string          `json:"entity_id"`
	WardID   string          `json:"ward_id,omitempty"`
	Details  json.RawMessage `json:"details,omitempty"`
}

// Outreach logs pilot reach (households informed) — entered by the partner.
type Outreach struct {
	ID         int64     `json:"id"`
	WardID     string    `json:"ward_id"`
	Date       time.Time `json:"date"`
	Households int       `json:"households"`
	Method     string    `json:"method"`
	Note       string    `json:"note"`
	LoggedBy   string    `json:"logged_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// Baseline is one observation of how long fixes took before Bhagirath.
type Baseline struct {
	ID        int64     `json:"id"`
	WardID    string    `json:"ward_id"`
	Source    string    `json:"source"` // resident_interview | helpline_data | partner
	DaysToFix float64   `json:"days_to_fix"`
	Resolved  bool      `json:"resolved"` // false = never fixed (counted as censored)
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// User is a dashboard login.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// Role controls dashboard permissions.
type Role string

// Roles, from most to least privileged.
const (
	RoleAdmin   Role = "admin"
	RolePartner Role = "partner"
	RoleJudge   Role = "judge" // demo deployments: read + simulator, no admin
	RoleViewer  Role = "viewer"
)

// Can reports whether the role may perform an action class.
func (r Role) Can(action string) bool {
	switch action {
	case "read":
		return r != ""
	case "operate": // transcripts, signals, acks, field verification, approvals
		return r == RoleAdmin || r == RolePartner
	case "simulate":
		return r == RoleAdmin || r == RolePartner || r == RoleJudge
	case "admin":
		return r == RoleAdmin
	}
	return false
}
