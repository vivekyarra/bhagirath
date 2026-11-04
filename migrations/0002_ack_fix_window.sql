-- Acknowledged complaint escalations keep a running "fix window" timer, so the
-- timer index must cover them too.
DROP INDEX IF EXISTS escalation_open_deadline_idx;
CREATE INDEX escalation_open_deadline_idx ON escalation (deadline_at) WHERE outcome IN ('pending','acknowledged');
