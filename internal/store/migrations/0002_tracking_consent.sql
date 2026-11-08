-- Which subscription a row is on: 1 for the first, one more each time an unsubscribed address
-- subscribes again. A Tracking choice belongs to the subscription it was made in and is never
-- inherited by a later one.
ALTER TABLE subscribers ADD COLUMN subscription integer NOT NULL DEFAULT 1;
-- Whether the request the pending confirmation link answers asked for tracking. The latest
-- request is the one the click confirms.
ALTER TABLE subscribers ADD COLUMN confirm_tracking boolean NOT NULL DEFAULT false;

-- Every Tracking choice, the proof of a Tracking consent and of every later decision. Rows are
-- only ever added: the trigger below refuses an update. Erasure deletes them with the
-- subscriber.
CREATE TABLE tracking_choices (
    id             bigserial   PRIMARY KEY,
    subscriber_id  bigint      NOT NULL REFERENCES subscribers (id) ON DELETE CASCADE,
    subscription   integer     NOT NULL,
    kind           text        NOT NULL CHECK (kind IN ('requested', 'given', 'refused', 'withdrawn')),
    at             timestamptz NOT NULL,
    -- `subscribe` for the subscribe form or API, `preferences` for the Preferences page.
    source         text        NOT NULL CHECK (source IN ('subscribe', 'preferences')),
    notice_version text        NOT NULL,
    -- The Tracking ceiling in force, for opens and for links.
    opens_ceiling  text        NOT NULL CHECK (opens_ceiling IN ('off', 'pseudonymous')),
    links_ceiling  text        NOT NULL CHECK (links_ceiling IN ('off', 'pseudonymous'))
);

CREATE INDEX tracking_choices_subscriber ON tracking_choices (subscriber_id, subscription, at, id);

CREATE FUNCTION tracking_choices_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'tracking choices are append-only';
END;
$$;

CREATE TRIGGER tracking_choices_append_only BEFORE UPDATE ON tracking_choices
    FOR EACH ROW EXECUTE FUNCTION tracking_choices_append_only();

-- The Subscribers with Tracking consent an issue was sent to: the base of its open and click
-- shares.
ALTER TABLE issues ADD COLUMN tracked_recipients integer;
