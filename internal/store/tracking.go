package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davideimola/salvo/internal/tracking"
)

// ChoiceKind is what a Tracking choice decided.
type ChoiceKind string

const (
	// Requested is tracking asked for and awaiting its confirmation click.
	Requested ChoiceKind = "requested"
	Given     ChoiceKind = "given"
	Refused   ChoiceKind = "refused"
	Withdrawn ChoiceKind = "withdrawn"
)

// ChoiceSource is where a Tracking choice was made.
type ChoiceSource string

const (
	// FromSubscribe is the subscribe form or API, and its confirmation click.
	FromSubscribe ChoiceSource = "subscribe"
	// FromPreferences is the Preferences page, and the tracking confirmation click.
	FromPreferences ChoiceSource = "preferences"
)

// TrackingChoice is one recorded decision about tracking. It is never changed once recorded.
type TrackingChoice struct {
	ID           int64
	SubscriberID int64
	// Subscription is the subscription it was made in (Subscriber.Subscription at the time).
	Subscription  int
	Kind          ChoiceKind
	At            time.Time
	Source        ChoiceSource
	NoticeVersion string
	// Ceiling is the Tracking ceiling in force when the choice was made.
	Ceiling tracking.Policy
}

const choiceColumns = `id, subscriber_id, subscription, kind, at, source, notice_version, opens_ceiling, links_ceiling`

func scanChoice(row pgx.Row) (*TrackingChoice, error) {
	var c TrackingChoice
	err := row.Scan(&c.ID, &c.SubscriberID, &c.Subscription, &c.Kind, &c.At, &c.Source, &c.NoticeVersion,
		&c.Ceiling.Opens, &c.Ceiling.Links)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

// AddTrackingChoice records a choice for the subscriber's current subscription. Kind, At,
// Source, NoticeVersion and Ceiling are taken from c; the rest is filled in.
func (s *Store) AddTrackingChoice(ctx context.Context, subscriberID int64, c TrackingChoice) (*TrackingChoice, error) {
	return scanChoice(s.pool.QueryRow(ctx, `INSERT INTO tracking_choices
		(subscriber_id, subscription, kind, at, source, notice_version, opens_ceiling, links_ceiling)
		SELECT id, subscription, $2, $3, $4, $5, $6, $7 FROM subscribers WHERE id = $1
		RETURNING `+choiceColumns,
		subscriberID, c.Kind, c.At, c.Source, c.NoticeVersion, c.Ceiling.Opens, c.Ceiling.Links))
}

// TrackingChoices returns every choice ever recorded for the subscriber, earlier
// subscriptions' included, oldest first.
func (s *Store) TrackingChoices(ctx context.Context, subscriberID int64) ([]TrackingChoice, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+choiceColumns+" FROM tracking_choices WHERE subscriber_id = $1 ORDER BY at, id", subscriberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackingChoice
	for rows.Next() {
		c, err := scanChoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// LatestTrackingChoice is the subscriber's state: the latest choice of their current
// subscription, or ErrNotFound when they have made none in it (no Tracking consent).
func (s *Store) LatestTrackingChoice(ctx context.Context, subscriberID int64) (*TrackingChoice, error) {
	return scanChoice(s.pool.QueryRow(ctx, `SELECT `+prefixed("c.", choiceColumns)+`
		FROM tracking_choices c JOIN subscribers s ON s.id = c.subscriber_id AND s.subscription = c.subscription
		WHERE c.subscriber_id = $1 ORDER BY c.at DESC, c.id DESC LIMIT 1`, subscriberID))
}

// LatestTrackingRequest is the latest `requested` choice of the subscriber's current
// subscription, or ErrNotFound.
func (s *Store) LatestTrackingRequest(ctx context.Context, subscriberID int64) (*TrackingChoice, error) {
	return scanChoice(s.pool.QueryRow(ctx, `SELECT `+prefixed("c.", choiceColumns)+`
		FROM tracking_choices c JOIN subscribers s ON s.id = c.subscriber_id AND s.subscription = c.subscription
		WHERE c.subscriber_id = $1 AND c.kind = 'requested' ORDER BY c.at DESC, c.id DESC LIMIT 1`, subscriberID))
}

// SetConfirmTracking records whether the latest request, the one the pending confirmation
// link answers, asked for tracking.
func (s *Store) SetConfirmTracking(ctx context.Context, id int64, wantsTracking bool) error {
	_, err := s.pool.Exec(ctx, "UPDATE subscribers SET confirm_tracking = $2 WHERE id = $1 AND status = 'pending'", id, wantsTracking)
	return err
}

// NoticeConsents is how many current Tracking consents were given under one Notice version.
type NoticeConsents struct {
	NoticeVersion string
	Consents      int
}

// ConsentsByNotice counts the current Tracking consents per Notice version, oldest version
// first: confirmed Subscribers whose latest choice of their current subscription is `given`.
func (s *Store) ConsentsByNotice(ctx context.Context) ([]NoticeConsents, error) {
	rows, err := s.pool.Query(ctx, `SELECT notice_version, count(*) FROM (
		SELECT DISTINCT ON (c.subscriber_id) c.kind, c.notice_version, c.at
		FROM tracking_choices c JOIN subscribers s ON s.id = c.subscriber_id AND s.subscription = c.subscription
		WHERE s.status = 'confirmed'
		ORDER BY c.subscriber_id, c.at DESC, c.id DESC
	) latest WHERE kind = 'given' GROUP BY notice_version ORDER BY min(at), notice_version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NoticeConsents
	for rows.Next() {
		var n NoticeConsents
		if err := rows.Scan(&n.NoticeVersion, &n.Consents); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SubscriberByTrackingConfirmHash returns the subscriber a tracking confirmation link was
// mailed to.
func (s *Store) SubscriberByTrackingConfirmHash(ctx context.Context, hash []byte) (*Subscriber, error) {
	return scanSubscriber(s.pool.QueryRow(ctx, "SELECT "+subscriberColumns+" FROM subscribers WHERE tracking_confirm_hash = $1", hash))
}

// RequestTracking records a confirmed subscriber's `requested` choice and, in the same step,
// arms the tracking confirmation link that answers it, replacing any earlier one. A
// subscriber no longer confirmed records nothing and is ErrNotFound.
func (s *Store) RequestTracking(ctx context.Context, id int64, confirmHash []byte, c TrackingChoice) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE subscribers SET tracking_confirm_hash = $2, tracking_confirm_sent_at = $3
			WHERE id = $1 AND status = 'confirmed'`, id, confirmHash, c.At)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return insertChoice(ctx, tx, id, c)
	})
}

// ClearTrackingConfirmSent forgets that a tracking confirmation went out, after Kannon refused
// to send it, so the next request sends one rather than waiting out the cooldown.
func (s *Store) ClearTrackingConfirmSent(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, "UPDATE subscribers SET tracking_confirm_sent_at = NULL WHERE id = $1", id)
	return err
}

// GiveTracking records the `given` choice of a tracking confirmation click and, in the same
// step, spends the link so it works once. A link already spent, or a subscriber no longer
// confirmed, records nothing and is ErrNotFound.
func (s *Store) GiveTracking(ctx context.Context, id int64, confirmHash []byte, c TrackingChoice) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE subscribers SET tracking_confirm_hash = NULL
			WHERE id = $1 AND tracking_confirm_hash = $2 AND status = 'confirmed'`, id, confirmHash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return insertChoice(ctx, tx, id, c)
	})
}

func insertChoice(ctx context.Context, tx pgx.Tx, subscriberID int64, c TrackingChoice) error {
	_, err := tx.Exec(ctx, `INSERT INTO tracking_choices
		(subscriber_id, subscription, kind, at, source, notice_version, opens_ceiling, links_ceiling)
		SELECT id, subscription, $2, $3, $4, $5, $6, $7 FROM subscribers WHERE id = $1`,
		subscriberID, c.Kind, c.At, c.Source, c.NoticeVersion, c.Ceiling.Opens, c.Ceiling.Links)
	return err
}
