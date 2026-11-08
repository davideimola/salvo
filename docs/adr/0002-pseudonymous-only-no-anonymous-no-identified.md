# Tracking modes: pseudonymous only, not anonymous and not identified

Salvo's Tracking ceiling takes `off` or `pseudonymous`. Kannon's `anonymous` mode looks like the
most private choice, and the Garante's guidelines (Provvedimento 284, §5) even exempt anonymous
open counting from consent, but Kannon keeps it only as Domain-wide hourly counters with no
batch, so it cannot answer the one question tracking is for (how did this issue do?) and would
add a second, consent-free legal path to explain. `identified` would add only a per-reader
history, at the cost of the address sitting readable in every tracking URL, and so in access
logs and `Referer`: the Garante asks for an opaque identifier instead, and Kannon has decided
not to provide one (kannon-email/kannon#411, its ADR 0006, "Identified means identified"),
pointing to `pseudonymous` for exactly this need. `full` adds IP and user agent on top. If a
per-reader history is ever wanted, it takes a new ADR, and per-recipient erasure in Kannon
(kannon-email/kannon#506) first.
