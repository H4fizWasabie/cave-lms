---
name: CAVELMS
description: The Cave Consult's accredited AI-training site, set like a printed academic journal.
colors:
  charcoal: "#1f2124"
  logo-gold: "#d2a649"
  deep-gold: "#86610f"
  ink-2: "#393c41"
  muted: "#5d6066"
  paper: "#f5f5f2"
  surface: "#fdfdfb"
  line: "#e3e3dd"
  line-strong: "#c9c9c1"
  ok: "#1d6a43"
  bad: "#a3361f"
typography:
  display:
    fontFamily: "Source Serif 4, Georgia, serif"
    fontSize: "clamp(2.5rem, 1.4rem + 4.4vw, 5rem)"
    fontWeight: 400
    lineHeight: 1.04
    letterSpacing: "-0.03em"
  page:
    fontFamily: "Source Serif 4, Georgia, serif"
    fontSize: "clamp(2rem, 1.55rem + 1.6vw, 2.75rem)"
    fontWeight: 700
    lineHeight: 1.15
    letterSpacing: "-0.01em"
  heading:
    fontFamily: "Source Serif 4, Georgia, serif"
    fontSize: "1.75rem"
    fontWeight: 700
    lineHeight: 1.15
  title:
    fontFamily: "Source Serif 4, Georgia, serif"
    fontSize: "1.375rem"
    fontWeight: 700
    lineHeight: 1.15
  lede:
    fontFamily: "Public Sans, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 400
    lineHeight: 1.55
  body:
    fontFamily: "Public Sans, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.55
  ui:
    fontFamily: "Public Sans, system-ui, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 600
    lineHeight: 1.2
  meta:
    fontFamily: "Public Sans, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 600
rounded:
  radius: "4px"
  radius-lg: "8px"
  radius-xl: "12px"
  radius-pill: "999px"
spacing:
  space-1: "0.25rem"
  space-2: "0.5rem"
  space-3: "0.75rem"
  space-4: "1rem"
  space-5: "1.5rem"
  space-6: "2rem"
  space-7: "3rem"
  space-8: "clamp(3rem, 2rem + 3vw, 4.5rem)"
components:
  button-primary:
    backgroundColor: "{colors.charcoal}"
    textColor: "{colors.surface}"
    rounded: "{rounded.radius}"
    padding: "0.5rem 1rem"
    height: "2.5rem"
  button-primary-hover:
    backgroundColor: "{colors.ink-2}"
  button-secondary:
    textColor: "{colors.charcoal}"
    rounded: "{rounded.radius}"
    padding: "0.5rem 1rem"
    height: "2.5rem"
  button-danger:
    backgroundColor: "{colors.bad}"
    textColor: "{colors.surface}"
    rounded: "{rounded.radius}"
  button-gold:
    backgroundColor: "{colors.logo-gold}"
    textColor: "{colors.charcoal}"
    rounded: "{rounded.radius}"
  card:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.radius-lg}"
    padding: "1.5rem"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.charcoal}"
    rounded: "{rounded.radius}"
    padding: "0.5rem 0.75rem"
---

# Design System: CAVELMS

Read this before any UI edit. The stylesheet is one file, `internal/app/static/cavelms.css`; its `:root` blocks hold every token named here. `internal/app/stylesheet_test.go` fails the build when a rule hard-codes a colour, font size, spacing value or radius, names a font family, removes a focus outline, or adds a breakpoint, so the system cannot drift again.

## Overview

**Creative North Star: "The Accredited Journal"**

CAVELMS sells accredited training to civil servants and HRD Corp-claimable companies. The direction chosen from the prototype (`prototype/premium-refresh`) is **variant B's structure set in variant A's typography and spacing**. From B, the site is credentials first: dark charcoal bands, the Certificate shown beside the headline, a trust strip, the four-step PCC pathway, Course cards with pictures, and a two-column Course page with a sticky card. From A, it is calm and editorial: large, light Source Serif display headlines, generous white space, ruled lists instead of boxes where a list reads better, and a printed-journal rhythm.

The identity is fixed: the CAVELMS logo, charcoal with logo gold, Source Serif 4 for display and Public Sans for everything else, all self-hosted. There is no dark mode. Gold is an accent, never a fill for whole sections; charcoal carries the weight.

The Lab (Lab pages, the Dashboard builder, Lab Plans) is outside the refresh. It shares the tokens and shared components but keeps its own section of rules; don't restyle it from a page ticket.

**Key Characteristics:**
- Paper background, near-white surfaces, hairline rules; depth from tone and rules, shadows only on hover and popovers.
- Serif for headings and the voice; sans for interface and reading.
- One primary action per view, in charcoal; gold marks, it doesn't shout.
- Every page works at 375px without sideways scrolling, in Malay and English.

## Colors

A warm-neutral paper palette with one metal: logo gold, used sparingly against charcoal.

### Primary
- **Charcoal** (`--brand-ink`, alias `--text`): body text, primary buttons, dark bands (footer, hero bands, admin sidebar), the focus outline.
- **Logo Gold** (`--brand-gold`): the gold rule under hovered links and open menus, accents on charcoal, `.btn-gold`, the selected-tab bar, progress fills. Never as text on a light background (it fails contrast).
- **Deep Gold** (`--brand-gold-deep`): gold that is readable as text on light (5.5:1 on surface): short labels, numbers, icons.
- **Gold Tint** (`--brand-gold-soft`, mixed from Logo Gold): selection, callouts, selected options, the "joined" badge.

### Neutral
- **Ink 2** (`--ink-2`): secondary copy, descriptions, hover on charcoal.
- **Muted** (`--muted`): meta, hints, captions, crumbs (5.7:1 on paper).
- **Paper** (`--bg`): the page background.
- **Surface** (`--surface`): cards, panels, fields, the header, menus.
- **Line** (`--line`): hairlines, dividers, card borders.
- **Line Strong** (`--line-strong`): field borders, ruled-list rules; body text on charcoal.

### Status
- **OK** (`--ok`, tint `--ok-soft`): passed, open, valid, saved.
- **Bad** (`--bad`, tint `--bad-soft`): errors, revoked, destructive buttons.

### On charcoal
- `--on-dark` (text and links on charcoal), `--on-dark-muted` (meta on charcoal, 5.6:1), `--on-dark-line` (hairlines on charcoal).

### Named Rules
**The Eleven Rule.** The stylesheet has eleven colour values, all in `:root`. A new shade is mixed from them in the rule (`color-mix(in srgb, var(--brand-ink) 6%, transparent)` for a hover tint), never typed as hex, rgb or hsl. The stylesheet test enforces it.

**The Gold Is Not Text Rule.** Logo Gold never sets text on paper or surface. Use Deep Gold for gold text.

## Typography

**Display Font:** Source Serif 4 (400, 600, 700; fallback Georgia) via `--font-display`
**Body Font:** Public Sans (400, 600, 700; fallback system-ui) via `--font-body`
**Mono:** the platform monospace via `--font-mono`, only for code, Certificate numbers and other data.

**Character:** A light, large serif speaks for the brand like a journal masthead; a plain, sturdy sans does the work.

### Hierarchy (eight roles, `--fs-*`)
- **Display** (`--fs-display`, 40-80px fluid; with `.display`: weight 400, line-height 1.04, tracking -0.03em): hero headlines and the Exam score. One per page at most.
- **Page** (`--fs-page`, 32-44px fluid): the page `h1` (`.page-head h1`, `.adm-head h1`, the Course title), weight 700 today. The public-page tickets may set their page titles light (400) in the editorial voice; admin and the Lab keep 700.
- **Heading** (`--fs-heading`, 28px): section `h2`.
- **Title** (`--fs-title`, 22px): card and block titles (`h3`), big numbers in panels.
- **Lede** (`--fs-lede`, 18px): the paragraph under a page title, Lesson prose, small card titles, the confirmation question.
- **Body** (`--fs-body`, 16px): running text and every text field (16px stops iOS zooming on focus).
- **UI** (`--fs-ui`, 15px): buttons, nav, labels, table cells, secondary copy.
- **Meta** (`--fs-meta`, 13px): badges, table heads, captions, crumbs, footnotes, `.btn-sm`.

Headings balance (`text-wrap: balance`), paragraphs avoid orphans (`text-wrap: pretty`). Body measure stays at 65-75ch (`max-width: 42-44rem` on prose and ledes). Numbers that change or line up get `.num` (tabular, lining figures).

### Named Rules
**The Eight Sizes Rule.** `font-size` is always one of the eight `--fs-*` tokens (or `inherit`). A new size means the role is wrong, not the scale.

**The Variable Family Rule.** Rules name fonts only through `--font-display`, `--font-body` and `--font-mono`.

**The No Kicker Rule.** No eyebrow or kicker line above a heading. Let the heading speak.

## Layout

- **Container:** `.wrap` (max 82rem plus gutters, centred; side padding `--gutter`, 24-64px fluid). `.wrap-narrow` for reading pages (43rem).
- **Spacing scale** (`--space-1` to `--space-8`): 4, 8, 12, 16, 24, 32, 48px, then `--space-8` (48-72px fluid) between page sections. Every `padding`, `margin` and `gap` is a step, `0`, `auto`, a 1-2px hairline, `--gutter`, or a `calc()` of those (`calc(-1 * var(--space-3))` for a negative margin). Tight inside a group, a step or two up between groups, more space above a heading than below it.
- **Inner page frame:** `.page-pad` (top `--space-7`, bottom `--space-8`) then `.page-head` (`h1` + muted `p`) or `.page-head-row` (title left, actions right, wraps).
- **Sections:** `.section` (`--space-8` top and bottom) and `.section-alt` (surface band between hairlines); `.section-head` / `.section-head-row` above the content.
- **Breakpoints (three, no more):** `max-width: 1279px` (header folds into the Menu), `max-width: 999px` (two columns become one, admin sidebar becomes a scroller), `max-width: 720px` (phone). Write desktop-first overrides inside these; for a min-width rule use `not all and (max-width: 720px)`.
- **375px:** no page except the Lab may scroll sideways. Long words wrap (`overflow-wrap: anywhere` on user text), tables scroll inside `.table-wrap`, wide media scrolls inside its own box, flex rows wrap.

## Elevation & Depth

Flat by default. Depth comes from paper versus surface, hairline borders and charcoal bands.

### Shadow Vocabulary
- **Lift** (`--shadow-lift`): hover on cards and tiles (`.course-card`, `.cert-chip`, Gallery tiles).
- **Pop** (`--shadow-pop`): header menus, Chat popovers; anything floating over the page.
- **Focus halo** (`--focus`): the gold halo a focused text field adds beside its outline.

### Named Rules
**The Flat-At-Rest Rule.** Surfaces don't cast shadows at rest; a shadow answers hover or floating.

## Shapes

Gently squared: `--radius` (4px) on controls, badges and small cards; `--radius-lg` (8px) on cards, panels and menus; `--radius-xl` (12px) on Chat shells and bubbles; `--radius-pill` for counters, chips and avatars; `50%` for circles. `--radius-phone` (26px) is the Lab's phone-preview bezel only. Nested corners are concentric: an inner corner is the outer radius minus the padding, never the same radius.

**The No Accent Stripe Rule.** No coloured `border-left`/`border-top` thicker than 1px on rounded cards, list items or callouts. (The Lab's `.lab-hero`, `.lab-steps`, `.lab-plan` and `.tp-card.is-picked` still carry them; they are Lab legacy, not a pattern to copy.)

## Components

All live in `cavelms.css`; find a component by its class. Class names below are the API.

### Buttons
Squared, confident, one weight (600, `--fs-ui`). `.btn` plus one variant:
- **Primary** `.btn-solid`: charcoal fill. One per view: the next step.
- **Secondary** `.btn-ghost`: charcoal outline. Alternatives beside a primary.
- **Ghost** `.btn-quiet`: text only, tinted on hover. Cancel, Back, low-stakes actions.
- **Danger** `.btn-danger-solid`: red fill, only as the confirm button of a confirmation page. `.btn-danger` (red text) for a quiet destructive link that leads to that page.
- **Accent** `.btn-gold`: gold fill, on charcoal bands only.
- **Sizes:** default (min-height 40px), `.btn-sm` (32px, `--fs-meta`), `.btn-lg` (48px, `--fs-body`); `.btn-block` fills its row. An icon goes first: `<svg class="i">` from `/static/icons.svg`.
- **States:** hover darkens or tints (150ms colour fade), press scales to .96 (only without reduced motion), `:disabled` / `aria-disabled="true"` at 50% opacity, focus is the shared 2px charcoal outline at 2px offset.

### Badges
`.badge` (13px, 600, 4px radius) plus `.badge-open` (green), `.badge-closed` (grey), `.badge-joined` (gold tint), `.badge-revoked` (red), `.badge-difficulty` (outlined). A badge states a status; it is never a button.

### Cards and panels
- `.card`: surface, 1px `--line`, `--radius-lg`, padding `--space-5`. `.card-head` (title + actions, wraps), `.card-head h2` at Title, `.card-sub` muted; `.card-flush` drops the padding for a table (`.card-head-pad` puts it back on the head).
- `.panel`: the Course page's side card (`.panel-k` label, `.panel-big` big value, `.panel-msg` note).
- Never nest a card in a card. Prefer a ruled list (rows split by `--line`, like `.lesson-list`) where items are read in order.

### Form fields
- `.form` (grid, `--space-4` gaps) of `.field` (label, control, `small` hint, `.field-err` / `.field-ok`). `.opt` marks "(optional)" in a label. `.field-grid` / `.field-grid-2` put fields side by side (one column on phones); `.field-half` limits a short field; `.suffix` adds a unit after an input; `.check` and `.check-card` for checkboxes; `.search` for a search box with its icon; `.inline-form` for a one-line filter.
- Text fields: surface, 1px `--line-strong`, `--radius`, `--fs-body`; hover darkens the border; focus sets a charcoal border, the gold `--focus` halo and a transparent 2px outline, which Windows High Contrast draws as a real ring. Never set `outline: none` on focus (the stylesheet test fails).
- Errors: `aria-invalid="true"` turns the border red; `.field-err` with `role="alert"` says what is wrong and how to fix it.
- `.form-foot`: the form's last row, a hairline above, secondary action left, primary right.

### Page head
`.crumbs` (meta, muted links) then `.page-head` (`h1` at Page, `p` muted at most 44rem) or `.page-head-row` (with actions). Admin pages use `.adm-head` / `.adm-head-actions` inside `.adm-main`.

### Empty state
`.empty-state`: centred, dashed `--line-strong` border, `--radius-lg`; an icon (`.i`, deep gold), an `h3` at Title, a muted `p`, and one button for the next step. In a list, `.empty` is the one-line version.

### Callouts and results
`.callout` (gold tint, icon + text) for a note the reader must not miss; `.result` with `.result-valid` / `.result-revoked` for a verdict; `.up-err` for an upload error.

### Tables
`.table-wrap` (scrolls sideways inside itself) around `.table`: meta-size heads on paper, UI-size cells, row hover tint, `.actions` right-aligned. `td small` for a second line.

### Tabs
`.lab-tabs` / `.lab-tab` with `aria-current="page"`: a gold bar under the current tab. Shared by the Lab and the Course tabs (`course-tabs` template).

### Header
The eight menus stay as the client specified (`.scratch/top-nav-spec.md`): Home, Courses, Lab (a link), Classroom, Pricing, Program, Certification, Community. Each is a `<details class="hdr-drop" name="hdr">` with a `summary.hdr-link`, so opening one closes the others; `header.js` closes the open one on Escape and returns focus to its button.
- **Wide (1280px and up):** surface bar, logo 44px tall, menus in `--fs-ui` ink-2. A gold 2px rule grows under a link on hover and stays under an open menu's button. The menu (`.hdr-panel`) is a surface card with `--shadow-pop`; its links are full-width rows tinted on hover; a Courses menu's last link (All courses) sits under a rule.
- **Phone and tablet (below 1280px):** BM | EN and a Menu button; the Menu is a sheet across the screen under the header, each site menu a full-width row with a caret (`.hdr-caret`) that turns when its group opens, then the account links.
- The account (`.hdr-account`) is Log in + Register (`.btn-solid.btn-sm`) for a Visitor, or the Learner's name opening their menu.

### Footer
A charcoal band: brand column, then Learn / Account / Contact columns with gold uppercase meta headings, on-dark links, muted address, a hairline above the copyright.

### Confirmation page ("Are you sure?")
Destructive actions (delete, leave, remove, revoke) go to a page of their own; no JavaScript, no modal. The pattern is Delete course:
- **Template** (`delete_course.html`): the page head names the action; then `<div class="card confirm">` holding an error line if any, the question as a `p` naming the thing in quotes and what is lost ("Delete "Go Basics"? This cannot be undone."), and a `.form-foot` with Cancel (`a.btn.btn-quiet` back to where the Admin came from) on the left and the action (`form method="post"` with `button.btn.btn-danger-solid` named for the action) on the right. On a Learner page wrap it in `.wrap.page-pad` instead of `.adm-main`.
- **Handler shape** (`courses.go`): `GET /…/{id}/delete` renders the page (`confirmDeleteCourse`); `POST /…/{id}/delete` does it and redirects with `303 See Other` (`deleteCourse`). When the action is refused, re-render the same page with `Error` and the right status (409 for "in use"), the confirm button hidden. The row action that leads there is a link (`a.btn.btn-danger.btn-sm`), never a form that acts on one click.
- **Test shape** (`TestAdminDeletesACourseAfterConfirming` in `courses_test.go`): GET the confirm page and expect 200 with the question; POST and expect 303; then check the thing is gone. Add a case that the GET alone changes nothing, and one that the wrong role is refused.
- **Confirmed posts** (Remove lesson, Revoke certificate): the confirm form also sends `confirm=yes` (a hidden input) and the handler checks `confirmed(r)`; a POST without it does nothing and redirects (303) to the confirmation page. Tests post `confirming`. Use this for any new confirmation.
- Copy goes in the text catalog in Malay and English.

### Busy indicator
`cave-loader` template (`.cave-loader`): the logo's gold peak drawing itself while a slow form is sent; a still pulse under reduced motion.

## Motion

Tokens: `--duration-quick` (150ms, hover and press), `--duration-fast` (250ms, page fade-in, page-head rise, tabs), `--duration-slow` (400ms, arrivals), `--duration-very-slow` (500ms, one emphasis per page); `--ease-smooth-out`; distances `--distance-small/base/medium`. Colour fades run for everyone; movement only inside `@media (prefers-reduced-motion: no-preference)`. Page changes use cross-document View Transitions. Transition named properties, never `all`.

## Page section convention

Parallel tickets share this one stylesheet. Page-specific rules go in their own section at the **end** of the file, headed:

```css
/* === Page: <name> (#<ticket>) === */
```

for example `/* === Page: Course (#310) === */`. Inside it, use only the tokens and shared components above. Don't edit another page's section or a shared component from a page ticket; if a shared component needs a change, say so in the ticket.

## Do's and Don'ts

### Do:
- **Do** build pages from the shared components and the `--fs-*`, `--space-*`, `--radius*` and colour tokens; run `./test.sh ./internal/app/ -run TestTheStylesheet` after CSS edits.
- **Do** give every page one primary (`.btn-solid`) next step.
- **Do** check every touched page at 1280 and 375px, in both languages, with `document.documentElement.scrollWidth > innerWidth` false.
- **Do** keep a visible focus ring on every control (the global `:focus-visible` outline, or a transparent outline plus the halo on fields).
- **Do** mark placeholders for proof the client hasn't supplied (numbers, testimonials) as placeholders.

### Don't:
- **Don't** type a hex, `rgb()` or `hsl()` colour, a raw font size, a raw spacing value or radius in a rule; mix or pick a token.
- **Don't** add a breakpoint, a font, a dependency or an external host (the CSP is `default-src 'self'`).
- **Don't** put an eyebrow or kicker above a heading, gradient text, glass, or a thick coloured side stripe on a card.
- **Don't** use a modal for a confirmation; use the confirmation page.
- **Don't** restyle the Lab from a page ticket.
