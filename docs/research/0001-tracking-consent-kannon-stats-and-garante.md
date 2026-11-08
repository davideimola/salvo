# Research 0001: tracking consent, Kannon's Stats API, and the Garante's guidelines

Research for [issue #7](https://github.com/davideimola/salvo/issues/7) ("Opt-in tracking, per reader, with consent Salvo can prove"). Researched on 2026-11-08.

Sources are primary only: Kannon's source code and `.proto` files, Salvo's source code, and the Garante's own pages on garanteprivacy.it. One web search was used only to find the docweb number of the Garante's guidelines (see [Sources used only to locate](#sources-used-only-to-locate)). Each legal claim is marked **Verified** (quoted from the primary text) or **Unverified**.

Pinned references:

- Kannon `main` at [`b944a01`](https://github.com/kannon-email/kannon/tree/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960) (2026-11-01). Links below of the form `K:path#Lx` point into this commit, at `https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/<path>#Lx`.
- Kannon latest release [`v0.5.1`](https://github.com/kannon-email/kannon/tree/v0.5.1) (2026-05-22, commit `72588a4`).
- Salvo `main` at [`17004d2`](https://github.com/davideimola/salvo/tree/17004d290124086137b2278fcb0835e2514bae59). Links of the form `S:path#Lx` point into this commit.

---

## Short answers

### Question 1: Kannon's Stats API

- **Which Kannon version Salvo targets.** Salvo pins no Kannon version. `go.mod` does not import Kannon. Salvo talks to Kannon as Connect JSON over HTTP ([S:internal/kannon/kannon.go](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/internal/kannon/kannon.go#L1-L6)), and `compose.yaml` runs only Postgres. In practice Salvo needs **unreleased Kannon `main`**. The latest release, v0.5.1, has no `tracking.proto`, no Tracking Policy, no `oneClickUnsubscribe` and no ADR 0002. Salvo already sends `tracking` and `oneClickUnsubscribe` on every call. Everything below is answered against `main@b944a01`.
- **The `GetStats` shape.** It takes `{domain, from_date, to_date, skip, take}` and returns `{total, stats[]}`. Each `Stats` row is `{message_id, domain, email, timestamp, type, data, tracking_mode}`.
- **Does it return events per recipient with their Mode?** It returns them per recipient, but **not with the Mode.** The field exists in the proto, but the v1 handler never fills it, so it is always `TRACKING_MODE_UNSPECIFIED` on the wire ([K:pkg/statsapi/statsv1/impl.go#L82-L91](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/statsapi/statsv1/impl.go#L82-L91)). The issue's assumption fails for v1.
- **How ANONYMOUS and PSEUDONYMOUS events look.** **ANONYMOUS** events are never stored per recipient. They exist only as Domain/hour/type counters, so `GetStats` never returns them. **PSEUDONYMOUS** events are returned with `email = <128-bit random hex>@track.<domain>`. That pseudonym is drawn fresh for each Delivery and stored nowhere, so nobody can map it back to the reader.
- **Filters and pagination.** v1 `GetStats` can filter by **Domain and time range only**: no batch/message id and no type. Pagination is `skip`/`take` (offset). Kannon's newer **REST `GET /v1beta1/domains/{domain}/stats/events`** does have a `batch_id` filter, a `type` filter and cursor pagination. It returns `tracking_mode` and, under pseudonymous, `tracking_identity`. Kannon calls REST its primary API and deprecates the Connect APIs (ADR 0014).
- **Retention inside Kannon.** Per-recipient rows are kept for `stats.retention`, default 8760h (one year). Aggregate counters are kept for `stats.aggregated_retention`, default one year. Tracking tokens expire after 3 months, so opens later than that are not recorded. Kannon has **no per-recipient erasure**: rows expire only by retention.
- **Is it enough to keep events out of Salvo?**
  - **IDENTIFIED: yes, with REST v1beta1.** Filter by `batch_id`, count distinct emails, and divide by the consenting readers Salvo sent to. The per-reader history works only within Kannon's retention. `salvo erase` cannot delete Kannon's rows.
  - **PSEUDONYMOUS: per-issue counts only, and no per-reader history** by design.
  - **ANONYMOUS: no per-issue figure.** The only source is Domain-wide hourly event counts, which have no batch dimension and do not deduplicate repeat opens. "A share of consenting readers" cannot be computed.
  - **With v1 `GetStats` alone:** Salvo has to page every event of the Domain over a time window, filter by `message_id` on the client, and cannot read the Mode.
- **The per-recipient policy on send.** Use `Recipient.tracking` (`TrackingPolicy{opens, links}`, optional) in both `SendHTML` and `SendTemplate`, plus the batch-level `tracking` field.
  - A recipient above the Domain ceiling is rejected on its own, with `reason = tracking_above_ceiling`.
  - A batch above the ceiling fails the whole call.
- **How Salvo reads the Domain ceiling.** It calls `pkg.kannon.admin.apiv1.Api/GetDomain` and reads `domain.tracking` ([S:internal/kannon/admin.go#L67-L77](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/internal/kannon/admin.go#L69-L77)). The admin panel calls it in [S:internal/server/delivery.go#L50](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/internal/server/delivery.go#L50). Salvo's counters come from `kannon.stats.apiv2.StatsApiV2/GetAggregatedStats`.
- **What ADR 0002 means for Salvo.** Kannon is a processor that executes the per-recipient instruction and stores no consent. Salvo, acting for the controller, must hold the consent, its provenance and the withdrawal mechanism. It must send the right per-recipient Policy every time, because Kannon cannot detect a wrong one. The Domain ceiling is the only backstop.

### Question 2: the Garante's guidelines (Provvedimento n. 284, 17 April 2026)

The document was found and read on garanteprivacy.it: [doc. web n. 10241943](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10241943), published in the Gazzetta Ufficiale Serie Generale n. 98 of 29 April 2026, with a 6-month compliance window.

- **Aggregate/anonymous opens: probably no consent needed (Verified, with conditions).** §5 names "a statistical count measuring the overall open rate" as a case where the controller may rely on an art. 122 exemption instead of consent. The conditions are anonymisation that rules out "personalised measurements", the **same pixel for all recipients of a campaign**, and anonymised IP and client. The rationale given is deliverability and anti-spam ("tra l'altro"). §6 requires consent whenever *individual* measurement is used to tune campaigns or to profile. The burden of qualifying sits with the controller.
- **Click tracking: not addressed (Verified absence).** The guidelines speak only of pixels. Neither the provvedimento nor the Garante's vademecum mentions tracked links or redirects. Whether the aggregate exemption covers clicks is **open**.
- **Does a change of the privacy notice require new consent?** **Not addressed** in n. 284 (Verified absence). For cookies, the 2021 guidelines allow asking again when "one or more of the conditions of the processing change significantly" (§6.2). Applying that to email pixels is an inference, not the Garante's text.
- **Retention of per-recipient tracking events:** **not addressed** (Verified absence). Ordinary GDPR storage limitation applies; not researched further here.
- **How consent must be collected** (Verified):
  - prior to use, preferably when the address is collected, after the reader is informed;
  - "informato, libero, specifico ed inequivocabile";
  - for promotional mail, it **may be bundled** into the single consent to receive the communications, if asked neutrally;
  - withdrawable easily **and granularly**: stop the mail, or stop only the tracking and keep receiving the mail. A footer link or icon leading to a choices area is the suggested mechanism;
  - refusing tracking must not limit the service.

  The guidelines do **not** require a *separate* checkbox; Salvo's separate, unticked box is stricter than required. The word "unticked" does not appear in n. 284 (Unverified for email; the 2021 cookie guidelines require "an active and conscious action").
- **Proof of consent** (Verified): "tutte le scelte dell'interessato siano debitamente registrate dal titolare, anche ai fini della dimostrazione" (art. 7(1) GDPR). That covers every choice, refusals and withdrawals included, not only the consents.
- **Privacy by design** (Verified): the Garante suggests an unintelligible, non-sequential per-recipient id, mapped to the address in a separate internal layer, so the address is not in the pixel request. Kannon's IDENTIFIED/FULL put the address in the token, inside the URL (ADR 0006). PSEUDONYMOUS keeps it out but cannot be mapped back by anyone. Neither matches the suggestion exactly.

---

## Question 1: detailed findings

### 1.1 Which Kannon version

- Salvo's [`go.mod`](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/go.mod) requires only `go-oidc`, `pgx` and `oauth2`. There is no Kannon module and no image tag in `compose.yaml` or `Dockerfile`.
- Salvo already depends on post-v0.5.1 wire fields. `SendHTML` is sent with `tracking: {opens: TRACKING_MODE_OFF, links: TRACKING_MODE_OFF}` and `oneClickUnsubscribe.urlTemplate` ([S:internal/kannon/kannon.go#L64-L95](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/internal/kannon/kannon.go#L66-L91)).
- `git grep -i "tracking|unsubscribe" v0.5.1 -- .proto` returns nothing. The tracking feature landed after v0.5.1:
  - `1de5695` (2026-07-26): "feat!: tracking Policy: per-Domain/Batch/Recipient control"
  - `9d8b9f4` (2026-08-02): pseudonymous Mode
  - `97c95e1`: one-click unsubscribe
  - `3dbd54f` (2026-10-31): REST v1beta1
  - `d98d7d9`: aggregated retention
- **Consequence:** Salvo requires an unreleased Kannon. Pinning a commit or image digest, or waiting for a release, is an open item.

### 1.2 Stats API v1: `GetStats`, quoted

[K:.proto/kannon/stats/apiv1/statsapiv1.proto#L9-L26](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/.proto/kannon/stats/apiv1/statsapiv1.proto#L9-L26). The Connect path is `/kannon.StatsApiV1/GetStats`, because the package is `kannon`.

```proto
service StatsApiV1 {
  rpc GetStats(GetStatsReq) returns (GetStatsRes) {}
  rpc GetStatsAggregated(GetStatsAggregatedReq)
      returns (GetStatsAggregatedRes) {}
}

message GetStatsReq {
  string domain = 1;
  google.protobuf.Timestamp from_date = 2;
  google.protobuf.Timestamp to_date = 3;
  uint32 skip = 4;
  uint32 take = 5;
}

message GetStatsRes {
  int64 total = 1;
  repeated pkg.kannon.stats.types.Stats stats = 2;
}
```

[K:.proto/kannon/stats/types/stats.proto#L15-L28](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/.proto/kannon/stats/types/stats.proto#L15-L28) and `#L69-L78`:

```proto
message Stats {
  string message_id = 1;
  string domain = 2;
  string email = 3;
  google.protobuf.Timestamp timestamp = 4;
  string type = 5;
  StatsData data = 6;
  // The Tracking Mode that governed the engagement channel this event came
  // from, ... Only engagement events (opened, clicked) state a Mode; every
  // other outcome leaves it unspecified.
  pkg.kannon.tracking.types.TrackingMode tracking_mode = 7;
}
message StatsDataOpened  { string user_agent = 1; string ip = 2; }
message StatsDataClicked { string user_agent = 1; string ip = 2; string url = 3; }
```

`StatsData` is a `oneof` of `accepted`, `delivered`, `failed`, `bounced`, `opened`, `clicked`, `rejected` and `error`.

**`tracking_mode` is not populated by `GetStats`.** The v1 handler builds the response like this ([K:pkg/statsapi/statsv1/impl.go#L82-L91](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/statsapi/statsv1/impl.go#L82-L91)):

```go
func statToPb(s *stats.Stat) *types.Stats {
    return &types.Stats{
        MessageId: s.MessageID,
        Domain:    s.Domain.String(),
        Email:     s.Email,
        Timestamp: timestamppb.New(s.Timestamp),
        Type:      string(s.Type),
        Data:      statspb.FromOutcome(s.Outcome),
    }
}
```

The Mode is persisted in `stats.data->>'trackingMode'`, and it is filled on the NATS bus message by `statspb.FromEvent`. Only the v1 read path drops it. This looks like a Kannon gap worth an issue or PR upstream.

**Query semantics** ([K:internal/db/statistics.sql#L11-L21](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/internal/db/statistics.sql#L11-L21)):

```sql
SELECT * FROM stats WHERE domain = $1
AND timestamp >= @start AND timestamp < @stop
ORDER BY timestamp DESC LIMIT @take OFFSET @skip;
```

The query has no `message_id` filter and no type filter. Paging is by offset, and `total` comes from a separate `COUNT(*)`.

**`GetStatsAggregated` (v1)** groups the per-recipient `stats` rows by hour and type (`QueryStatsTimeline`, same file `#L23-L32`). It therefore **excludes ANONYMOUS** engagement, which has no rows.

**Authentication:** both Stats APIs use the Admin Token ([K:pkg/api/api.go#L273-L274](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/api/api.go#L273-L274); `CONTEXT.md` line 112). Salvo already holds that token as `KANNON_ADMIN_TOKEN`.

### 1.3 Stats API v2, which Salvo uses today

[K:.proto/kannon/stats/apiv2/statsapiv2.proto](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/.proto/kannon/stats/apiv2/statsapiv2.proto): `GetAggregatedStats({domain, from_date, to_date})` returns `stats[] {type, timestamp, count}`.

It reads `aggregated_stats` (`PRIMARY KEY (domain, timestamp, type)`), which is incremented for **every** event in every Mode, ANONYMOUS included ([K:pkg/stats/stats.go#L173-L176](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/stats/stats.go#L173-L176)). Kannon's own design notes say:

> "Persisted aggregates are Domain/hour/type counters, not per-Batch counters (`internal/db/aggregated_stats.sql`). ... no per-Batch aggregate capability has been promised yet." ([K:docs/rest-api-design.md#L82](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/docs/rest-api-design.md#L82))

> "The `aggregated_stats` table ... has no Batch dimension; retained per-Delivery events cannot reconstruct complete per-Batch aggregates." ([K:CONTEXT.md#L225](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/CONTEXT.md#L225))

Salvo already works around this by reading the hours after an issue's send ([S:internal/server/admin.go#L138-L141](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/internal/server/admin.go#L138-L141)).

### 1.4 REST v1beta1 events, the better fit

[K:pkg/api/rest/events.go#L13-L87](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/api/rest/events.go#L13-L87) and the OpenAPI at [K:api/openapi/v1beta1.yaml#L731-L770](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/api/openapi/v1beta1.yaml#L731-L770):

- The endpoint is `GET /v1beta1/domains/{domain}/stats/events?from&to[&type][&batch_id][&limit≤1000][&cursor]`. It requires role `admin` or `analyst`, with the Admin Token or a Domain API key as Bearer.
- Results are ordered by timestamp descending, then id descending, with an opaque cursor.
- `batch_id` is the same `message_id` the Mailer API returns. The handler maps `batchIDParameter: s.MessageID`. Salvo already stores it per issue as `issues.kannon_message_id` ([S:internal/store/migrations/0001_init.sql#L40](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/internal/store/migrations/0001_init.sql#L40)).
- Each opened or clicked item carries `tracking_mode` (`pseudonymous` | `identified` | `full`). Under `pseudonymous` the `email` field is removed and `tracking_identity` is set. `ip` and `user_agent` appear only under `full`.
- The OpenAPI description reads: "Anonymous events exist only as counters. Pseudonymous events must not expose email or original delivery_id." (`#L2000`).
- The SQL also excludes anonymous and sentinel rows explicitly ([K:internal/db/inline.sql#L72](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/internal/db/inline.sql#L72)).
- `GET /v1beta1/domains/{domain}/stats/aggregated?from&to&group_by` returns Domain counters. It too has no batch dimension.

ADR 0014 records that REST is now primary and that the "Existing gRPC APIs are deprecated but remain available" ([K:docs/adr/0014-rest-is-the-primary-api-with-explicit-domain-resources.md](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/docs/adr/0014-rest-is-the-primary-api-with-explicit-domain-resources.md)).

### 1.5 How each Mode is represented

| Mode | Token identity ([ADR 0006](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/docs/adr/0006-tracking-identity-in-the-token-sentinel-addresses.md)) | Row in `stats`? | Seen by v1 `GetStats` | Seen by REST events | Counted in `aggregated_stats` |
| --- | --- | --- | --- | --- | --- |
| OFF | no pixel, no rewritten links | no | - | - | no |
| ANONYMOUS | `anonymous@track.<domain>`, one token shared per Batch (per `(Batch, url)` for links) | **no** | no | no | yes |
| PSEUDONYMOUS | `<32 hex>@track.<domain>`, fresh per Delivery, stored nowhere | yes, `email` = pseudonym | yes, as `email`; Mode not shown | yes, `tracking_identity` | yes |
| IDENTIFIED | the recipient's address | yes | yes; Mode not shown | yes, `email` | yes |
| FULL | the address, plus IP and UA retained | yes | yes; Mode not shown, IP/UA in `data` | yes, with `ip` and `user_agent` | yes |

Evidence:

- The stats worker skips the row under Anonymous ([K:pkg/stats/stats.go#L224-L246](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/stats/stats.go#L224-L246)): `if mode == tracking.ModeAnonymous { ... "anonymous event: counted in aggregate, no per-recipient row" ... return msg.Ack() }`.
- The tracker keeps no identity below Pseudonymous, and keeps IP and UA only under Full ([K:pkg/tracker/srv.go#L95-L103](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/tracker/srv.go#L95-L103)).
- The pseudonym is random, generated per Delivery, and unlinkable ([K:internal/tracking/identity.go#L38-L47](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/internal/tracking/identity.go#L38-L47); ADR 0006 line 75: "generated once per Delivery ... the pixel token and every link token of the Delivery carry the same value ... regenerated for every Batch, stored nowhere, and derived from nothing").
- The comment in `tracking.proto` ("a pseudonym drawn afresh for every batch") is looser than the code, which draws one per Delivery. Within a Batch, one pseudonym equals one recipient.

### 1.6 Retention inside Kannon

- Per-recipient rows: `stats.retention`, default `8760h` ([K:pkg/stats/stats.go#L25-L39](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/stats/stats.go#L25-L39); `README.md` line 231). The example docker-compose config uses `720h`.
- Counters: `stats.aggregated_retention`, default `8760h`. These "are the only record of anonymous engagement" ([K:docs/UPGRADING.md#L85-L90](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/docs/UPGRADING.md#L85-L90)).
- Pruning runs every 10 minutes (`cleanupCycle`, `stats.go#L109`).
- Tracking tokens expire after 3 months: `tokenExpirePeriod = time.Hour * 24 * 30 * 3` ([K:internal/statssec/keys.go#L20](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/internal/statssec/keys.go#L20)).
- There is no per-recipient erasure: "Kannon has no per-recipient erasure anywhere today (stat rows expire by retention only)" (ADR 0006, line 120).

### 1.7 Can Salvo compute the issue's figures without storing events?

The denominator Salvo needs is "readers who consented, in this issue's batch". Salvo knows it at send time and should snapshot the count on the issue, because consent changes afterwards. Kannon cannot supply it.

| Need | IDENTIFIED | PSEUDONYMOUS | ANONYMOUS |
| --- | --- | --- | --- |
| Unique openers or clickers per issue | REST events with `batch_id` and `type=opened` / `type=clicked`, distinct `email` | REST events, distinct `tracking_identity` | **Not possible.** Only hourly Domain event counts, with no batch dimension and no dedup |
| Share of consenting readers | Yes | Yes | **No.** Only a ratio of raw events to consenting readers, mixed with anything else sent in those hours |
| Per-reader history | Yes, REST events (no email filter, so per batch or by window, then filter client-side), within Kannon retention | No, by design | No |
| Warn when Kannon counts an event for a reader without consent | Yes: an event whose `email` is not in Salvo's consented set for that issue | Only by checking the Mode: any pseudonymous event when the reader's Mode should be off | Only Domain-level: any `opened`/`clicked` counter while `TRACKING=off` (today's check) |

**What is missing:**

1. v1 `GetStats` drops `tracking_mode`, has no `message_id` or type filter, and uses offset paging. Use REST v1beta1 instead, or fix v1 upstream.
2. There are no per-Batch aggregates, so ANONYMOUS cannot give per-issue figures. Kannon's own docs say this is not promised.
3. Neither API filters by recipient email, so a reader's history means scanning by batch or by time window.
4. The history lasts only as long as Kannon's retention, and `salvo erase` cannot reach Kannon's rows. The README would need to state that Kannon keeps them until `stats.retention`, or Kannon needs an erasure call. ADR 0006 sketches a denylist; it is not built.
5. Under ANONYMOUS, counts are unauthenticated: replaying a URL inflates them (ADR 0003 consequences). Apple MPP inflates opens in every Mode.

### 1.8 Setting the per-recipient policy on send

[K:.proto/kannon/mailer/types/send.proto#L14-L28](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/.proto/kannon/mailer/types/send.proto#L14-L28):

```proto
message Recipient {
  string email = 1;
  map<string, string> fields = 2;
  // The Recipient-level Tracking Policy — the most specific level of the
  // cascade, where a caller states the consent it holds in its own CRM
  // (ADR 0002). States nothing when omitted ...
  // Consent may only narrow ... A Recipient stating a Mode above its
  // Domain's ceiling is Rejected on its own, with a reason in
  // SendRes.rejected_recipients, while the rest of the Batch proceeds
  optional pkg.kannon.tracking.types.TrackingPolicy tracking = 3;
}
```

`SendHTMLReq` and `SendTemplateReq` both carry `optional TrackingPolicy tracking = 10` at batch level: "A Mode above that ceiling fails the call" ([K:.proto/kannon/mailer/apiv1/mailerapiv1.proto#L20-L58](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/.proto/kannon/mailer/apiv1/mailerapiv1.proto#L20-L58)). The rejection reasons include `tracking_above_ceiling` and `unsupported_tracking_mode` (`#L76-L101`). Each recipient is capped at 2000 per call.

The effective Policy is the most restrictive of Domain, Batch and Recipient. It is resolved at intake and frozen on the Delivery, and a Domain change does not affect Deliveries already queued (ADR 0003).

In JSON (protojson), Salvo's `Recipient` struct needs `"tracking": {"opens": "TRACKING_MODE_…", "links": "TRACKING_MODE_…"}`. Today it has only `email` and `fields` ([S:internal/kannon/kannon.go#L34-L37](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/internal/kannon/kannon.go#L35-L38)). The issue's plan works: the batch carries `TRACKING`, a consenting reader carries `TRACKING`, and every other reader carries `OFF`.

**Not verified here:** when a recipient's links are `OFF` while the batch allows more, is that recipient's HTML left free of rewritten links and of the pixel? This follows from "resolved and frozen per Delivery" plus `retained()`, but the envelope builder was not read line by line. Cover it in the "mixed batch" test the issue already lists.

### 1.9 Reading the Domain ceiling

[K:.proto/kannon/admin/apiv1/adminapiv1.proto#L46-L51](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/.proto/kannon/admin/apiv1/adminapiv1.proto#L46-L51):

```proto
message Domain {
  string domain = 1;
  string dkim_pub_key = 3;
  // The ceiling every batch and recipient of this domain is resolved against.
  pkg.kannon.tracking.types.TrackingPolicy tracking = 4;
}
```

Salvo calls `POST /pkg.kannon.admin.apiv1.Api/GetDomain {"domain": …}` with `X-Kannon-Admin-Token` and reads `domain.tracking.{opens,links}`. `Tracking.Off()` requires both to be `TRACKING_MODE_OFF` ([S:internal/kannon/admin.go#L58-L77](https://github.com/davideimola/salvo/blob/17004d290124086137b2278fcb0835e2514bae59/internal/kannon/admin.go#L58-L77)). For issue #7 the check becomes "Domain rank ≥ `TRACKING` on both axes". The REST equivalent is `GET /v1beta1/domains/{domain}`, whose response has `tracking`.

Note that ADR 0003 defaults **new Kannon Domains to `identified`**, so the "Domain must be off" check is doing real work.

### 1.10 ADR 0002, and what it means for Salvo

[K:docs/adr/0002-kannon-executes-tracking-instructions-does-not-store-consent.md](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/docs/adr/0002-kannon-executes-tracking-instructions-does-not-store-consent.md) (Accepted 2026-07-26).

Its decision: "Kannon is an **executor**. The Tracking Policy arrives with each send request and Kannon applies it. Kannon holds no record of consent, and no code path consults one." It frames the Domain as the controller and Kannon as the Art. 28 processor. Consent "belongs where it is collected ... when it was given, through which form, against which privacy notice." It rejects a provenance-less `consents` table as "false assurance", and rejects deriving consent from unsubscribe state.

What that means for Salvo:

1. **Salvo is the consent register.** It must store provenance: when, the source (form or API), the notice version, and the confirmation click. It must also store withdrawals and refusals, which the Garante's text requires as well (see 2.5).
2. **Every send must state the per-recipient Policy.** An omitted field defers to Batch and Domain, so a bug that drops the field widens tracking up to the ceiling. Always send an explicit `OFF` for non-consenting readers.
3. **Kannon cannot catch a wrong instruction.** Salvo's tests and the ceiling are the only guards. Keep `TRACKING` and the Domain ceiling equal.
4. **Withdrawal is Salvo's job.** The preferences page lives in Salvo. Kannon offers only the one-click unsubscribe header, which is a separate decision.
5. **Withdrawal does not stop mail already queued**, because the Policy is frozen per Delivery (ADR 0003, "No kill switch"). It does stop what that reader is sent next.

ADR 0002's own summary of the Garante (Art. 122, in force 29 April 2026, six months) is a secondary source for Question 2. It was checked against the primary text below and matches it.

---

## Question 2: detailed findings

**Primary document:** Garante per la protezione dei dati personali, *Provvedimento del 17 aprile 2026, "Linee Guida in materia di utilizzo di tracking pixel nelle comunicazioni di posta elettronica"*, Registro dei provvedimenti **n. 284 del 17 aprile 2026**, [doc. web n. 10241943](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10241943). It was published in G.U. Serie Generale n. 98 of 29 April 2026. The compliance term is "6 mesi dal momento della loro pubblicazione in Gazzetta Ufficiale".

**Companion Garante pages** (primary, the Garante's own summaries):

- Press release of 21 April 2026, [doc. web n. 10241977](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10241977).
- News of 5 October 2026, [doc. web n. 10303585](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10303585): "Entro il 29 ottobre 2026 ... dovranno adeguare informative, consensi e sistemi di revoca". Enforcement is part of the H2 2026 inspection plan.
- Vademecum "Tracking pixel nelle e-mail. Le indicazioni del Garante", [doc. web n. 10303688](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10303688) ([PDF](https://www.garanteprivacy.it/documents/10160/0/VADEMECUM+Tracking+pixel+nelle+e-mail.+Le+indicazioni+del+Garante.pdf/4ee553a6-6f96-b766-d71a-d6e1c617d024?version=2.0)).

The provvedimento has no numbered paragraphs. Citations below use its section headings: §1 to §6 of the "PREMESSO".

### 2.1 Legal frame (Verified)

§1(a) says inserting the pixel and reading the open "sono operazioni conformi alla fattispecie di accesso al terminale disciplinata dall'art. 122 del Codice". Following EDPB Guidelines 2/2023, "il menzionato art. 122 del Codice ... si [applica] all'uso dei pixel di tracciamento nelle e-mail".

§5: art. 122(2-bis) imposes "un divieto generalizzato di trattamento", unless:

- (a) prior consent is given;
- (b) the processing is necessary for transmitting the communication; or
- (c) it is necessary for a service the user requested.

### 2.2 Aggregate or anonymous counting (Verified, with conditions)

§5, first example of where controllers "possano beneficiare della deroga rispetto all'acquisizione di uno specifico consenso":

> "ogniqualvolta l'impiego di tracking pixel sia funzionale all'effettuazione di un conteggio di tipo statistico che misuri la percentuale globale di apertura dei messaggi. I risultati di tali misurazioni sono infatti necessari, tra l'altro, al fine di migliorare la deliverability delle e-mail ... come pure per contrastare fenomeni di spam. Resta inteso che simili indagini di tipo statistico impongono l'adozione di tecniche di anonimizzazione delle informazioni, in modo da non consentire misurazioni personalizzate. A tal fine, l'Autorità suggerisce l'impego di pixel univoci, cioè non differenziati per ciascun utente, ma uguali per tutti i destinatari di una stessa campagna, come pure l'anonimizzazione degli altri dati tecnici correlati (indirizzo IP, client etc.)"

The vademecum, p. 7, gives the same point: "il consenso può non essere necessario quando il pixel è utilizzato esclusivamente per ottenere statistiche complessive sull'apertura dei messaggi ... purché siano adottate tecniche che impediscano di misurare il comportamento del singolo destinatario. A questo scopo possono essere utilizzati pixel uguali per tutti i destinatari della stessa campagna". It adds: "Spetta comunque al titolare ... verificare se nel caso concreto ricorrono i presupposti".

§6 sets the boundary: consent is required "ogniqualvolta la misurazione individuale e l'analisi del tasso di apertura delle e-mail siano utilizzate per valutare e migliorare la performance delle campagne promozionali", or to adapt frequency, or to profile.

**Fit with Kannon ANONYMOUS** (an inference, not the Garante's words):

- It matches the technical conditions. There is one shared token per Batch: "one signature per Batch for the pixel and one per `(Batch, url)` for links" (ADR 0003). Nothing isolates a recipient, and IP and UA are not retained.
- The exemption's stated purpose is an overall open rate, including for deliverability and anti-spam. Showing per-issue open percentages to the newsletter owner sits close to "percentuale globale di apertura". Whether that counts as "statistical", or as campaign-performance measurement, which needs consent only when *individual*, is the controller's call under accountability.
- The 2021 cookie guidelines treat analytics the same way: equated to technical cookies only if they prevent singling out and serve "unicamente alla produzione di statistiche aggregate" ([doc. web n. 9677876](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/9677876), §7.2). n. 284 explicitly calls itself "analogamente a quanto già stabilito per l'impiego di cookie" (§4).
- **Clicks:** n. 284 and the vademecum discuss only pixels. The only "click" in the text is "cliccando sul link 'disiscriviti'". Whether the aggregate exemption covers anonymous click counting is **not stated** (Verified absence). EDPB Guidelines 2/2023, which n. 284 relies on, could not be read in this session (the PDF could not be rendered), so its treatment of tracked links is **Unverified**.

### 2.3 Does a change of the privacy notice require consent again? (Not addressed)

n. 284 does not mention a re-consent obligation on changes to the notice (Verified absence; the full text was searched).

Related text that exists:

- **Transitional regime (§4, §6):** for processing already under way, the notice may be given "con l'inoltro del primo messaggio utile ovvero il primo momento di discontinuità". A granular withdrawal mechanism must then be offered. That regime "destinato ad essere progressivamente dismesso".
- **Service messages (§5):** pixels in messages announcing "modifiche ... alle informative sul trattamento dei dati personali" may be exempt from consent.
- **By analogy only:** the 2021 cookie guidelines, §6.2, say a recorded choice must not be solicited again unless "mutino significativamente una o più condizioni del trattamento", or 6 months have passed since the banner was shown. Applying this to email tracking consent is **an inference**, not in n. 284. Storing the notice version, as the issue plans, keeps the option open.

### 2.4 Retention of per-recipient tracking events (Not addressed)

n. 284 sets no retention period for tracking events (Verified absence). The general GDPR principles (art. 5(1)(e), storage limitation) apply but were not researched here. Kannon's defaults (one year, see 1.6) are a Kannon choice, not a regulatory figure.

### 2.5 How consent is collected and proved (Verified)

- **Timing:** "il consenso dovrà essere raccolto preferibilmente al momento stesso dell'acquisizione dell'indirizzo di posta elettronica ..., dopo che ... l'interessato sia stato debitamente informato" (§6).
- **Quality:** "informato, libero, specifico ed inequivocabile" (§5(a); vademecum p. 7).
- **Bundling is allowed for promotional mail:** "il consenso alla ricezione degli strumenti di tracciamento in questione possa, in linea di principio, essere ricompreso in quello, più generale, alla ricezione delle comunicazioni promozionali ... alla condizione, tuttavia, che la richiesta sia formulata in modo neutro e privo di forzature" (§6). So a **separate** checkbox is *not* required by n. 284. Salvo's separate, unticked, optional checkbox is a stricter choice, and it avoids the question of whether a newsletter is "promozionale".
- **Unticked:** not stated in n. 284 (Unverified for email). The 2021 cookie guidelines §7.1 require "un intervento attivo e consapevole dell'utente, opportunamente riscontrabile e dimostrabile". The GDPR (recital 32, pre-ticked boxes) was not fetched in this session.
- **Granular withdrawal without unsubscribing:** "revocare in modo agevole le scelte pregresse, anche in modo granulare: optando cioè per la revoca del consenso unico prestato, con l'effetto di impedire la futura ricezione di ulteriori messaggi, oppure revocandolo solo parzialmente, con esclusivo riguardo soltanto al tracciamento" (§6).
- **Mechanism:** "inserendo, ad esempio, all'interno di ogni messaggio e-mail inviato una icona standardizzata ovvero un link, posizionato nel footer, che conduca l'utente verso un'area dedicata ... ovvero di cessare soltanto la ricezione dei tracking pixel, continuando cioè a ricevere le comunicazioni di posta elettronica prive di tali marcatori" (§6). This matches the issue's `{{ preferences_url }}` exactly.
- **No penalty for refusing:** "alla persona che intenda rifiutare il tracciamento dovrà essere garantita la piena fruibilità del servizio, che non dovrà comunque subire, per questa sola ragione, alcuna limitazione" (§6).
- **Proof:** "tutte le scelte dell'interessato siano debitamente registrate dal titolare, anche ai fini della dimostrazione cui questi potrebbe essere chiamato, specie in caso di controversie (cfr. art. 7, par. 1 del Regolamento)" (§6). This covers *all* choices, so withdrawals and refusals need a record too, not only consents.
- **Information** (§4): a layered notice is allowed, for example a short notice in the address-collection form with a link to the full one. Using pixels without informing the reader makes them unusable ("pena la loro inutilizzabilità").

### 2.6 Privacy by design (Verified), compared with Kannon's Modes

§6, last paragraph: "il mittente generi un identificativo inintelligibile e non sequenziale e lo associ all'indirizzo di posta elettronica del destinatario, mantenendo tale corrispondenza in un layer interno e separato della piattaforma ... senza che l'indirizzo e-mail sia ricompreso nella richiesta tecnica generata dal caricamento del pixel." Vademecum p. 9 repeats it.

How Kannon's Modes compare:

- **IDENTIFIED/FULL** deliberately put the address in the signed but readable JWT inside the URL. ADR 0006, "Identified means identified": "Under Identified and Full the address stays in the URL, deliberately."
- **PSEUDONYMOUS** keeps the address out of the URL but has no mapping layer at all, so nothing can be attributed to a reader.
- **Neither matches the suggestion.** A per-reader history with the address kept out of the request has no Kannon Mode today. The suggestion is "opportuno", not mandatory.

---

## Open / unverified

1. **Kannon version.** Salvo pins none and needs unreleased `main`. Pin a commit or digest, or wait for a release after v0.5.1.
2. **v1 `GetStats` omits `tracking_mode`** ([impl.go#L82-L91](https://github.com/kannon-email/kannon/blob/b944a01dda8e1fdfb02f7a4d60a13ab00d58e960/pkg/statsapi/statsv1/impl.go#L82-L91)). Either use REST v1beta1 or report it upstream.
3. **Per-batch aggregates do not exist in Kannon**, so ANONYMOUS cannot give per-issue or per-reader-share figures. This needs a decision: drop "share of consenting readers" for anonymous, or ask Kannon for a Batch dimension on counters.
4. **Erasure.** Kannon keeps per-recipient rows until `stats.retention` and has no erasure call. `salvo erase` cannot honour "deleted with the rest" for Kannon's copy.
5. **The mixed-batch rendering** (no pixel and no rewritten links for a recipient at `OFF` inside a batch at `IDENTIFIED`) is inferred from ADR 0003 and the tracker code, not read in the envelope builder. Test it.
6. **Click tracking under the aggregate exemption:** not addressed by n. 284. EDPB Guidelines 2/2023 were not read (**Unverified**).
7. **Re-consent on a notice change:** not in n. 284. The cookie-guidelines analogy (§6.2 of doc. web 9677876) is an inference.
8. **Retention of tracking events:** no figure in n. 284.
9. **"Unticked" checkbox:** not in n. 284. GDPR recital 32 was not fetched (**Unverified**).
10. **Whether a non-commercial newsletter is "comunicazione promozionale"**, which decides whether bundling is even available: not resolved by the text. A separate checkbox sidesteps it.
11. **Whether per-issue open rates shown to the owner fall within "conteggio di tipo statistico"** or need consent even when anonymous: the text leaves it to the controller's accountability. The issue's interim rule ("treat anonymous like the other modes") is the conservative reading.

## Sources used only to locate

- A web search (2026-11-08) returned the garanteprivacy.it docweb ID 10241943 among secondary results (law-firm and news sites). None of those secondary sources is cited for any claim above.
