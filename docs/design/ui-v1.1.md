# Interface: current v2.0.3 conventions

This file retains its original name for existing links. It describes the current
Mini App, bot and admin, replacing the earlier v1.1 brass/green proposal.

## Shared visual language

[Shared tokens](../../internal/design/tokens.css) define neutral surfaces and green
accents. Dark mode uses a near-black background, charcoal cards and muted green
highlights. Light mode uses pale neutral backgrounds and white cards. Semantic
warning/error colors retain their own purpose. Legacy token names such as brass
are compatibility aliases, not a direction to restore the former palette.

System fonts, bordered cards and 9/14/16px radii form the base. Mini App fields use
16px text and bounded widths; large amounts use tabular numerals and must remain
readable without splitting digits across lines. Both rendered applications use
the shared tokens; Cloudflare's versioned asset build also includes them.

## Mini App navigation

| Root | Purpose | Child workflows |
| --- | --- | --- |
| Home | Next payment and shortcuts | Budget, Plan, individual loan |
| Plan | Outcome and selected chart | Payments, milestones, strategies, calculation detail and history |
| Loans | Scannable loan records | Add/edit terms, icon choice, balance/payment/statement workflows |
| Activity | Recorded payments and plan comparison | Payment details and corrections |
| More | Language and account actions | Edit budget, add loan, collapsed reminder settings |

Keep primary information in compact cards. Use child screens and disclosure
sections for detail; the design aims to reduce scrolling, not force every form
onto one physical screen. Loan icons are explicitly chosen, with the bank icon
as default; the app does not guess an icon from a lender's name.

Charts share a selector, visible series legends and a selected-month readout.
Projected interest is a forecast; recorded interest belongs to payment activity
and may be unknown when the bank allocation is absent. Required-only comparison
is labeled as a comparison, not presented as paid history.

The header language action opens More. English/Armenian selection is first;
reminder preferences are collapsed below it. Budget and Money include the
collapsed [budget explanation](../product/budgeting.md). Inputs for month/date/time
must stay within their card on iOS; checkboxes and their labels have independent
sizes. Retry actions must not shrink into broken words.

## Loading and recovery

Preserve readable cached figures with a visible stale state when refresh fails.
Warnings follow resources used by the visible screen; hidden stale snapshots
remain stale until reread. Empty, loading and failed states need distinct copy
and an explicit action where appropriate. Plan strategy comparison loads on
request and clears old figures when reopened.

The API wrapper bounds fetch and response-body waits separately; this is not a
single end-to-end request deadline. Financial writes are not automatically
retried. A user's explicit retry must retain durable command identity.

## Bot

Use short headings and clear action buttons. Native emoji may support recognition;
there is no current blanket emoji ban. Language is available through the bot's
language controls and `/language`. Existing keyboard labels remain compatible
across deployments. Keep financial messages private and avoid presenting
unconfirmed payments or projections as lender-confirmed facts.

See [messaging](../architecture/07-messaging.md) for exact commands, delivery
semantics and reminder behavior.

## Admin

The operator surface shares tokens but has its own navigation and information
density. Role, purpose, TOTP and independent review gates are part of the workflow;
visual polish must not bypass them. See [admin](../architecture/06-admin-ui.md).

## Verification and historical previews

v2.0.3 includes Mini App behavior tests and synthetic 320px/390px English/Armenian
browser checks. Those checks do not certify Telegram's physical iPhone webview.
The HTML mockups and three-loan dataset in this directory are design/reference
assets, not live account state or proof of supported lender behavior. See
[current state](../current-state.md) and [release evidence](v3/release-checklist.md).
