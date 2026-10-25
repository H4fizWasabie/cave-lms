---
target: Cave LMS UI/UX
total_score: 21
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 3
target_identity: "file:/home/hafiz/Desktop/cave LMS/internal/app/templates"
timestamp: 2026-10-25T13-23-55Z
slug: internal-app-templates
---
# Critique: Cave LMS (internal/app/templates), 2026-10-25
Method: dual-agent (A: design review, B: detector + browser overlay)

## Heuristics: 21/40 (Acceptable)
1 Status 2: my-exams shows "Not taken" for a pending attempt; no lesson progress; no save confirmations
2 Real world 2: BM/EN mix (EN hero CTAs in BM, Malay-only pricing image on EN)
3 Control 2: Leave course / Remove lesson / Revoke cert, one click, no confirm
4 Consistency 3: coherent system; "mark not done" styled as green success chip
5 Error prevention 1: exam submits blanks silently; timer auto-submits silently; draw count 30 > 2 questions no warning
6 Recognition 2: IC requirement revealed only on My Certificates; login lands on marketing /
7 Flexibility 2: no bulk actions/shortcuts; filters need submit
8 Aesthetic 3: calm, restrained; landing text wall; admin dashboard has no data
9 Error recovery 2: field-err role=alert exists, lightly tested
10 Help 2: good inline hints, no help hub

## Priority issues
P1 Learning loop broken: lesson.html:27 back button only; completion lives on course page; Exam is primary CTA at 0/N lessons -> clarify, layout
P1 Exam safeguards: exam-timer.js:9 silent submit; no blank-answer confirm, numbering, aria-live, attempt-cost notice; my_exams.html:15 no pending state -> harden
P1 Certificate prerequisite (IC, set-once) hidden until after passing -> onboard
P2 Mobile overflow + prose: .c-title-row no wrap (css:226), .prose lacks pre/code/table/blockquote (css:348) -> adapt, typeset
P2 Nav sprawl: 8 top-level menus, 18 flat admin items with repeated icons, login -> marketing / -> distill

## Detector
CLI templates: 1 (flat-type-hierarchy, false positive: unresolved {{static}} href)
CLI static: 5 (side-tab x2 css:729,757; border-accent-on-rounded x3 css:719,726,734)
Overlay: /courses + /lab skipped-heading h1->h3; /lab kicker-above-heading; footer low-contrast = false positive (5.25:1)
Token sprawl: 51 hex (72 hard-coded outside :root), 47 font-sizes, 17 radii, 60 spacing values, 6 breakpoints
A11y: text inputs lose focus ring in forced-colors (css:259, :277)
