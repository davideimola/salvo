# Salvo stores no tracking events; Kannon's copy is the only one

Opens and clicks are read from Kannon's event API, filtered by the issue's batch, whenever a
page needs them; Salvo stores only the Tracked recipients of each issue, the denominator Kannon
cannot know. A second copy in Salvo would be one more thing to retain, erase and justify. The
cost: how long events live is Kannon's `stats.retention` (the README suggests 90 days, the life
of a tracked link), and `salvo erase` cannot reach Kannon's copy until Kannon can erase one
recipient.
