<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/lockup-dark.svg">
    <img src="assets/lockup.svg" alt="Salvo" width="330">
  </picture>
</p>

# Salvo

**A self-hosted newsletter that doesn't watch its readers.** Salvo keeps your subscribers, runs
the double opt-in and sends each issue through [Kannon](https://github.com/kannon-email/kannon),
with no tracking pixel and no tracked links.

A cannon fires a salvo, and Kannon is the cannon. In Italian *salvo* also means safe, which is
what your readers' inboxes stay. The mark is an S drawn as a lit fuse.

## Why

Most newsletter tools put a pixel in every email, unique to each reader, so they can tell you
who opened what and when. In Italy, the data protection authority's guidelines of 17 April 2026
treat that pixel like a cookie: it needs the reader's prior, informed consent.

Salvo starts from the other end: it tracks nothing. Every send tells Kannon so, whatever the
Domain allows, and the admin page warns you if Kannon ever counts an open or a click anyway.
You still see what matters: how many emails were delivered and how many bounced.

That stays the default. If you do want to know how an issue did, you can raise a Tracking
ceiling and measure the readers who agree to it, and only them: see
[Tracking and Tracking consent](#tracking-and-tracking-consent) before you do.

It began as the newsletter of [davideimola.dev](https://davideimola.dev), replacing Kit, which
cannot send without a per-reader pixel. The story of that migration is
[davideimola/davideimola.dev#132](https://github.com/davideimola/davideimola.dev/issues/132).

## How it fits together

```
your website ---API_KEY---> salvo ----> Postgres (subscribers, issues)
                              ^  |
reader's mail client          |  v
  /confirm, /unsubscribe, ----+  Kannon (tracking off) ----> SMTP
  /preferences                |
                              |
you, signed in with Google    |
  /admin ---------------------+
```

- **Your website** owns the content: it renders each issue's HTML and posts subscribe requests
  to Salvo's API.
- **Salvo** owns the list: who subscribed, when they confirmed, who left. It decides who gets
  an issue and when.
- **Kannon** only delivers. It keeps no list, and every send from Salvo states tracking `off`,
  on top of the Domain's own setting. With a Tracking ceiling raised, Salvo states each reader's
  policy instead, `off` for everyone who did not agree.

## What it does

- **Double opt-in.** Subscribing creates a `pending` address and mails a confirmation link. The
  address is `confirmed` only when that link is clicked, and the time of the click is your proof
  of consent.
- **A form that reveals nothing.** New, waiting, confirmed or unsubscribed: every address gets
  the same answer, so nobody can probe your list.
- **No mailbox flooding.** At most one confirmation email per address every 10 minutes.
- **Expiry.** A confirmation link works for 7 days, and an address that never confirms is
  deleted after 30.
- **Safe tokens.** Every token is 32 random bytes, never derived from the address, and no URL
  carries an address. Only the hash of the confirmation token is stored.
- **One-click unsubscribe.** Every issue carries `List-Unsubscribe` and `List-Unsubscribe-Post`
  (RFC 8058), so the mail client's own unsubscribe button works with no further step. The
  link in the body opens a page with one button: mail scanners open every link, and a GET must
  not unsubscribe anyone.
- **Unsubscribed means never again,** unless the address subscribes and confirms again.
- **Erasure on request.** One button on the subscriber's admin page, or `salvo erase EMAIL`.
  Nothing about the address is left in Salvo, its Tracking choices included.
- **An issue goes out once.** Your website can save an issue and send it to you as a test, but
  only the admin page sends it to the list. You type the issue's slug to confirm, and only after
  testing its latest version.
- **No double sends.** If Kannon refuses a send, nothing went out and the issue goes back to
  draft. If the connection drops halfway, the issue stays `sending` with the error shown, and
  Salvo does not retry, because retrying could mail everybody twice.
- **An admin page.** Subscribers, issues, preview, test, send, delete, and Kannon's delivery
  counters. If Kannon ever counts an open or a click, the page warns you.
- **Tracking, off by default.** Off until you raise a Tracking ceiling, and then only for
  the Subscribers who gave a Tracking consent by a click. See [below](#tracking-and-tracking-consent).

The rules live in `internal/newsletter`, and its tests are the specification.

## Tracking and Tracking consent

Salvo tracks nothing unless you raise a Tracking ceiling, and with both ceilings `off`
everything above holds exactly as written. Raising one makes you answerable for more than a
setting: read this section first.

### The Tracking ceilings

`TRACKING_OPENS` and `TRACKING_LINKS` are the Tracking ceilings for opens and for links, set
separately, each `off` or `pseudonymous`. Measuring only clicks is a sound choice: Apple Mail
Privacy Protection fetches every image for its users, so opens are inflated anyway.

`pseudonymous` is the only mode on offer. Kannon tracks each recipient under a random identity
drawn for that one delivery and stored nowhere, so you learn how many distinct readers opened or
clicked an issue, never who. Kannon's `anonymous` mode cannot tell one issue from another, and
`identified` puts the address in every tracking URL; Salvo accepts neither, nor `full`, and
refuses to start on any of them ([ADR 0002](docs/adr/0002-pseudonymous-only-no-anonymous-no-identified.md)).

While a ceiling is raised, `NOTICE_VERSION` is required: it names the privacy notice in force,
and it is recorded with every Tracking choice. Without it Salvo refuses to start.

A Subscriber is never tracked beyond the ceiling in force when they gave their Tracking consent.
Each send uses the lower of that ceiling and the current one, so lowering a ceiling takes effect
for everyone at the next issue, and raising it reaches nobody who agreed to less.

### How a Subscriber gives Tracking consent

Only a click in an email gives it. A box or a button alone would let anyone who types an address
into a form, or holds a forwarded issue, consent for someone else
([ADR 0001](docs/adr/0001-salvo-holds-tracking-consent-and-confirms-it-by-click.md)).

- **When subscribing.** Your form shows a separate, optional, unticked box, and its backend sends
  `"tracking": true` to `POST /api/subscribers` when it is ticked. The confirmation email says
  what the newsletter measures and that confirming also agrees to it, and the one click
  confirms both. A Subscriber who confirms without the box has their refusal recorded. If a
  pending address submits the form again with a different choice, the latest one is what the
  click confirms. Sending `"tracking": true` while both ceilings are `off` gets a `400`, so a
  checkbox that would do nothing is noticed at once. Salvo hosts no subscribe form yet: the
  box is your form's.
- **Already on the list.** The form with the box ticked, or the button on the Preferences page,
  mails a tracking confirmation email, and its link, `/confirm-tracking`, gives the Tracking
  consent. The link works once and for 7 days, is stored only as a hash, and goes out at most
  once every 10 minutes, like the confirmation link. The form with the box unticked changes
  nothing: a blank box never withdraws a consent.

Both emails are themselves never tracked, and neither are test sends.

A Tracking consent belongs to the subscription it was given with. A Subscriber who unsubscribes
and later subscribes again starts with none. Subscribers from before tracking existed, and those
imported from Kit, have none either.

### The proof

Every Tracking choice is a new row, never an update: requested, given, refused or withdrawn,
with when, where (the subscribe form or API, or the Preferences page), the Notice version and
both ceilings in force. A Subscriber's admin page lists them all, earlier subscriptions included
and marked as such, so you can answer them or a regulator. The overview counts the current
Tracking consents by Notice version.

### The Preferences page and Withdraw

Write `{{ preferences_url }}` in every issue, next to the unsubscribe link: while a ceiling is
raised Salvo refuses to save an issue without it. Kannon fills it in for each reader, like the
unsubscribe link, and with the same token.

The Preferences page says plainly whether the reader's opens and clicks are measured, and has
one button: Withdraw for a reader with a Tracking consent, or ask for one for a reader without.
Opening it changes nothing, because mail scanners open every link. Withdraw ends the Tracking
consent and keeps the subscription; unsubscribing is a separate link from the page. With both
ceilings `off` the page says the newsletter tracks nothing.

Withdraw stops tracking from the next issue. It does not reach mail Kannon has already accepted,
whose policy Kannon fixed when it took the send.

### Sending

Every issue still goes out as one batch. Salvo sets the batch's policy to the ceilings and every
recipient's own policy explicitly: their consent's, held to the ceilings, or `off`. Kannon
applies the most restrictive of Domain, batch and recipient, so a Domain set too high by mistake
still tracks nobody who did not agree.

Set the Domain's own tracking policy in Kannon to at least the ceilings: Kannon rejects, one by
one, every recipient above it. So while a ceiling is raised, every send first reads the Domain's
policy with `KANNON_ADMIN_TOKEN` and is refused before anything goes out when the Domain allows
less, when Kannon does not answer, or when no admin token is set. The issue goes back to draft
with the reason, and `/admin` warns about the mismatch before you try.

The issue stores its Tracked recipients: how many of its recipients were actually tracked.

### What you see

With `KANNON_ADMIN_TOKEN` set, each sent issue's page shows its unique opens and unique clicks
as a share of its Tracked recipients, never of the whole list, with how many recipients were
not measured beside them and a note on Apple Mail Privacy Protection. If Kannon counts more
distinct openers or clickers than there were Tracked recipients, someone was measured without
a Tracking consent, and the page warns you. If Kannon does not answer, the page says so and
keeps working.

The figures are read from Kannon's event API each time the page opens. Salvo stores no tracking
events, only each issue's Tracked recipients
([ADR 0003](docs/adr/0003-salvo-stores-no-tracking-events.md)).

### What you answer for

Salvo records the Tracking consent and honours it; whether your measurement is lawful is still
yours to answer for.

- **Your privacy notice.** It must say what is measured, through Kannon, for how long, and how
  to withdraw. Link it with `PRIVACY_URL`, and change `NOTICE_VERSION` whenever the notice
  changes. Salvo does not ask for consent again after a change: the overview's count per Notice
  version tells you how many consents were given under an older one.
- **Kannon's Domain policy.** It is yours to set, as above. Salvo checks it before every send and
  states each recipient's policy anyway.
- **Kannon's event retention.** Kannon keeps each event for `stats.retention`, one year by
  default. Set it to 90 days (`2160h`), the life of a tracked link: Kannon's tracking tokens
  expire after three months, so later events are not recorded anyway.
- **Erasure.** `salvo erase` deletes the address and every Tracking choice from Salvo. It does
  not reach Kannon's copy of the events, which stays until Kannon's retention removes it:
  Kannon cannot erase one recipient. The events carry the pseudonym, not the address, but
  tell a Subscriber who asks the truth.
- **Kannon's version.** Tracking needs a Kannon with per-recipient Tracking Policies and
  the `v1beta1` event API: Kannon `main`, which Salvo already depends on.

## Run it

You need:

- **Postgres**. The tests run on 17.
- **Kannon**, with a Domain whose tracking policy is `off` for opens and links, and an API key
  for that Domain. With a Tracking ceiling raised, the Domain must allow at least the ceiling
  instead, and `KANNON_ADMIN_TOKEN` must be set.
- **A Google OAuth client** for the admin page, with `PUBLIC_URL/auth/callback` as its redirect
  URI. Only `OWNER_EMAIL` gets in.

The image is `ghcr.io/davideimola/salvo`. Migrations are embedded in the binary: run them once,
then serve.

```sh
docker run --rm --env-file salvo.env ghcr.io/davideimola/salvo:main migrate
docker run -d -p 8080:8080 --env-file salvo.env ghcr.io/davideimola/salvo:main
```

### Wire up your website

Subscribe a reader from your form's backend, never from the browser, because the API key is a
secret:

```sh
curl -X POST "$PUBLIC_URL/api/subscribers" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"email":"reader@example.org"}'
```

Save an issue, then send it to yourself as a test:

```sh
curl -X PUT "$PUBLIC_URL/api/issues/2026-11" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"subject":"November","html":"<p>Hello.</p><a href=\"{{ unsubscribe_url }}\">Unsubscribe</a>"}'

curl -X POST "$PUBLIC_URL/api/issues/2026-11/test" -H "Authorization: Bearer $API_KEY"
```

The HTML is yours to render, footer included. Wherever a reader's unsubscribe link goes, write
`{{ unsubscribe_url }}`, and Kannon fills it in for each reader. Salvo refuses an issue without
it. Likewise `{{ preferences_url }}` is each reader's [Preferences page](#the-preferences-page-and-withdraw):
optional while both Tracking ceilings are `off`, required as soon as one is raised. When the
test looks right, open `/admin` and send it.

## HTTP

| Route | Who | What |
|---|---|---|
| `POST /api/subscribers` `{"email","tracking"}` | website, `Authorization: Bearer API_KEY` | start the double opt-in, asking for a Tracking consent too when the optional `tracking` is `true`; for an address already confirmed, `tracking: true` mails the tracking confirmation email instead. `202` whatever the address's state; `400` `invalid_email`, or `tracking_off` for `tracking: true` while both ceilings are `off` |
| `PUT /api/issues/{slug}` `{"subject","html"}` | website | save or replace a draft; the HTML must contain `{{ unsubscribe_url }}` (else `400` `no_unsubscribe_url`), and `{{ preferences_url }}` while a ceiling is raised (else `400` `no_preferences_url`) |
| `POST /api/issues/{slug}/test` | website | send the draft to `TEST_RECIPIENT` |
| `GET /confirm?token=` | reader | confirm, then redirect to `CONFIRMED_URL` |
| `GET /confirmed` | reader | Salvo's own "you're in" page, where `CONFIRMED_URL` points by default |
| `GET /confirm-tracking?token=` | reader | the link in the tracking confirmation email: gives the Tracking consent; `404` for a used or unknown link, `410` once expired |
| `GET /unsubscribe?token=` | reader | a page with an Unsubscribe button |
| `POST /unsubscribe?token=` | reader, mail client | unsubscribe; `List-Unsubscribe=One-Click` is the RFC 8058 form |
| `GET /preferences?token=` | reader | the Preferences page, with the unsubscribe link's token: whether opens and clicks are measured; changes nothing |
| `POST /preferences` `token`, `action=withdraw` | reader | Withdraw the Tracking consent and stay subscribed |
| `POST /preferences` `token`, `action=allow` | reader | mail the tracking confirmation email; `400` while both ceilings are `off` |
| `/admin` | owner, signed in with Google | subscribers and their Tracking choices, issues, preview, test, send, delete, delivery counters, opens and clicks among Tracked recipients |
| `GET /healthz` | your orchestrator | `200` when the database answers |

## Configuration

Environment variables. All are required except `LISTEN_ADDR`, `KANNON_ADMIN_TOKEN` and the
ones under [What readers see](#what-readers-see) and [Tracking](#tracking).

| Variable | What it is |
|---|---|
| `LISTEN_ADDR` | where to listen, default `:8080` |
| `DATABASE_URL` | `postgres://...` |
| `PUBLIC_URL` | Salvo's own origin, the base of every link in an email, e.g. `https://news.example.org` |
| `SITE_URL` | your website's origin, e.g. `https://example.org` |
| `API_KEY` | what your website sends as a bearer token, at least 32 characters |
| `KANNON_URL` | Kannon's API, e.g. `http://kannon.kannon.svc:50051` |
| `KANNON_DOMAIN` | the Kannon Domain you send from, e.g. `news.example.org` |
| `KANNON_API_KEY` | a Kannon API key for that Domain |
| `KANNON_ADMIN_TOKEN` | optional: Kannon's admin token, so `/admin` shows the delivery counters, an issue's opens and clicks, and the Domain's tracking. Required to send while a Tracking ceiling is raised, for the [Domain check](#sending). Kannon has no read-only credential; Salvo only reads with it |
| `SENDER_EMAIL`, `SENDER_NAME` | who the issues come from, e.g. `hello@example.org`, `Ada Lovelace` |
| `TEST_RECIPIENT` | where test sends go |
| `OWNER_EMAIL` | the one Google account allowed into `/admin` |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | the OAuth client for `/admin` |
| `SESSION_KEY` | signs the admin session cookie, at least 32 characters |

### What readers see

All optional. Left empty, each has a default that reads well for any newsletter.

| Variable | What it is | Default |
|---|---|---|
| `NEWSLETTER_NAME` | the newsletter's name, e.g. `Ada's notes`: the home page's title, and the subject below | `SENDER_NAME` |
| `NEWSLETTER_ABOUT` | a sentence or two on what it is, in the confirmation email | none |
| `NEWSLETTER_SIGNOFF` | the line that closes the confirmation email, e.g. `Ada` | none |
| `CONFIRM_SUBJECT` | the confirmation email's subject | `Confirm your subscription to NEWSLETTER_NAME` |
| `SUBSCRIBE_URL` | where your subscribe form is, for "Subscribe again" | `SITE_URL` |
| `CONFIRMED_URL` | where a reader lands after confirming | `PUBLIC_URL/confirmed` |
| `PRIVACY_URL` | your privacy notice, linked from the confirmation email | no link |

The confirmation email always opens with `SENDER_NAME`. With tracking `off` it says the
newsletter carries no tracking pixel and no tracked links.

### Tracking

Off unless you raise a Tracking ceiling. Read [Tracking and Tracking consent](#tracking-and-tracking-consent) before you do.

| Variable | What it is | Default |
|---|---|---|
| `TRACKING_OPENS` | the Tracking ceiling for opens: `off` or `pseudonymous` | `off` |
| `TRACKING_LINKS` | the Tracking ceiling for links: `off` or `pseudonymous` | `off` |
| `NOTICE_VERSION` | the version of your privacy notice, recorded with every Tracking choice; required while a ceiling is raised | none |

Any other mode refuses to start. While a ceiling is raised, `KANNON_ADMIN_TOKEN` is needed too,
and Kannon's `stats.retention` should be 90 days.

## Commands

```sh
salvo                          # serve (the default)
salvo migrate                  # apply the database migrations, then exit
salvo erase EMAIL              # delete one address and everything stored about it
salvo import-kit < export.csv  # add Kit's active subscribers as confirmed, once, at cutover
```

## Status

Salvo runs one real newsletter, davideimola.dev's, and everything that newsletter says
about itself is configuration now. Next, in order:

1. A hosted subscribe form, for when you have no website backend of your own, with the
   tracking box when a ceiling is raised.
2. Dashboards: list growth, and deliveries and bounces per issue.

[Tracking and Tracking consent](#tracking-and-tracking-consent) is built: off unless you turn it on, and then only for
readers who said yes.

## Development

```sh
docker compose up -d   # the Postgres the tests use, on 127.0.0.1:55432
go test ./...
```

Every push to `main` publishes `ghcr.io/<owner>/<repository>:main-<sha>-<timestamp>`, the tag
shape Flux's image automation sorts by. The image's entrypoint is `/salvo`.

## License

[Apache License 2.0](LICENSE), like Kannon. The mark and the wordmark in `assets/` are covered
by it too.
