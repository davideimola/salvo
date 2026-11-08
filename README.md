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

It began as the newsletter of [davideimola.dev](https://davideimola.dev), replacing Kit, which
cannot send without a per-reader pixel. The story of that migration is
[davideimola/davideimola.dev#132](https://github.com/davideimola/davideimola.dev/issues/132).

## How it fits together

```
your website ---API_KEY---> salvo ----> Postgres (subscribers, issues)
                              ^  |
reader's mail client          |  v
  /confirm, /unsubscribe -----+  Kannon (tracking off) ----> SMTP
                              |
you, signed in with Google    |
  /admin ---------------------+
```

- **Your website** owns the content: it renders each issue's HTML and posts subscribe requests
  to Salvo's API.
- **Salvo** owns the list: who subscribed, when they confirmed, who left. It decides who gets
  an issue and when.
- **Kannon** only delivers. It keeps no list, and every send from Salvo states tracking `off`,
  on top of the Domain's own setting.

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
  Nothing about the address is left.
- **An issue goes out once.** Your website can save an issue and send it to you as a test, but
  only the admin page sends it to the list. You type the issue's slug to confirm, and only after
  testing its latest version.
- **No double sends.** If Kannon refuses a send, nothing went out and the issue goes back to
  draft. If the connection drops halfway, the issue stays `sending` with the error shown, and
  Salvo does not retry, because retrying could mail everybody twice.
- **An admin page.** Subscribers, issues, preview, test, send, delete, and Kannon's delivery
  counters. If Kannon ever counts an open or a click, the page warns you.

The rules live in `internal/newsletter`, and its tests are the specification.

## Run it

You need:

- **Postgres**. The tests run on 17.
- **Kannon**, with a Domain whose tracking policy is `off` for opens and links, and an API key
  for that Domain.
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
it. Likewise `{{ preferences_url }}` is each reader's Preferences page, where they see whether
they are tracked and can Withdraw while staying subscribed: optional while both Tracking
ceilings are `off`, required as soon as one is raised. When the test looks right, open `/admin`
and send it.

## HTTP

| Route | Who | What |
|---|---|---|
| `POST /api/subscribers` `{"email","tracking"}` | website, `Authorization: Bearer API_KEY` | start the double opt-in, asking for a Tracking consent too when `tracking` is `true`; `202` whatever the address's state, `400` for an invalid address or for `tracking` while every ceiling is `off` |
| `PUT /api/issues/{slug}` `{"subject","html"}` | website | save or replace a draft; the HTML must contain `{{ unsubscribe_url }}`, and `{{ preferences_url }}` while a ceiling is raised |
| `POST /api/issues/{slug}/test` | website | send the draft to `TEST_RECIPIENT` |
| `GET /confirm?token=` | reader | confirm, then redirect to `CONFIRMED_URL` |
| `GET /confirmed` | reader | Salvo's own "you're in" page, where `CONFIRMED_URL` points by default |
| `GET /unsubscribe?token=` | reader | a page with an Unsubscribe button |
| `POST /unsubscribe?token=` | reader, mail client | unsubscribe; `List-Unsubscribe=One-Click` is the RFC 8058 form |
| `GET /preferences?token=` | reader | the Preferences page: whether opens and clicks are measured; changes nothing |
| `POST /preferences` `token`, `action=withdraw` | reader | Withdraw the Tracking consent and stay subscribed |
| `/admin` | owner, signed in with Google | subscribers, issues, preview, test, send, delete, delivery counters |
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
| `KANNON_ADMIN_TOKEN` | optional: Kannon's admin token, so `/admin` shows the delivery counters and checks that the Domain's tracking is off. Kannon has no read-only credential; Salvo only reads with it |
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

Off unless you raise a Tracking ceiling. A Subscriber is tracked only after giving a Tracking
consent, by ticking a separate box when subscribing and then clicking the confirmation link,
and never beyond the ceiling in force when they gave it.

| Variable | What it is | Default |
|---|---|---|
| `TRACKING_OPENS` | the Tracking ceiling for opens: `off` or `pseudonymous` | `off` |
| `TRACKING_LINKS` | the Tracking ceiling for links: `off` or `pseudonymous` | `off` |
| `NOTICE_VERSION` | the version of your privacy notice, recorded with every Tracking choice; required while a ceiling is raised | none |

Any other mode refuses to start.

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

1. A hosted subscribe form, for when you have no website backend of your own.
2. Dashboards: list growth, and deliveries and bounces per issue.
3. Opt-in tracking, per reader. Kannon can set a tracking policy for each recipient, and Salvo
   is where the consent would live, with its proof and a way to withdraw it while still
   receiving the newsletter. Off unless you turn it on, and then only for readers who said yes.

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
