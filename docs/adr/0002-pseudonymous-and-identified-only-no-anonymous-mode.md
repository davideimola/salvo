# Tracking modes: pseudonymous and identified, not anonymous

Salvo's Tracking ceiling takes `off`, `pseudonymous` or `identified`. Kannon's `anonymous` mode
looks like the most private choice, and the Garante's guidelines (Provvedimento 284, §5) even
exempt anonymous open counting from consent, but Kannon keeps it only as Domain-wide hourly
counters with no batch, so it cannot answer the one question tracking is for (how did this
issue do?) and would add a second, consent-free legal path to explain. `full` is excluded for
the IP and user agent it records. `identified` waits on Kannon carrying an opaque identity in
place of the address, as the Garante suggests, and on per-recipient erasure there; until then
Salvo ships `pseudonymous`.
