---
target: Cave LMS UI/UX (Premium refresh, final pass)
total_score: 26
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:/home/hafiz/Desktop/cave LMS/internal/app/templates"
timestamp: 2026-10-25T20-42-06Z
slug: internal-app-templates
build: premium-refresh e0b0c5d, local 127.0.0.1:8130, DB cave_final
---
# Critique: Cave LMS (internal/app/templates), Premium refresh final pass, 2026-10-25
Method: dual-agent (A: design review, ~45 pages at 1280/375, BM+EN · B: detector + browser overlay on 7 pages)
Lab out of scope: not scored; Lab findings listed separately.

## Heuristics: 26/40 (Acceptable, top of band)
| # | Heuristic | Score | Key issue |
|---|---|---|---|
| 1 | Status | 3 | progress bars, done ticks, sticky timer, pending badge on My exams; result page stops at the score |
| 2 | Real world | 3 | Malay default, KP wording; price table compares against TextExpander/Vercel/Supabase (client copy) |
| 3 | Control | 3 | Mark incomplete, Leave page with Stay; IC step has no Cancel (exam_ic.html:21) |
| 4 | Consistency | 2 | "My Courses"/"My Certificates" vs "My exams"; some deletes confirm, others act on one click; admin Exam page has two primary buttons |
| 5 | Error prevention | 2 | one-click POSTs: exam.html:35 delete question, admin_course.html:30 remove learner, admin_question_bank.html:30, organisation_form.html:16, programme_form.html:21, assignment.html:19; draw count 30 > 2 questions accepted |
| 6 | Recognition | 3 | Lesson numbers, "Continue to Lesson 3", crumbs; finished Lesson shows two different "next" links |
| 7 | Flexibility | 2 | no skip link (content ~13 Tabs in); no copy on prompt `pre` blocks |
| 8 | Aesthetic | 3 | calm, ruled, credentials first; stand-in cover repeats the Course title on every card and Course page; home 7.4k px desktop / 13k px phone |
| 9 | Error recovery | 3 | specific field errors; 500 page has no link or retry |
| 10 | Help | 2 | nothing explains Exam rules or what follows a pass; set-once IC warning is a small hint |

## Design specificity
Authored for this product: Source Serif display, charcoal bands with gold, the Learner's own name on the sample Certificate beside the headline, PCC strip, HRD Corp / MOF / JPK trust strip, Malay-first copy. Weakest point: the title-only stand-in cover, the most repeated component, reads generic.

## Cognitive load
5/8 pass. Fails: minimal choices (8 header menus + BM|EN + account; admin shows the public header and the 13-item sidebar at once), single primary (admin Exam page), working memory (IC asked in three places, wired differently). Exam start facts repeat each value in their label ("2 / Questions: 2", learner_exam.html:10-14).

## Emotional journey
Good early reassurance ("Join in one click", Certificate preview in the Learner's name). The peak is flat: the result page shows the score and "Passed", then only "Back to the Exam"; the Exam page after a pass offers the Answer sheet as its primary, not the Certificate. The IC step appears without warning when Start is pressed.

## Strengths
1. Confirmation pages (Leave, Delete Course, Remove Lesson, Revoke) follow DESIGN.md exactly.
2. Exam safeguards: blank answers flagged with a jump link, sticky timer bar, announced warnings, "n of N", pending state.
3. Lesson prose: measure, gold-ruled quote, tables scroll in their own box; themed selection, caret, scrollbar, tabular numerals.

## Priority issues
- [P1] One-click destructive POSTs (six forms above). Fix: link to a confirmation page with `confirm=yes` / `confirmed(r)`. /impeccable harden
- [P1] Passing the Exam is a dead end. Fix: primary "Get your Certificate" (or "Set your IC", "Finish Lessons") on the result and Exam pages; Answer sheet secondary. /impeccable clarify, delight
- [P2] IC requirement surprises at Exam start; IC step has no Cancel; set-once warning is a hint, not a callout. /impeccable clarify
- [P2] Invisible focus in the Course hero (`:focus-visible` charcoal outline on charcoal `.cp-hero`; gold overrides only on footer, home hero/close, admin sidebar) and no skip link. /impeccable audit
- [P2] Stand-in cover repeats the title; drop the title from a picture-less cover. /impeccable distill

## Persona red flags
- Jordan (first-timer): two disagreeing "next Lesson" links; IC surprise; "Open" the only label on My exams; hosting-brand price comparison.
- Sam (a11y): no skip link; invisible focus in the Course hero; hero pills look like buttons.
- Casey (mobile): Join ~1230 px down on /courses/2; "Your progress / 2 of 5 Lessons" cramped in the panel; admin tables crushed (titles one word per line, dates over 4 lines).

## Minor observations
Exam fact labels repeat values; admin result dates/numbers need `.num` + nowrap; "Mark incomplete" indent at 375; `pre` prompts cut off (no wrap/copy); register password error repeats its hint; admin course tabs show no current tab on the overview; my_exams empty state uses `p` for its title; Learner footer lacks My exams / My profile / Log out. Motion is sound: rise only without reduced motion, page transitions gated, 150 ms press scale.

## Detector
- CLI templates: exit 2, 77 findings, all false positives: 76 design-system-color "rgb(0,0,0)" + 1 flat-type-hierarchy (layout.html), both from the unresolved `{{static "cavelms.css"}}` href.
- CLI static: 52 findings, all Lab: 4 in the tokenised Lab section (side-tab :694 :722, border-accent-on-rounded :684 :691), 48 in Lab-as-on-main (side-tab 2, border-accent 3, font-size 31, color 6, radius 5, font 1). Non-Lab CSS: 0.
- Overlay (bypass CSP, 7 pages): `/` 3, `/courses` 3, `/courses/1` 1 tight-leading (`.course-cover-title` 1.15; acceptable at 22px), lesson 0, `/my-courses` 2 (tight-leading on 15px cover title: real; line-length `.mc-desc` ~95 chars: real), `/admin` 0, `/admin/courses/1` 0.
- Token sprawl (excl. Lab-as-on-main / incl.): hex 11/25, hard-coded colour outside :root 0/26, font-sizes 8/35, radii 13/17 (only `0` and `50%` literal), spacing 5/46 (hairlines and calc of tokens), breakpoints 3/3.
- A11y: no sideways scroll on any checked page at 1280 or 375; no heading skips; forced-colors ring visible on focused inputs; `::selection` themed. TestTheStylesheet passes.

## Lab (not scored)
All 52 static detector findings; Lab pages not visited.

## Questions to consider
1. If the Certificate is the product, why is the moment you pass the plainest page?
2. Does an Admin need the 8 public menus while editing an Exam?
3. Who is the price comparison against Vercel and Supabase for?

## vs baseline (2026-10-25T13-23-55Z, 21/40)
| # | Heuristic | Old | New |
|---|---|---|---|
| 1 | Status | 2 | 3 |
| 2 | Real world | 2 | 3 |
| 3 | Control | 2 | 3 |
| 4 | Consistency | 3 | 2 |
| 5 | Error prevention | 1 | 2 |
| 6 | Recognition | 2 | 3 |
| 7 | Flexibility | 2 | 2 |
| 8 | Aesthetic | 3 | 3 |
| 9 | Error recovery | 2 | 3 |
| 10 | Help | 2 | 2 |
| | **Total** | **21** | **26** |

Consistency drops because the refresh made Leave/Delete/Revoke confirm, which exposes the forms that still act on one click, and because page titles differ in case.

Baseline priority issues:
- P1 Learning loop broken: **resolved** (Lesson page marks done/incomplete and continues to the next Lesson; Exam gated on Lessons). Residual: two "next" links on a finished Lesson.
- P1 Exam safeguards: **resolved** (blank-answer flag, timer bar with announced warnings, "n of N", attempts left, pending state on My exams).
- P1 IC prerequisite hidden: **partly resolved** (Course panel callout, IC step before Start, My certificates); still not stated on the Exam start page.
- P2 Mobile overflow + prose: **resolved** (no sideways scroll on ~45 pages; `.prose` styles pre/code/table/blockquote).
- P2 Nav sprawl: **partly resolved** (admin sidebar 18 -> 13; the 8 header menus are client-mandated; Admin pages still carry the public header).

Baseline detector: static 5 -> 0 outside the Lab (the 5 were Lab rules); skipped heading fixed; forced-colors focus fixed. Token sprawl: hex 51 -> 11, hard-coded colour 72 -> 0, font-sizes 47 -> 8, radii 17 -> 13 (token-derived), spacing 60 -> 5, breakpoints 6 -> 3 (excl. Lab).
