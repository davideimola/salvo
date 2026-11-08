# Salvo

A self-hosted newsletter: it keeps the list, runs the double opt-in, and sends each issue
through Kannon. Kannon only delivers; everything a reader agreed to is Salvo's to hold.

## The list

**Subscriber**:
One address that asked to receive the newsletter, in whatever state it is now.
_Avoid_: reader (fine in copy addressed to people, not as a domain term), contact, member

**Subscription consent**:
The Subscriber's agreement to receive the newsletter, given by clicking the confirmation link.
Its proof is the time of that click.
_Avoid_: consent (on its own), opt-in

**Unsubscribe**:
Ending the Subscription consent. An unsubscribed Subscriber is never mailed again unless they
subscribe and confirm again.
_Avoid_: opt-out, withdraw

## Tracking

**Tracking consent**:
The Subscriber's optional, separate agreement to have their opens and clicks measured, up to
the Tracking ceiling in force when it was given. It belongs to one Subscription consent: it is
never inherited by a later subscription.
_Avoid_: consent (on its own), tracking opt-in

**Tracking choice**:
One recorded decision about tracking: given, refused, withdrawn, or asked for and awaiting its
confirmation click. Choices are only ever added; the latest one is the Subscriber's state.
_Avoid_: consent record, consent status

**Preferences page**:
The page linked from every issue where a Subscriber sees whether they are tracked, Withdraws,
or asks to give a Tracking consent. Unsubscribing is a separate step from it.
_Avoid_: settings page, profile

**Withdraw**:
Ending the Tracking consent while staying subscribed.
_Avoid_: unsubscribe, opt-out

**Tracking ceiling**:
The most tracking the owner allows for opens and, separately, for links. No Subscriber is
tracked beyond it, and a Subscriber without Tracking consent is not tracked at all.
_Avoid_: tracking level, tracking setting

**Tracked recipients**:
The Subscribers with Tracking consent an issue was sent to: the base of every open and click
share for that issue, never the whole list.
_Avoid_: audience, list size

**Notice version**:
The version of the privacy notice in force when a Tracking consent was given.
_Avoid_: policy version
