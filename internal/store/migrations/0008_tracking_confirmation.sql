-- The tracking confirmation link a confirmed Subscriber is mailed after asking for tracking,
-- stored only as a hash, and when it was sent: it expires and is rate-limited like the
-- confirmation link. Cleared by its click, so it works once.
ALTER TABLE subscribers ADD COLUMN tracking_confirm_hash bytea UNIQUE;
ALTER TABLE subscribers ADD COLUMN tracking_confirm_sent_at timestamptz;
