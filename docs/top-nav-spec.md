## Problem Statement

The client wants the site's header to work like a proper menu that walks a Visitor through who The Cave Consult is, what it teaches, what it sells and what comes next. Today the header is five flat links (Courses, Lab, Gallery, CaveText, Check a certificate). There is nowhere to see Plan prices, a Learner's own Exams or membership, or upcoming Programmes, and nothing introduces the company or Coach Adam from the header.

## Solution

Replace the header links with eight buttons, each opening a menu on click:

1. **Home** (Utama): Welcome, Coach Adam and About CAVELMS (jumps to the matching sections of the home page), and Gallery. A Company background item will be added when the client sends the text.
2. **Courses** (Kursus): the 3 latest open Courses, then All courses.
3. **Lab**: shows "From static dashboard to hosted dashboard" as the menu's subtitle (also the Lab page headline), and links to the Lab.
4. **Classroom** (Kelas): My courses and a new My exams page.
5. **Shop** (Kedai): a new public Plans page, CaveText, and a new My membership page.
6. **Program**: a new public page listing upcoming Programmes.
7. **Certification** (Pensijilan): Check a certificate, and My certificates for a logged-in Learner.
8. **Community** (Komuniti): hidden until the community channel exists.

On phones, the existing Menu button lists the same eight sections as expandable groups. The footer stays as it is.

## User Stories

1. As a Visitor, I want the header to show Home, Courses, Lab, Classroom, Shop, Program and Certification, so that I can see everything CAVELMS offers at a glance.
2. As a Visitor, I want each header button to open its menu when I click it, so that the menus work on touch screens as well as with a mouse.
3. As a Visitor, I want an open menu to close when I open another one, so that only one menu covers the page at a time.
4. As a Visitor on a phone, I want the Menu button to list the eight sections as expandable groups, so that I can reach every link on a small screen.
5. As a Visitor, I want the header labels in Malay when the site is in Malay (Utama, Kursus, Lab, Kelas, Kedai, Program, Pensijilan), so that the header matches the rest of the page.
6. As a Visitor, I want Home > Welcome to take me to the welcome section of the home page, so that I can read the introduction from any page.
7. As a Visitor, I want Home > Coach Adam to take me to Coach Adam's picture and introduction, so that I know who teaches.
8. As a Visitor, I want Home > About CAVELMS to take me to the section explaining CAVELMS, so that I understand what the site is for.
9. As a Visitor, I want Home > Gallery to open the Gallery, so that I can browse videos, ebooks and pictures.
10. As the Admin, I want the Company background item left out until its text exists, so that Visitors never land on an empty section.
11. As a Visitor, I want the Courses menu to preview the 3 latest open Courses by title, so that I can jump straight into one.
12. As a Visitor, I want the Courses menu to end with All courses, so that I can see every Course.
13. As a Visitor, when there are fewer than 3 open Courses, I want the menu to show only those there are, so that it never shows empty rows.
14. As a Visitor, I want the Lab menu to say "From static dashboard to hosted dashboard", so that I understand what the Lab does before opening it.
15. As a Visitor, I want the Lab page headline to say "From static dashboard to hosted dashboard", so that the page matches the menu.
16. As a Visitor, I want to see the Classroom button, so that I know a Learner gets a classroom by registering.
17. As a Visitor, I want Classroom's items to send me to log in and return me to that page afterwards, so that I end up where I meant to go.
18. As a Learner, I want Classroom > My courses to open my enrolled Courses, so that I can carry on learning.
19. As a Learner, I want Classroom > My exams to list the Exam of every Course I am enrolled in, so that I can see what I still need to sit.
20. As a Learner, I want each Exam on My exams to show not taken, passed or failed, so that I know where I stand.
21. As a Learner, I want each Exam I have attempted to show my best score against its pass mark, so that I know how close I am.
22. As a Learner, I want each Exam on My exams to link to that Exam, so that I can sit or retake it.
23. As a Learner enrolled in Courses without an Exam, I want My exams to leave those Courses out, so that the list only shows things I can sit.
24. As a Learner with no Exams, I want My exams to say so and link to the Courses, so that I know what to do next.
25. As a Visitor, I want Shop > Plans to list every Plan with its name, length, Deployment limit and price, so that I can compare them.
26. As a Visitor, I want a Buy button on each Plan that has a payment link, so that I can pay for it.
27. As a Visitor, I want CaveText Pro shown on the Plans page with its price and what it includes, so that I can compare it with the Plans.
28. As the Admin, I want to set a price on each Plan, so that the Plans page can show it.
29. As the Admin, I want a Plan with no price to show without a price rather than "RM 0", so that an unpriced Plan doesn't look free.
30. As a Visitor, I want Shop > CaveText to open the CaveText page, so that I can still find the extension now that it has left the top bar.
31. As a Learner, I want Shop > My membership to show my current Plan and the date its period ends, so that I know how long my Lab access lasts.
32. As a Learner with a Plan period queued after the current one, I want My membership to show the last end date, so that I know when my access really ends.
33. As a Learner without a Plan, I want My membership to say I have none and link to the Plans page, so that I know how to get one.
34. As a Learner, I want My membership to show whether I have CaveText Pro and until when, so that I know when to renew.
35. As a Learner, I want My membership to show my Snippet cap, so that I know how many Snippets I can keep.
36. As a Visitor, I want My membership to send me to log in, so that I see my own membership afterwards.
37. As a Visitor, I want the Program page to list upcoming Programmes with title, start date, blurb, picture and link, so that I can plan to join the next one.
38. As a Visitor, I want Programmes ordered by start date, soonest first, so that the next one is at the top.
39. As a Visitor, I want a Programme to drop off the page once its start date has passed, so that the page only shows what is still coming.
40. As a Visitor, when no Programme is coming up, I want the Program page to say so, so that it doesn't look broken.
41. As the Admin, I want to add, edit and delete Programmes from the Admin area, so that I can keep the Program page current.
42. As the Admin, I want a Programme's picture and link to be optional, so that I can announce something before those exist.
43. As the Admin, I want Programmes kept after their date has passed and listed in the Admin area, so that I can reuse or delete them.
44. As a Visitor, I want Certification > Check a certificate to open the certificate check, so that I can verify someone's Certificate.
45. As a Learner, I want Certification > My certificates to open my Certificates, so that I can download them.
46. As a Visitor, I want My certificates hidden from the Certification menu, so that I only see what works for me.
47. As the Admin, I want the Community button hidden for now, so that nobody finds a section that does nothing.
48. As a keyboard user, I want to open, move through and close each menu with the keyboard, so that the header is usable without a mouse.
49. As a screen-reader user, I want each button announced as a menu that is open or closed, so that I know what clicking it does.
50. As a Learner, I want the account menu (my name, Admin area, Log out) and the language switch to stay where they are, so that the change doesn't move things I already use.

## Implementation Decisions

- **Header**: the shared nav-links block in the layout becomes eight menu groups, each a native disclosure element like the existing account menu (click to open, no hover). The same block feeds the wide header and the phone Menu, so both stay in step. Opening one menu closes the others; this is the only script the header needs.
- **Labels**: all new strings go in the text catalog in both languages, under the existing `nav.*` keys plus new ones. Admin content (Programme text) stays as written, per ADR 0005.
- **Home anchors**: the home page sections get stable ids (welcome, coach, about). Company background is left out entirely until its text and section exist.
- **Courses menu**: the layout needs the latest 3 open Courses on every page. The shared page data that already carries the Learner and language gains this list, loaded once per page from the courses table (open Courses, newest first, limit 3).
- **Lab**: new catalog string for "From static dashboard to hosted dashboard", used as the menu subtitle and the Lab page headline.
- **My exams** (new Learner page, login required): one row per enrolled Course that has an Exam, showing the Course, status (not taken, passed, failed), best score and pass mark, and a link to the Exam. Built from the existing enrollments, exams and exam attempts tables. No schema change.
- **Plan price** (schema change): Plans gain an optional price in whole ringgit (nullable integer). The Admin Plan form gains a price field. An empty price means "no price shown".
- **Plans page** (new public page): every Plan with name, length in days, Deployment limit, price if set, and a Buy button when a payment link is set. CaveText Pro is shown as its own card, using its existing price and payment link from the CaveText settings.
- **My membership** (new Learner page, login required): current Plan name and the end of the last queued Plan period, CaveText Pro status and end date, and Snippet cap. Read from the existing plan periods and CaveText Pro data. No schema change.
- **Programmes** (new table and pages): a Programme has a title, a start date, a blurb, an optional picture and an optional link. Pictures are uploaded the same way as Gallery pictures but are not Gallery items. The public Program page lists Programmes whose start date is today or later, soonest first. A new Admin sidebar section lets the Admin list (past included), add, edit and delete Programmes.
- **Certification menu**: My certificates appears only for a logged-in Learner.
- **Community**: no button, no route.
- **Visitors and login-only pages**: Classroom items, My membership and My certificates use the existing login requirement, which sends a Visitor to log in and back.

## Testing Decisions

- A good test drives the site over HTTP as a Visitor, Learner or Admin and checks what comes back. It never reaches into handlers, queries or template internals.
- The single seam is the existing HTTP test harness (`site` with `get`, `post`, `loginAsNewAdmin`, `expectStatus`) against a real database. No new seam.
- Covered:
  - the header on any page shows the eight sections in both languages, the 3 latest open Courses, My certificates only when logged in, and no Community;
  - My exams statuses (not taken, passed, failed, best score) and the empty state;
  - an Admin sets a Plan price, it shows on the Plans page, and an unpriced Plan shows no price;
  - My membership with no Plan, an active Plan, a queued period, and CaveText Pro;
  - Programme add, edit and delete, a past Programme hidden from the public page but listed in the Admin area, and the empty state;
  - Visitors sent to log in from the login-only pages.
- Prior art: `plans_test.go` (Admin form round trips), `exam_attempts_test.go` (attempt outcomes), `cavetext_pro_test.go` (Pro status), `*_language_test.go` (both languages), `gallery_test.go` (picture uploads).
- Menu open/close behaviour and keyboard use are checked by hand in the browser, not in Go tests.

## Out of Scope

- **Assignment**: Learner uploads work, Admin grades it by hand. Own spec later; no Classroom item until it exists.
- **Community channel**: one channel, Admin-invited Learners, messages refreshed every few seconds. Own spec later; the Community button appears with it.
- **Company background**: the section and menu item wait for the client's text.
- **Footer**: unchanged.
- Selling Plans on the site itself: Buy still goes to the external payment link, and Plans are still given by the Admin or by CaveText Pro.

## Further Notes

- Glossary updated during grilling: new term **Programme** (an upcoming programme or learning module the Admin announces; not a Course), and **Plan** now has a price the Admin sets.
- The client's word for Shop > My membership was "Members". The glossary avoids "member" for a Learner, so the page is called My membership.
- Coach Adam may override any Malay label.
