# Salvo holds Tracking consent, and only a confirmation click gives it

Kannon executes a per-recipient Tracking Policy and stores no consent (Kannon ADR 0002), so the
Tracking consent, its proof and every later choice live in Salvo, which states the policy for
every recipient on every send, `off` included. A Tracking consent counts only once the
Subscriber clicks a link mailed to them: at subscription it rides on the confirmation click,
and a Subscriber already on the list who opts in from the preferences page gets a confirmation
email of its own. A button alone would let anyone holding a forwarded issue consent on the
recipient's behalf, which Salvo could not then prove. A Tracking consent belongs to the
Subscription consent it came with and is not revived by a later subscription.
