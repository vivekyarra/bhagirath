-- Bhagirath schema v1.
-- Coordinates are stored as plain lat/lon; spatial logic (300 m clustering,
-- point-in-polygon) lives in Go (internal/geo) so the rule engine is identical
-- in production and in the backtest, and so any managed Postgres works.

CREATE TABLE ward (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    city_or_block    TEXT NOT NULL DEFAULT '',
    district         TEXT NOT NULL DEFAULT '',
    state            TEXT NOT NULL DEFAULT '',
    polygon          JSONB NOT NULL,
    centroid_lat     DOUBLE PRECISION NOT NULL,
    centroid_lon     DOUBLE PRECISION NOT NULL,
    default_language TEXT NOT NULL DEFAULT 'hi',
    landmarks        JSONB NOT NULL DEFAULT '[]',
    partner_name     TEXT NOT NULL DEFAULT '',
    demo             BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE media (
    id           BIGSERIAL PRIMARY KEY,
    sha256       TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size         INTEGER NOT NULL,
    data         BYTEA,
    kind         TEXT NOT NULL CHECK (kind IN ('audio','photo','document')),
    created_at   TIMESTAMPTZ NOT NULL,
    delete_after TIMESTAMPTZ
);

CREATE TABLE reporter (
    id                 BIGSERIAL PRIMARY KEY,
    ref                TEXT NOT NULL UNIQUE,
    phone_hash         TEXT UNIQUE,
    phone_enc          BYTEA,
    kind               TEXT NOT NULL DEFAULT 'resident'
                       CHECK (kind IN ('resident','clinic','chemist','asha','hospital','volunteer')),
    label              TEXT NOT NULL DEFAULT '',
    preferred_language TEXT NOT NULL DEFAULT 'hi',
    preferred_channel  TEXT NOT NULL DEFAULT 'voice' CHECK (preferred_channel IN ('voice','whatsapp')),
    ward_id            TEXT REFERENCES ward(id),
    home_lat           DOUBLE PRECISION,
    home_lon           DOUBLE PRECISION,
    has_smartphone     BOOLEAN,
    consent_at         TIMESTAMPTZ,
    consent_asked_at   TIMESTAMPTZ,
    consent_declined   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at         TIMESTAMPTZ NOT NULL
);
CREATE INDEX reporter_ward_idx ON reporter (ward_id) WHERE consent_at IS NOT NULL;

CREATE TABLE complaint (
    id                BIGSERIAL PRIMARY KEY,
    ref               TEXT NOT NULL UNIQUE,
    reporter_id       BIGINT REFERENCES reporter(id),
    channel           TEXT NOT NULL CHECK (channel IN ('missed_call','ivr','whatsapp','web','volunteer')),
    provider_msg_id   TEXT,
    audio_media_id    BIGINT REFERENCES media(id) ON DELETE SET NULL,
    transcript        TEXT NOT NULL DEFAULT '',
    transcript_asr    TEXT NOT NULL DEFAULT '',
    transcript_status TEXT NOT NULL DEFAULT 'none'
                      CHECK (transcript_status IN ('none','needs_review','asr','reviewed')),
    language          TEXT NOT NULL DEFAULT '',
    issue_type        TEXT NOT NULL DEFAULT 'other'
                      CHECK (issue_type IN ('dirty_water','foul_smell','sewage_mixing','no_water','leak','illness','other')),
    severity          SMALLINT NOT NULL DEFAULT 1 CHECK (severity BETWEEN 1 AND 5),
    description_en    TEXT NOT NULL DEFAULT '',
    lat               DOUBLE PRECISION,
    lon               DOUBLE PRECISION,
    location_text     TEXT NOT NULL DEFAULT '',
    location_method   TEXT NOT NULL DEFAULT 'unresolved'
                      CHECK (location_method IN ('gps','landmark','registered_address','manual','ward_only','unresolved')),
    ward_id           TEXT REFERENCES ward(id),
    status            TEXT NOT NULL DEFAULT 'open'
                      CHECK (status IN ('open','escalated','claimed_fixed','verified_fixed','reopened','invalid')),
    duplicate_of      BIGINT REFERENCES complaint(id),
    needs_followup    BOOLEAN NOT NULL DEFAULT FALSE,
    extraction        JSONB NOT NULL DEFAULT '{}',
    reopen_count      INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,
    verified_fixed_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX complaint_provider_msg_idx ON complaint (channel, provider_msg_id) WHERE provider_msg_id IS NOT NULL;
CREATE INDEX complaint_ward_created_idx ON complaint (ward_id, created_at);
CREATE INDEX complaint_status_idx ON complaint (status);
CREATE INDEX complaint_reporter_idx ON complaint (reporter_id, created_at);

CREATE TABLE water_test (
    id              BIGSERIAL PRIMARY KEY,
    ref             TEXT NOT NULL UNIQUE,
    source          TEXT NOT NULL CHECK (source IN ('jjm_public','lab_report','field_test_kit','partner_test')),
    lat             DOUBLE PRECISION,
    lon             DOUBLE PRECISION,
    ward_id         TEXT NOT NULL REFERENCES ward(id),
    sampled_at      TIMESTAMPTZ NOT NULL,
    parameter       TEXT NOT NULL,
    value           DOUBLE PRECISION,
    unit            TEXT NOT NULL DEFAULT '',
    limit_value     DOUBLE PRECISION,
    is_contaminated BOOLEAN NOT NULL,
    evidence_media  BIGINT REFERENCES media(id) ON DELETE SET NULL,
    evidence_url    TEXT NOT NULL DEFAULT '',
    note            TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    created_by      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX water_test_ward_idx ON water_test (ward_id, sampled_at);

CREATE TABLE illness_report (
    id              BIGSERIAL PRIMARY KEY,
    ref             TEXT NOT NULL UNIQUE,
    reporter_type   TEXT NOT NULL CHECK (reporter_type IN ('clinic','chemist','asha','hospital','resident')),
    reporter_id     BIGINT REFERENCES reporter(id),
    ward_id         TEXT NOT NULL REFERENCES ward(id),
    lat             DOUBLE PRECISION,
    lon             DOUBLE PRECISION,
    reported_at     TIMESTAMPTZ NOT NULL,
    cases_diarrhoea INTEGER NOT NULL DEFAULT 0 CHECK (cases_diarrhoea >= 0 AND cases_diarrhoea <= 10000),
    cases_vomiting  INTEGER NOT NULL DEFAULT 0 CHECK (cases_vomiting >= 0 AND cases_vomiting <= 10000),
    ors_units_sold  INTEGER CHECK (ors_units_sold IS NULL OR (ors_units_sold >= 0 AND ors_units_sold <= 100000)),
    notes           TEXT NOT NULL DEFAULT '',
    complaint_id    BIGINT REFERENCES complaint(id),
    created_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX illness_ward_idx ON illness_report (ward_id, reported_at);

CREATE TABLE alert (
    id                   BIGSERIAL PRIMARY KEY,
    ward_id              TEXT NOT NULL REFERENCES ward(id),
    level                TEXT NOT NULL CHECK (level IN ('watch','warning','critical')),
    peak_level           TEXT NOT NULL CHECK (peak_level IN ('watch','warning','critical')),
    rule_version         TEXT NOT NULL,
    rule_sha256          TEXT NOT NULL,
    triggered_at         TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL,
    resolved_at          TIMESTAMPTZ,
    signals              JSONB NOT NULL,
    reason_text          TEXT NOT NULL,
    reason_local         TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL CHECK (status IN ('active','resolved')),
    advisory_approved_at TIMESTAMPTZ,
    advisory_approved_by TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX alert_one_active_per_ward ON alert (ward_id) WHERE status = 'active';

CREATE TABLE alert_transition (
    id          BIGSERIAL PRIMARY KEY,
    alert_id    BIGINT NOT NULL REFERENCES alert(id),
    at          TIMESTAMPTZ NOT NULL,
    from_level  TEXT NOT NULL,
    to_level    TEXT NOT NULL,
    reason_text TEXT NOT NULL,
    signals     JSONB NOT NULL
);
CREATE INDEX alert_transition_alert_idx ON alert_transition (alert_id, at);

CREATE TABLE official (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL,
    role              TEXT NOT NULL,
    department        TEXT NOT NULL DEFAULT '',
    ward_ids          TEXT[] NOT NULL,
    phone_enc         BYTEA,
    email             TEXT NOT NULL DEFAULT '',
    ladder_level      TEXT NOT NULL CHECK (ladder_level IN ('1','2','3','H','P')),
    grievance_channel TEXT NOT NULL DEFAULT '',
    preferred_channel TEXT NOT NULL DEFAULT 'email' CHECK (preferred_channel IN ('email','whatsapp','grievance')),
    language          TEXT NOT NULL DEFAULT 'en',
    active            BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE escalation (
    id               BIGSERIAL PRIMARY KEY,
    ref              TEXT NOT NULL UNIQUE,
    alert_id         BIGINT REFERENCES alert(id),
    complaint_id     BIGINT REFERENCES complaint(id),
    official_id      BIGINT NOT NULL REFERENCES official(id),
    level            TEXT NOT NULL CHECK (level IN ('1','2','3','H','P')),
    ward_id          TEXT NOT NULL REFERENCES ward(id),
    sent_at          TIMESTAMPTZ NOT NULL,
    channel          TEXT NOT NULL,
    token_hash       TEXT NOT NULL UNIQUE,
    deadline_at      TIMESTAMPTZ NOT NULL,
    acknowledged_at  TIMESTAMPTZ,
    ack_via          TEXT NOT NULL DEFAULT '',
    grievance_number TEXT NOT NULL DEFAULT '',
    outcome          TEXT NOT NULL CHECK (outcome IN ('pending','acknowledged','timed_out','resolved','closed')),
    parent_id        BIGINT REFERENCES escalation(id),
    reason           TEXT NOT NULL,
    closed_at        TIMESTAMPTZ,
    CHECK (alert_id IS NOT NULL OR complaint_id IS NOT NULL)
);
CREATE INDEX escalation_open_deadline_idx ON escalation (deadline_at) WHERE outcome = 'pending';
CREATE INDEX escalation_alert_idx ON escalation (alert_id);
CREATE INDEX escalation_complaint_idx ON escalation (complaint_id);

CREATE TABLE closure (
    id                BIGSERIAL PRIMARY KEY,
    complaint_id      BIGINT NOT NULL REFERENCES complaint(id),
    claimed_at        TIMESTAMPTZ NOT NULL,
    claimed_by        TEXT NOT NULL,
    escalation_id     BIGINT REFERENCES escalation(id),
    evidence_type     TEXT NOT NULL CHECK (evidence_type IN ('photo','retest','both')),
    evidence_media_id BIGINT REFERENCES media(id) ON DELETE SET NULL,
    retest_id         BIGINT REFERENCES water_test(id),
    note              TEXT NOT NULL DEFAULT '',
    callback_attempts INTEGER NOT NULL DEFAULT 0,
    next_callback_at  TIMESTAMPTZ,
    callback_at       TIMESTAMPTZ,
    callback_result   TEXT NOT NULL DEFAULT 'pending' CHECK (callback_result IN ('pending','confirmed','denied','no_answer')),
    callback_channel  TEXT NOT NULL DEFAULT '',
    verified          BOOLEAN NOT NULL DEFAULT FALSE,
    decided_at        TIMESTAMPTZ,
    CHECK (evidence_media_id IS NOT NULL OR retest_id IS NOT NULL)
);
CREATE UNIQUE INDEX closure_one_pending_per_complaint ON closure (complaint_id) WHERE decided_at IS NULL;
CREATE INDEX closure_callback_due_idx ON closure (next_callback_at) WHERE decided_at IS NULL;

CREATE TABLE outbound_message (
    id              BIGSERIAL PRIMARY KEY,
    kind            TEXT NOT NULL,
    channel         TEXT NOT NULL,
    recipient_type  TEXT NOT NULL CHECK (recipient_type IN ('reporter','official','partner')),
    recipient_id    BIGINT NOT NULL DEFAULT 0,
    recipient_label TEXT NOT NULL DEFAULT '',
    language        TEXT NOT NULL DEFAULT 'en',
    subject         TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL,
    link            TEXT NOT NULL DEFAULT '',
    interactive     JSONB,
    status          TEXT NOT NULL CHECK (status IN ('queued','sent','delivered','failed','simulated')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    provider_id     TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    entity          TEXT NOT NULL DEFAULT '',
    entity_id       BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    sent_at         TIMESTAMPTZ
);
CREATE INDEX outbound_due_idx ON outbound_message (next_attempt_at) WHERE status = 'queued';
CREATE INDEX outbound_provider_idx ON outbound_message (provider_id) WHERE provider_id <> '';

CREATE TABLE advisory (
    id          BIGSERIAL PRIMARY KEY,
    alert_id    BIGINT NOT NULL REFERENCES alert(id),
    reporter_id BIGINT NOT NULL REFERENCES reporter(id),
    level       TEXT NOT NULL,
    language    TEXT NOT NULL,
    channel     TEXT NOT NULL,
    sent_at     TIMESTAMPTZ NOT NULL,
    delivered   BOOLEAN NOT NULL DEFAULT FALSE,
    message_id  BIGINT REFERENCES outbound_message(id),
    text        TEXT NOT NULL,
    UNIQUE (alert_id, reporter_id, level)
);

CREATE TABLE event_log (
    id        BIGSERIAL PRIMARY KEY,
    at        TIMESTAMPTZ NOT NULL,
    actor     TEXT NOT NULL,
    action    TEXT NOT NULL,
    entity    TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    ward_id   TEXT,
    details   JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX event_log_at_idx ON event_log (at DESC);
CREATE INDEX event_log_action_idx ON event_log (action);

CREATE TABLE outreach (
    id         BIGSERIAL PRIMARY KEY,
    ward_id    TEXT NOT NULL REFERENCES ward(id),
    date       DATE NOT NULL,
    households INTEGER NOT NULL CHECK (households >= 0 AND households <= 100000),
    method     TEXT NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    logged_by  TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE baseline (
    id          BIGSERIAL PRIMARY KEY,
    ward_id     TEXT NOT NULL REFERENCES ward(id),
    source      TEXT NOT NULL CHECK (source IN ('resident_interview','helpline_data','partner')),
    days_to_fix DOUBLE PRECISION NOT NULL CHECK (days_to_fix >= 0),
    resolved    BOOLEAN NOT NULL DEFAULT TRUE,
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE app_user (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin','partner','judge','viewer')),
    created_at    TIMESTAMPTZ NOT NULL
);

-- Conversation state for multi-step channels (follow-up questions, IVR).
CREATE TABLE conversation (
    reporter_id  BIGINT PRIMARY KEY REFERENCES reporter(id),
    state        TEXT NOT NULL,
    complaint_id BIGINT REFERENCES complaint(id),
    closure_id   BIGINT REFERENCES closure(id),
    expires_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);

-- Webhook idempotency: providers retry, we must not double-log complaints.
CREATE TABLE inbound_seen (
    provider   TEXT NOT NULL,
    message_id TEXT NOT NULL,
    at         TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (provider, message_id)
);

-- Persistent key/value (demo clock offset, job leases).
CREATE TABLE kv (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE SEQUENCE complaint_ref_seq;
CREATE SEQUENCE escalation_ref_seq;
CREATE SEQUENCE test_ref_seq;
CREATE SEQUENCE illness_ref_seq;
