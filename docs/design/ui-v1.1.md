# Interface v1.1: one identity, three surfaces

The Mini App, the bot's messages and the admin share one visual language.
This note records the rules, so a new screen or message can be written
without re-deriving them.

## Identity

Deep bottle green is the colour of the institutions Marum argues with.
Brass marks only money that works for the borrower: the paid-off share, the
safe extra, freed money, and the one commit button (Approve plan). Semantic
colours (ok, warn, danger) are their own hues and never stand in for the
accent. No emoji anywhere in the product; a title carries the meaning.

Light: green is a surface, the hero and the buttons. Dark: the ground is a
neutral near-black, green becomes the accent, and the hero is a dark card
with its figure in green. One green per theme.

## Tokens

`internal/design/tokens.css` is the only place a colour is named.
Both surfaces prepend it to their own stylesheet at serve time: the Mini App
when it serves `styles.css`, the admin when it serves `/style.css`. Radii are
9, 14 and 16 px; type weights are 400 and 600 only; cards are separated by
borders, never shadows.

## Mini App

Every screen is an app bar, one hero, then cards. The hero holds the only
large figure on the screen. Amounts sit in key/value rows, right-aligned,
tabular numerals.

| Screen | Answers | Sub-screen |
| --- | --- | --- |
| Loans | how much, when next, how far along | Loan: the facts, Update balance, Edit terms, Remove |
| Add | the terms, the dates, a live estimate | — |
| Budget | the monthly amount and whether it covers what is required | Edit budget |
| Plan | the debt-free date, the monthly amount, the saving; strategy as three rows | — |

A sub-screen names a parent instead of a tab: it gets a back button
(Telegram's own inside the client) and the dock hides. The app bar owns the
title and one right-hand action per screen.

States: skeletons take the shape of the hero and cards; the offline banner
keeps the cached figures on screen and offers a retry; validation is
field-level; an empty screen names the next step. The eye on the loans hero
masks every amount on the device.

## Bot

Every message opens with a bold title. Every figure block goes through
`figures` in `internal/app/advice.go`, which renders label/value rows as a
padded monospace block, so amounts line up in both alphabets. Prose is for
what a number cannot say; the tip is one italic line, last.

The persistent keyboard has four buttons: open the app, what to do, my
loans, budget. Language and help live in the "/" command menu. Inline rows
put the primary action alone on top, inspection second, changing the
question last. Retired labels stay in `legacyButtons` so a keyboard drawn
before a deploy still works.

## Admin

The admin maps its roles onto the shared tokens and adds only the sidebar,
the zebra stripe and a spacing scale. The loan page opens with "What the
borrower sees": the Mini App's hero and rows, from the same read model,
before the record behind it.
