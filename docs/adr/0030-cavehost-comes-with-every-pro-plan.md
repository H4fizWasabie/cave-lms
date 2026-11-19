---
status: accepted, amends ADR 0025 and the CaveHost spec (#536)
---

# CaveHost comes with every Pro Plan

CaveHost was for a few Learners the Admin selected, assigned by hand and never sold. The client now sells it: every Pro Plan gives a CaveHost Plan period as long as its Lab Plan period, whether paid at the Pro Plan's price, with any Promo (the RM99 one included) or given by the Admin, and the Admin can still assign CaveHost on its own. CaveHost is never sold alone, and CaveText Pro and the Lab Plan bought apart give none, so the Pro Plan is no longer just the two together: CaveHost is the reason to buy it. `/pricing` and the COMBO Pro Plan card list CaveHost, and a Learner without it who opens CaveHost is pointed to the Pro Plan. Learners holding a Pro Plan at launch get CaveHost once, until their current Lab Plan period ends.

Sold to anyone, CaveHost may have hundreds of Learners, not the few ADR 0025 counted on. Its data stays on the VPS disk for now, but each Learner may have at most 3 CaveHost Deployments, replacing ADR 0025's "no cap on Spreadsheets", so the disk and the one-at-a-time hourly Syncs have a known ceiling (3 × 50 MB per Learner). Data moves to R2 when the disk or the hourly round stops keeping up, as ADR 0025 already says.

Learners' HTML is still deployed as it is, with no cleaning. Cave never runs it: it is stored and served to visitors' browsers under the sandboxed page CSP on `pages.cavelms.com` (ADR 0017, ADR 0024), which is what keeps a page from reaching anything. Cleaning would strip the scripts every chatbot dashboard needs. The server-side risks are the parts Cave itself runs, the Spreadsheet fetch and the xlsx read, and those keep their limits (Google-only addresses, 60 s, 50 MB, the unzip cap). Takedown stays the answer to a page that phishes.

Rejected: (1) CaveHost only with Promos that carry a tick, as with Ebooks, which would leave full-price Pro Plan buyers with less; (2) CaveHost as its own product with a price, which the client did not want yet; (3) sanitising uploaded HTML, for the reason above.
