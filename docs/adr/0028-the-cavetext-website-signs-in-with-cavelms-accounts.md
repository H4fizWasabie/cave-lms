---
status: accepted
---

# The CaveText website signs in with CAVELMS accounts

The client wants CaveText to stand on its own: a Learner signs in, registers, resets their password and buys CaveText Pro or the Pro Plan on the CaveText website, without being sent to CAVELMS. The CaveText website's account and pricing pages are served by CAVELMS at the CaveText website's address, in CaveText's design and languages, and every account they make or use is an ordinary CAVELMS Learner. Signing in there keeps the Learner signed in to the CaveText website only, not to CAVELMS; the Lab, even when bought with the Pro Plan there, is still used on CAVELMS. CAVELMS keeps its own CaveText page as a second way to buy.

Rejected: (1) CaveText keeping its own accounts, which makes a CaveText user and a CAVELMS Learner two people, breaks Payments matched by CAVELMS email, the Pro Plan's Lab Plan, Early places and Ebook access, and means moving every existing CaveText user; (2) a password form on the static CaveText website posting to CAVELMS from another site, which needs cross-site requests and a second login to guard; (3) signing in inside the extension's own window, which is no website, and leaves registering and buying on CAVELMS.
