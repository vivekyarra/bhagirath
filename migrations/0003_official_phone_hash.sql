-- Officials can acknowledge escalations by replying on WhatsApp; their
-- numbers are matched by keyed hash, never stored in plain text.
ALTER TABLE official ADD COLUMN phone_hash TEXT;
CREATE INDEX official_phone_hash_idx ON official (phone_hash) WHERE phone_hash IS NOT NULL;
