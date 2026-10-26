---
target: Cave LMS UI/UX (Premium refresh, closing pass, 0a78b4f)
total_score: 26
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:/home/hafiz/Desktop/cave LMS/internal/app/templates"
timestamp: 2026-10-26T00-56-00Z
slug: internal-app-templates
---
build: premium-refresh 0a78b4f, local 127.0.0.1:8136, DB cave_crit_final (copy of the critique data)

# Critique: Cave LMS (internal/app/templates), Premium refresh, closing pass, 2026-10-26
Method: dual-agent (A: design review, ~60 pages at 1280/375, BM+EN, visitor/Learner/Admin, full journey join -> Lessons -> IC -> MCQ Exam -> pass -> Certificate · B: detector + browser overlay on 7 pages + probes on 15)
The Lab is out of scope. It is not scored; its findings are listed separately.

## Heuristics: 26/40 (Acceptable, top of band)
| # | Heuristic | Score | Key issue |
|---|---|---|---|
| 1 | Status | 3 | progress, answered counter, timer, "Awaiting Result" all work. Syllabus Exam row never shows Passed (course.html:86-93). "Ready at 08:50" released ~08:52. No "IC saved" feedback. |
| 2 | Real world | 3 | glossary terms consistent BM/EN. "Congratulations, your answers are in" fires at submit, even before a fail (catalog.go:1608). Admin shows a Written-exam tab on MCQ-only Courses (admin_course.html:14). |
| 3 | Control | 3 | confirmations, IC Cancel, Mark incomplete, Submit anyway. Chat message delete and leave Community still act on one click (chat.html:92, :12). |
| 4 | Consistency | 2 | destructive actions now confirm consistently, but: IC is double entry with a callout on the Exam step and single entry with a hint on My profile (exam_ic.html:11-21 vs my_profile.html:14-22). "All Courses" (catalog.go:1484). Admin "Baharu"/"baharu" casing and the "Pentadbir:" prefix on some titles only. Failed uses `badge-revoked` (my_exams.html:16). 404 and 500 pages differ. |
| 5 | Error prevention | 3 | six new confirm pages, draw-count warning (exam.html:84-86), IC double entry, blank-answer check. Still: profile IC has no confirm field; "Publish drafts" relies on JS `data-confirm` (exam.html:20). |
| 6 | Recognition | 3 | next-step buttons name their target; one pager "Next Lesson" on a done Lesson. Program, Community and visitor Certification are one-link dropdowns (layout.html:199-213); Admin Course tabs have no current tab (admin_course.html:9-15). |
| 7 | Flexibility | 2 | skip link now present. No Course search or filter, no sortable Admin tables, no copy on prompt `pre` blocks. |
| 8 | Aesthetic | 2 | core Learner pages are calm with one primary each. The periphery is noisy: home is 7427 px desktop and 13138 px phone; an 8-vendor hosting table on Pricing and home (pricing.html:15, home.html:182-187); Admin has the public header plus an 18-item sidebar; the drawn cover is identical on every card. |
| 9 | Error recovery | 3 | field errors name the fix (IC mismatch, blank answers keep the work); the 500 now offers Try again and Home. 404 and 403 pages have no links in main (message.html:14 sets `OfferHome` only on 500); "Attempt not found" renders an empty box; register repeats its hint (register.html:23-24). |
| 10 | Help | 2 | new: "Starting uses 1 of your 2 attempts", IC notice, set-once callout. Still unexplained before submit: the ~10-min result delay, the retake rule after a pass, Answer-sheet timing, what the Certificate needs. No FAQ or help page. |

Scoring note: Assessment A scored 25 (9 = 2). The parent held 9 at 3. The common error paths (forms, IC, Exam, 500) now name the fix and keep the work. The 404/403 dead ends were already there at e0b0c5d and were missed then; they are not a regression. Drop 9 to 2 next run if they remain. Aesthetic falls 3 -> 2 because this pass calls the unchanged periphery (home length, the vendor table, the doubled Admin chrome) what it is. The 26/40 run was generous here; nothing regressed.

## Design specificity
The design is authored for this product:
- the live Certificate preview in the Learner's name (hero, Course panel, My certificates)
- the PCC four-step pathway
- the HRD Corp / MOF / JPK strip
- Malay first
- journal serif on charcoal

The generic parts are the lower half of home (feature-explainer sections), Pricing (icon-in-tinted-square cards and the dev-hosting comparison), and the Admin shell. The weakest repeated component is the drawn cover (layout.html:263-266). It is now title-free, but it is the same dark box with a faint gold letter on every card, and its level label repeats the Self-paced chip. At 375 it costs ~170 px above Join.

## Cognitive load
5/8 pass.
- Pass: single focus (Learner pages), grouping, visual hierarchy, one thing at a time (IC step, attempt), working memory (counter, timer, attempt-cost line).
- Fail: chunking (8 header menus, ~13 home sections, 18 sidebar items); minimal choices (8 menus + BM|EN + account; Admin Course page has 5 tab-buttons plus the sidebar); progressive disclosure (the Admin Exam page shows the list, AI generate, bank copy and settings at once, exam.html:16-89, with two primaries at :59 and :87).

## Emotional journey
- Peaks: your own name on the Certificate preview right after joining; the IC step's callout, double entry and fixable mismatch error; the blank-answer catch ("your other answers are still here").
- Valleys:
  - Join sits ~1260 px down on a phone.
  - Submit says "Congratulations" and then the Learner waits ~10 min for a Result they were never told about.
  - A pass before the IC was set (Course 1) shows only "Download your Answer sheet" on the Exam page. The IC cue sits only in the not-passed branch (learner_exam.html:21-30).
- End: the pass Result now has a primary "View your Certificate" (fixed since 26/40). But it opens the /my-certificates list, not the new Certificate, and shows no number and no thumbnail. "100%" is followed by "Score: 100%" (exam_attempt.html:10-11). The peak is better but still plain.

## Strengths
1. **The Certificate is the spine.** Preview in the Learner's name, then the public verify page (Sah, number, date) and the PDF.
2. **The Exam safety net.** One-sentence `.sr` facts with visual values `aria-hidden` (learner_exam.html:11-16), IC double entry with a set-once callout, the blank-answer alert that keeps answers, and timer warnings announced politely.
3. **One destructive-action pattern.** Nine confirmation pages name the object in quotes and say what is lost, with Cancel left and the red action right.

## Priority issues
- **[P1] The pass peak is plain and the Certificate path is indirect.**
  - Move "Congratulations" from submit (catalog.go:1608) to the pass Result.
  - Link the Result to `/certificates/{id}` and show its number and thumbnail.
  - Drop the duplicate "Score".
  - State the result delay on the Exam start page.
  - Add the IC callout and IC-step link to the passed-without-IC Exam page (learner_exam.html:27-31).
  - /impeccable delight, clarify
- **[P1] 404 and 403 are dead ends.** Set `OfferHome`, or a contextual Back to the Course, on every not-found or forbidden render (message.html:14). Give "Attempt not found" its message. /impeccable harden
- **[P2] Navigation overload.**
  - Turn the three one-link dropdowns into plain links (layout.html:199-213).
  - Give Admin its own shell without the public header (layout.html:146-213).
  - Add My exams, My profile and Log out to the Learner footer (layout.html:63-66).
  - /impeccable distill
- **[P2] Phone Course page.** Join sits at 1255-1272 px on /courses/1-3 at 375. Put the CTA before the cover and the Certificate art, or drop the cover on phones. /impeccable adapt
- **[P2] Admin density.**
  - Results dates wrap to 4 lines and the table overflows its card even at 1280 (admin_results.html).
  - "5 daripada 5" wraps to 3 lines.
  - Tables clip at 375 with no scroll cue.
  - The Exam page has two primaries.
  - /impeccable layout

## Persona red flags
- **Jordan (first-timer):**
  - Lands on a 7.4k px home.
  - Pricing never says Courses are free and compares against TextExpander, Vercel and Supabase.
  - "Program" and "Komuniti" each open to one link.
  - "2 Attempts left" comes with no rules.
  - "Congratulations", then an unannounced wait.
  - A pass without a Certificate until a profile callout is noticed.
- **Sam (keyboard, screen reader):**
  - The skip link "Langkau ke kandungan" works, and the gold 2px ring shows on the charcoal hero and the footer (fixed).
  - Malay legends and titles in the EN UI have no `lang` (exam_attempt.html:49, course.html:79).
  - The my_exams empty-state title is a `p` (my_exams.html:28).
  - "Course complete" is a green `p` styled like a button (course.html:41).
  - 404s offer no links.
- **Casey (375):**
  - No page scrolls sideways.
  - Join at ~1260 px.
  - The Lesson 2 `pre` prompt is clipped mid-word with no copy (cavelms.css:1444-1445).
  - Admin tables are clipped.
  - "Mark incomplete" sits ~13 px right of the title edge (course.html:81).
  - The five Exam fact tiles stack 2+2+1.

## Minor observations
- Page h1 weight is 400 on the hero pages and 700 elsewhere, and `.x-score` is 700 (cavelms.css:401).
- At 375 the Result h1 hyphen-breaks "Multiple-/choice".
- "Attempts left: 1" still shows after a pass.
- My profile can't show or change the name printed on the Certificate.
- A register 500 (email off in this environment) drops the typed name and email.
- The outcomes list on /courses/1 mixes EN and BM.
- Home card titles balance-wrap into ~240 of 300 px.
- The h1 `rise` starts at opacity 0 (reduced motion respected).

## Detector
- **CLI templates:** exit 2, 78 findings, all false positives: 77 design-system-color "rgb(0,0,0)" and 1 flat-type-hierarchy (layout.html). The cause is the unresolved `{{static "cavelms.css"}}` href, so no CSS was read. 3 of them are in Lab templates.
- **CLI static:** exit 2, 52 findings, all Lab, 0 outside the Lab.
  - Tokenised Lab section: 4 (border-accent-on-rounded :694 :701, side-tab :704 :732).
  - Lab-as-on-main: 48 (font-size 31, color 6, radius 5, border-accent 3, side-tab 2, font 1).
- **Overlay:** live-server detect.js, bypass CSP, preflight mutation passed. 0 findings on all 7 pages (`/`, `/courses`, `/courses/1`, lesson, `/my-courses`, `/admin`, `/admin/courses/1`). A positive control (an injected side-tab card plus a gradient-text h2) reported 2, so the overlay works. The 9 findings from the 26/40 run (tight-leading cover titles, `.mc-desc` line length) are gone with the cover title.
- **Probes:** 15 pages at 1280 and 375.
  - No sideways scroll, no heading skips, one h1 each.
  - The first Tab reaches `a.skip` -> `main#main`.
  - The hero focus ring is gold 2px on charcoal, 7.14:1 (cavelms.css:109).
  - `::selection` is themed.
  - The forced-colors ring shows on inputs.
  - EN titles are sentence case. BM titles are mixed: "Ruang Pentadbir", "Bank Soalan" vs "Log masuk", "Halaman tidak dijumpai". Home is a bare "CAVELMS".
- **Token sprawl:**

| Measure | Excluding Lab-as-on-main | Including it |
|---|---|---|
| Distinct hex | 11 (all in :root) | 25 |
| Hard-coded colour outside :root | 0 | 26 |
| Font sizes | 8 (all var) | 35 |
| Radii | 13 (all token, 0 or 50%) | 17 |
| Spacing | 6 (hairlines and calc of tokens) | 47 |
| Breakpoints | 3 | 3 |

- **Stylesheet test:** `go test -run TestTheStylesheet` skips without TEST_DATABASE_URL. A Python port of its 6 assertions passed 6/6 on the served CSS and failed 6/6 on a mutated control.

## Lab (not scored)
All 52 static detector findings are in the Lab. Lab pages were not visited. Pricing is a CaveText/Lab sales page and was judged only as public IA.

## Questions to consider
1. If the Certificate is the product, why does the pass moment send the Learner to a list instead of handing them their numbered Certificate?
2. Does a civil servant buying HRD-claimable training need a Vercel vs Supabase table on the home page?
3. Should the Admin live inside the public site at all?

## vs baseline (21/40, 2026-10-25T13-23-55Z) and vs 26/40 (2026-10-25T20-42-06Z, e0b0c5d)
| # | Heuristic | Baseline | 26/40 | Now |
|---|---|---|---|---|
| 1 | Status | 2 | 3 | 3 |
| 2 | Real world | 2 | 3 | 3 |
| 3 | Control | 2 | 3 | 3 |
| 4 | Consistency | 3 | 2 | 2 |
| 5 | Error prevention | 1 | 2 | 3 |
| 6 | Recognition | 2 | 3 | 3 |
| 7 | Flexibility | 2 | 2 | 2 |
| 8 | Aesthetic | 3 | 3 | 2 |
| 9 | Error recovery | 2 | 3 | 3 |
| 10 | Help | 2 | 2 | 2 |
| | **Total** | **21** | **26** | **26** |

The total holds at 26. Error prevention gains a point: the six one-click destructive POSTs now confirm, and the draw-count warning and IC double entry were added. Aesthetic loses a point. That is a stricter reading of the periphery, which did not change (home length, vendor table, Admin chrome); the cover itself improved. The other gains landed inside their bands:
- Status: the pass leads to the Certificate.
- Control: IC Cancel.
- Recognition: one "next".
- Error recovery: 500 links.
- Help: IC and attempt notices.

The 26/40 run's priority issues:
- **P1 one-click destructive POSTs: resolved.** All six now have confirmation pages. Only chat message delete and leave Community remain, both outside that list.
- **P1 passing is a dead end: mostly resolved.** The Result and Exam pages have a primary "View your Certificate". Remaining: it targets the list, the copy is premature, and the passed-without-IC Exam page has no IC cue.
- **P2 IC surprise, no Cancel, hint-only warning: resolved.** IC notice on the Exam start page, Cancel, set-once callout. The profile form still differs.
- **P2 invisible hero focus, no skip link: resolved.**
- **P2 the cover repeats the title: resolved.** The cover is now repetitive in another way.

26/40 minor items:
- Fixed: Exam fact labels; "My …" casing.
- Still open: `pre` copy/wrap; register hint repeat; Admin tabs current state; my_exams `p` title; Learner footer links; Admin date wrapping; Join depth on phone.

Detector trend:
- Overlay findings: 9 -> 0.
- CLI static outside the Lab: 0 -> 0.
- Templates false positives: 77 -> 78, one per template.
- Token counts unchanged except spacing 5 -> 6 (one more `calc` of a token).
