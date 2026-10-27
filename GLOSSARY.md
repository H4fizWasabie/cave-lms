# Cave LMS

CAVELMS is the product's public name ("Learn AI • Build Skills • Create Impact", powered by The Cave Consult). "Cave LMS" is the repo's working name. The Malay home page uses the client's own marketing copy word for word, and the English one translates it, so it says "online classes" for Courses and "programmes" for The Cave Consult's past in-person programmes, which are not Courses.

A website where anyone can register, join self-paced online courses made of lessons, sit an exam and earn a certificate, and browse a gallery of videos, ebooks, pictures and promotional material. Paying Learners also put dashboards they made with an AI chatbot online in the Lab, and keep their data up to date.

The site is written in Malay and English. Each term gives the Malay word the site uses for it; product names stay in English, and Coach Adam may override any of them.

## Language

**Visitor**:
A person who has not logged in.
_Malay_: Pelawat
_Avoid_: Guest, anonymous user

**Learner**:
A person with an account on the site. Anyone can become one. Their name is their real name, printed on their Certificates; their Username stands for them in links.
_Malay_: Pelajar
_Avoid_: User, member, student

**Username**:
The name a Learner picks to stand for them in links, such as their folder and their Deployments, so their real name stays private. Unique; picked when registering, and can be changed later, when their old links lead to the new one.
_Malay_: Nama pengguna
_Avoid_: Handle, slug, display name

**Profile picture**:
A Learner's own picture, shown beside their name in the header and in Chats; without one, a circle with their initials shows. Stored as a 256px square JPEG; it does not count against the Storage allowance.
_Malay_: Gambar profil
_Avoid_: Avatar, photo, display picture

**Admin**:
A person who runs the site: builds Courses, uploads Gallery items, sells Plans and takes down Deployments. Can use the Lab without a Plan.
_Malay_: Pentadbir
_Avoid_: Instructor, teacher, uploader

**Owner**:
The one Admin who can see the System log, named by an email address set on the server.
_Malay_: Pemilik
_Avoid_: Super admin, root

**System log**:
What the site records about its own running: errors, start-ups and events such as an email's logins being limited, kept 30 days. Only the Owner sees it.
_Malay_: Log sistem
_Avoid_: Audit log, activity log, history

**Registration**:
Creating a Learner account. Open to anyone, with email verification. Registering with an email that already has an account answers the same as a new one, makes nothing, and emails that account instead: a link to set its password if it is unverified, else a note that someone tried to register with it.
_Malay_: Pendaftaran
_Avoid_: Sign-up, enrollment

**Course**:
A programme the Admin builds from ordered Lessons, with an optional Exam. Learners can join it. Self-paced unless it is a Live Course.
_Malay_: Kursus
_Avoid_: Programme, class, module

**Live Course**:
A Course run for an Organisation on agreed dates, made by the Admin duplicating a Course: it has its own copies of the Lessons, Exams and Assignments, a start date, an end date and a number of days (1 to 5) that need not match the dates. Learners can join it until the end of its end date, Malaysia time; after that it is closed but still viewable.
_Malay_: Kursus Langsung
_Avoid_: Cohort, class, session, event

**Organisation**:
The body a Live Course is run for, such as a ministry, a police force or a training institute, with a name and a logo printed on its Live Courses' Certificates.
_Malay_: Organisasi
_Avoid_: Company, client (which means The Cave Consult), agency

**PCC Level**:
The level of a Live Course, set by its days: 1 Basic, 2 Foundation, 3 Practitioner, 4 or 5 Professional. Each Level has the client's badge. Never picked by hand.
_Malay_: Tahap PCC
_Avoid_: Grade, tier, rank

**Certificate No.**:
A Certificate's running number, shown as `PCC-LMS-` + the year's last two digits + four digits, such as `PCC-LMS-260001`; a year past 9999 Certificates gets a fifth digit. It runs in issue order, restarts at 0001 each year and is never reused. `LMS` sets it apart from the certificates the client issues outside CAVELMS (`PCC-2026-00509`). Certificates from before this format show the new one; the check page still finds them by the old `PCC-2026-00001`.
_Malay_: No. Sijil
_Avoid_: Certificate ID (that is the unguessable ID in its link), serial

**IC**:
A Learner's MyKad number or, without a MyKad, passport number, printed on their Certificates. A 12-digit MyKad is shown as `YYMMDD-PB-####`. The Learner sets it once, and must before their first Exam attempt; only the Admin can change it.
_Malay_: No. KP
_Avoid_: NRIC, ID number

**Enrollment**:
A Learner's membership in a Course. Created when the Learner joins, with no Admin approval.
_Malay_: Penyertaan
_Avoid_: Registration, subscription, signup

**Chat**:
A shared conversation of Messages. Each Course, Live Courses included, has its own, open only to its enrolled Learners and the Admin; the Community Chat is the one Chat not tied to a Course.
_Malay_: Sembang
_Avoid_: Channel, forum, group, discussion, room

**Community Chat**:
The one Chat for the whole site, not tied to any Course. Any verified account may join it and leave it; only those who joined, and the Admin, read and post in it.
_Malay_: Sembang Komuniti
_Avoid_: Open channel, lounge, forum, community (alone)

**Message**:
One text post in a Chat, by a Learner or the Admin, optionally replying to an earlier Message. Hidden by the Admin or deleted by its writer, it shows as removed.
_Malay_: Mesej
_Avoid_: Post, comment, chat (for one post)

**Reaction**:
One of six fixed emoji a person puts on a Message; each person has at most one per Message.
_Malay_: Reaksi
_Avoid_: Like, emoji vote

**Report**:
A Learner's flag that a Message is abusive, with an optional reason, for the Admin to hide the Message or dismiss the Report.
_Malay_: Laporan
_Avoid_: Complaint, flag

**Gallery**:
The collection of everything the Admin uploads.
_Malay_: Galeri
_Avoid_: Library, media library

**Gallery item**:
One uploaded thing in the Gallery.
_Malay_: Item Galeri
_Avoid_: Asset, media, file

**Format**:
What a Gallery item is: video, ebook or picture.
_Malay_: Format
_Avoid_: Type, kind

**Promotional material**:
Any Gallery item the Admin flags as promotional, whatever its Format. Promotional material is public by default.
_Malay_: Bahan promosi
_Avoid_: Ad, marketing asset

**Visibility**:
Who may see a Gallery item: everyone, any logged-in Learner, or only Learners enrolled in a given Course. An ebook that is not Promotional material is listed by its Visibility but opens only with Ebook access.
_Malay_: Keterlihatan
_Avoid_: Permission, access level

**Ebook access**:
The right to read in the browser the Gallery's ebooks (only Admins download them), beyond the ones that are Promotional material. A Learner gets it by paying for the Pro Plan with the code of a Promo that comes with Ebooks, or from the Admin giving the COMBO Pro Plan by hand, and keeps it for that year of CaveText Pro only. Anyone else sees a locked ebook. Admins always have it.
_Malay_: Akses Ebook
_Avoid_: Ebook subscription, premium ebooks

**Ebook email**:
The email a Learner gets each time a year of Ebook access is given, by a COMBO Pro Plan payment or by the Admin, and again whenever the Admin resends it. It links to the Gallery's ebooks and says until when they stay open; it never carries an ebook, so access still ends with that year.
_Malay_: E-mel Ebook
_Avoid_: Ebook delivery, download email

**COMBO Pro Plan**:
The Pro Plan bought with the code of a Promo that comes with Ebooks, or given so by the Admin: the Pro Plan with Ebook access for its year. It is the one place "combo" names something; the Pro Plan alone is never a combo.
_Malay_: Pelan Pro COMBO
_Avoid_: Bundle, Pro Plan promo

**Lesson**:
One step of a Course: a title naming its topic, up to five Objectives, and its content: text, a Gallery item, or text with either a Gallery item or a Lesson image above it. The AI can write the text from the title and Objectives, for the Admin to check before saving. A Course is its ordered Lessons.
_Malay_: Pelajaran
_Avoid_: Chapter, unit, module, lesson plan

**Lesson image**:
A picture the Admin uploads to one Lesson, with a short description for Learners who cannot see it. It belongs to that Lesson alone, is never a Gallery item, and goes when the Lesson goes.
_Malay_: Imej Pelajaran
_Avoid_: Lesson picture, cover, thumbnail

**Objective**:
One sentence saying what a Learner can do after a Lesson. A Lesson has up to five.
_Malay_: Objektif
_Avoid_: Learning outcome, goal, aim

**Draft**:
Course content the Admin has not yet published. A new Lesson saves as a Draft unless the Admin publishes it at once; Exam questions the AI writes are Drafts. AI-written Lesson text goes into the Lesson form for the Admin to check before saving. Learners never see a Draft.
_Malay_: Draf
_Avoid_: Unpublished, pending, suggestion

**Exam**:
A timed test belonging to a Course, in one format, with a pass mark and an attempt limit. A Course has at most one Exam per format.
_Malay_: Peperiksaan
_Avoid_: Quiz, test, assessment

**Multiple-choice Exam**:
An Exam whose questions each have 2 to 5 options and one correct answer, marked automatically.
_Malay_: Peperiksaan Objektif
_Avoid_: Objective exam, MCQ, quiz

**Written Exam**:
An Exam whose questions the Learner answers in their own words, marked by the AI against a model answer, which the Admin can override.
_Malay_: Peperiksaan Subjektif
_Avoid_: Subjective exam, essay test

**Result**:
A Learner's mark and pass or fail on one Exam attempt, released 10 minutes after they submit, on the site and by email. It never shows answers; those come only once the Learner passes.
_Malay_: Keputusan
_Avoid_: Grade, score report

**Answer sheet**:
A PDF of a passed Exam attempt: every question with the Learner's answer and the correct or model answer. Emailed with the pass Result and downloadable after.
_Malay_: Skema Jawapan
_Avoid_: Answer key, marking scheme, transcript

**Assignment**:
A task in a Course, with a title and instructions, that Learners answer by uploading files. It earns no mark and does not affect a Certificate.
_Malay_: Tugasan
_Avoid_: Homework, project, task

**Submission**:
A Learner's files for one Assignment, which the Admin downloads and comments on.
_Malay_: Serahan
_Avoid_: Upload, hand-in, entry

**Folder**:
A Learner's private page listing their Submissions by Course, with the Admin's comments, and, with a Plan, their Workbooks and their Dashboards' pages. Only they and the Admin see it.
_Malay_: Folder
_Avoid_: Dashboard (which means a Lab Dashboard), drive, locker

**Question bank**:
Every Exam question ever written or generated, site-wide, each tagged with its topic, format and, when known, its Difficulty. An Exam copies its questions from it.
_Malay_: Bank Soalan
_Avoid_: Library, question set

**Difficulty**:
How hard an Exam question is: Easy, Medium or Hard. Only the Admin sees it.
_Malay_: Tahap kesukaran (Mudah, Sederhana, Sukar)
_Avoid_: Level (which means a PCC Level), grade

**Certificate**:
A PDF of completion issued automatically once a Learner with an IC has passed every one of a Course's Exams and, on a self-paced Course, finished its Lessons. It is issued when the passing Result is released, never at submit, and copies the Learner's name and IC and the Course's PCC Level at issue. It has a unique ID anyone can check.
_Malay_: Sijil
_Avoid_: Diploma, award

**Programme**:
An upcoming programme or learning module The Cave Consult announces, which the Admin writes up by hand. It is not a Course: Learners cannot join it on the site, only follow its link.
_Malay_: Program
_Avoid_: Course, event, module

### Lab

**Lab**:
The area where Learners put their Dashboards online and upload their data. Open to a Learner with an active Plan, and always to the Admin.
_Malay_: Lab (kept in English)
_Avoid_: Workspace, studio, builder

**Plan**:
Paid access to the Lab for a set period, such as a year, with limits such as how many Deployments a Learner may have awake at once and their Storage allowance. A Learner gets one by buying the Lab Plan or the Pro Plan, or the Admin assigns one by hand; a Learner without one has no Lab. Courses stay free to join.
_Malay_: Pelan
_Avoid_: Tier, subscription, package, free plan

**Lab Plan**:
The one Plan the Admin marks to be sold on its own, a year of the Lab with its hosting and storage, at a price the Admin sets. The Pro Plan comes with the same Plan, with the same limits. Other Plans exist only to be assigned by hand.
_Malay_: Pelan Lab
_Avoid_: Lab subscription, hosting plan

**Pro Plan**:
CaveText Pro and the Lab Plan together, bought in one Payment, at a price the Admin sets, no more than the two bought apart. It gives nothing the two do not give separately.
_Malay_: Pelan Pro
_Avoid_: Bundle, package, combo

**Normal price**:
The price a Learner would pay without a promotion, shown crossed out beside a lower current price. Only the current price, or a running Promo's price, decides whether a Payment is enough.
_Malay_: Harga biasa
_Avoid_: Original price, list price, RRP

**Promo**:
A lower price for one product between two dates, which a Learner gets by entering the Promo's code on that product's payment form. The code itself lives in BCL, which limits how many times it is used in all (it cannot limit one person to one use) and which payment forms take it; CAVELMS only accepts a Payment of at least the Promo's price while the Promo runs. Several Promos may run at once; the lowest running price counts. Ending a Promo early keeps it listed. Admins never use a Promo: they give products by hand. A Pro Plan Promo may come with Ebooks: paying with its code gives Ebook access too, which the COMBO Pro Plan card advertises.
_Malay_: Promosi
_Avoid_: Coupon, voucher, discount

**Plan period**:
One stretch of a Learner's Plan, from its start to its end. A Plan assigned while another period is running starts when that one ends. When a Learner's last period ends, their Workbooks, Datasets and Deployment pages are kept for 90 days, then deleted unless a new period has started.
_Malay_: Tempoh Pelan
_Avoid_: Subscription, term, cycle

**Storage allowance**:
How much Workbook, Dataset and Deployment page data a Learner may keep while they have an active Plan period, such as 1 GB.
_Malay_: Had storan
_Avoid_: Quota, space, storage plan, subscription

**Dataset**:
The table Cave keeps for a Dashboard: one Part or more with the same columns, each cleaned from a Workbook after Clean-up and the Learner's confirmation. It belongs to exactly one Dashboard, so two Dashboards using one Workbook each have their own, and deleting the Workbook leaves it as it is. Adding data adds a Part; replacing the data makes the new Part the only one. Cave keeps no earlier Datasets: the Workbooks in the Folder are the way back. Never a Gallery item.
_Malay_: Set data
_Avoid_: Upload, spreadsheet, data file

**Part**:
The rows of one confirmed upload within a Dataset, from one Workbook, read the same way as the Dataset's first Part, with a date the Learner confirms. The Learner can replace or delete one Part without touching the others, and a replaced Part keeps its place in the order.
_Malay_: Bahagian
_Avoid_: Chunk, batch, period, file

**Workbook**:
A Learner's Excel file, kept as uploaded in their Folder, whether uploaded there or to a Dashboard. A Dashboard's Dataset is cleaned from a Workbook. Only a Learner with an active Plan can add one.
_Malay_: Buku kerja
_Avoid_: Upload, spreadsheet, data file, Dataset

**Clean-up**:
What Cave changes in an upload to turn it into a Dataset, such as skipping title rows, leaving `-` empty in a number column, reading a month name as 1 to 12 in a column that holds whole 1 to 12 only, or leaving empty the few cells that don't fit their column, shown to the Learner before they confirm.
_Malay_: Pembersihan
_Avoid_: Sanitise, normalise, transform, fix

**Dashboard**:
A Learner's dashboard in the Lab, with the Dataset it reads: either built by Cave from its Dataset and drawn in a Theme, or a page the Learner made, such as with an AI chatbot, and brought as one HTML file. Cave keeps it, its earlier pages or Datasets, and puts it online as a Deployment.
_Malay_: Dashboard (kept in English)
_Avoid_: Report, chart, app, web app, Template, Practice Dashboard and Work Dashboard (former kinds)

**Theme**:
How a Dashboard Cave builds looks: its colours, fonts and chart styles, one of five such as Corporate Executive or Midnight Intelligence, picked by the Learner and changeable at any time. Changing it never changes what the Dashboard shows.
_Malay_: Tema
_Avoid_: Template, skin, style, look

**Deployment**:
A Dashboard put online for anyone with its link. A Dashboard has at most one. Its page may read the Dashboard's current Dataset, so a new Dataset updates it; a new page replaces it at the same link, and the 3 earlier pages are kept to restore. It is awake while the Learner has an active Plan period. When the period ends it sleeps: its link shows it as inactive and nothing is deleted. It wakes, at the same link, when the Learner's next Plan period starts. Its link carries the Learner's Username and a name the Learner picks when deploying; that name never changes and is never given to another Deployment.
_Malay_: Deployment (kept in English)
_Avoid_: Publish (that is for Course content), hosting, site, web app

**Takedown**:
The Admin putting a Deployment to sleep early, with a recorded reason. The Dashboard itself stays.
_Malay_: Penurunan
_Avoid_: Ban, removal, unpublish

**CaveText**:
The browser extension that turns a typed shortcut in an AI chat box into a saved prompt. A Learner signs in to it with their CAVELMS account.
_Malay_: CaveText (kept in English)
_Avoid_: Snippet app, plugin

**Snippet**:
A saved prompt in CaveText: a name, a shortcut and a body, sitting in a Folder. Each Learner keeps their own, up to their Snippet cap.
_Malay_: Snippet (kept in English)
_Avoid_: Template, prompt

**Folder**:
A named group a Learner puts their Snippets in.
_Malay_: Folder

**Snippet cap**:
How many Snippets a Learner may keep: for a free Learner the early cap for an Early place, otherwise the late cap; with CaveText Pro, 2,000. Over it a Learner can still edit and delete, but not add. The Owner sets both caps and the number of early places.
_Malay_: Had Snippet
_Avoid_: Limit, quota

**Early place**:
A place within the first early places, a number the Owner sets. A Learner's place is handed out at their first CaveText sign-in and kept for good.
_Malay_: Tempat awal

**CaveText Pro**:
A paid year of CaveText's extras, one level only, at a price the Admin sets. It raises the Snippet cap to 2,000 and opens the Master prompts. It does not include the Lab: that is the Lab Plan, or both together as the Pro Plan. Buying it again starts a new year when the current one ends. Courses stay free.
_Malay_: CaveText Pro (kept in English)
_Avoid_: Subscription, storage service

**Payment**:
One paid order on the payment form of CaveText Pro, the Lab Plan or the Pro Plan. The form decides what it gives: a year of CaveText Pro, a period of the Lab Plan, or both, at once to the Learner whose CAVELMS email the order names, or failing that the payer's email. Each comes after any of the same kind still running, with no credit for time already paid. Each order counts once. One that names no Learner, pays, after any coupon's discount, less than that product's current price (or its lowest running Promo's price) or was paid on a form that is no current payment link is unmatched: it waits for an Admin to give it by hand and mark it done. Test Mode payments never count.
_Malay_: Bayaran
_Avoid_: Subscription, purchase, transaction, Pro payment

**Master prompt**:
A ready prompt a CaveText Pro Learner inserts with one click or its shortcut. There are three. The User master prompt is the Learner's own context, written by an AI chatbot they asked what it knows about them and pasted into CaveText; it cannot be edited, only deleted and replaced, and it sits above a Snippet's task when inserted. The Image and Dashboard master prompts are written by the client, the same for every Pro Learner, and inserted as they are.
_Malay_: Master prompt (kept in English)
_Avoid_: Meta prompt, profile, system prompt, Prompt set
