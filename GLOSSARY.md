# Cave LMS

CAVELMS is the product's public name ("Learn AI • Build Skills • Create Impact", powered by The Cave Consult). "Cave LMS" is the repo's working name. The Malay home page uses the client's own marketing copy word for word, and the English one translates it, so it says "online classes" for Courses and "programmes" for The Cave Consult's past in-person programmes, which are not Courses.

A website where anyone can register, join self-paced online courses made of lessons, sit an exam and earn a certificate, and browse a gallery of videos, ebooks, pictures and promotional material. Paying Learners also build and deploy dashboards in the Lab, from sample spreadsheets or their own.

The site is written in Malay and English. Each term gives the Malay word the site uses for it; product names stay in English, and Coach Adam may override any of them.

## Language

**Visitor**:
A person who has not logged in.
_Malay_: Pelawat
_Avoid_: Guest, anonymous user

**Learner**:
A person with an account on the site. Anyone can become one.
_Malay_: Pelajar
_Avoid_: User, member, student

**Admin**:
A person who runs the site: builds Courses, uploads Gallery items, sells Plans and takes down Deployments. Can use the Lab without a Plan.
_Malay_: Pentadbir
_Avoid_: Instructor, teacher, uploader

**Owner**:
The one Admin who can see the System log, named by an email address set on the server.
_Malay_: Pemilik
_Avoid_: Super admin, root

**System log**:
What the site records about its own running: errors and start-ups, kept 30 days. Only the Owner sees it.
_Malay_: Log sistem
_Avoid_: Audit log, activity log, history

**Registration**:
Creating a Learner account. Open to anyone, with email verification.
_Malay_: Pendaftaran
_Avoid_: Sign-up, enrollment

**Course**:
A self-paced, fully online programme the Admin builds from ordered Lessons, with an optional Exam. Learners can join it.
_Malay_: Kursus
_Avoid_: Programme, class, module

**Enrollment**:
A Learner's membership in a Course. Created when the Learner joins, with no Admin approval.
_Malay_: Penyertaan
_Avoid_: Registration, subscription, signup

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
Who may see a Gallery item: everyone, any logged-in Learner, or only Learners enrolled in a given Course.
_Malay_: Keterlihatan
_Avoid_: Permission, access level

**Lesson**:
One step of a Course: a Gallery item or text, with a title and an optional Objective. A Course is its ordered Lessons.
_Malay_: Pelajaran
_Avoid_: Chapter, unit, module

**Objective**:
One sentence saying what a Learner can do after a Lesson.
_Malay_: Objektif
_Avoid_: Learning outcome, goal, aim

**Course Outline**:
The ordered titles and Objectives of a Course's Lessons, before their content is written.
_Malay_: Rangka Kursus
_Avoid_: Syllabus, curriculum (and "plan", which means a Plan)

**Brief**:
The Admin's free-text description of a Course (topic, audience, tone) that the AI writes from.
_Malay_: Taklimat
_Avoid_: Prompt, instructions

**Draft**:
Course content the Admin has not yet published, such as anything the AI writes. Learners never see a Draft.
_Malay_: Draf
_Avoid_: Unpublished, pending, suggestion

**Exam**:
An auto-graded multiple-choice test belonging to a Course, with a pass mark.
_Malay_: Peperiksaan
_Avoid_: Quiz, test, assessment

**Certificate**:
A PDF of completion issued automatically when a Learner has finished a Course's Lessons and passed its Exam. It has a unique ID anyone can check.
_Malay_: Sijil
_Avoid_: Diploma, award

### Lab

**Lab**:
The area where Learners build Dashboards. Open to a Learner with an active Plan. Always open to the Admin.
_Malay_: Lab (kept in English)
_Avoid_: Workspace, studio, builder

**Plan**:
Paid access to the Lab for a set period, such as a year, with limits such as how many Deployments a Learner may have awake at once and their Storage allowance. Building Dashboards is unlimited. A Learner gets one with CaveText Pro, or the Admin assigns one by hand; a Learner without one sees only the demo. Courses stay free to join.
_Malay_: Pelan
_Avoid_: Tier, subscription, package, free plan

**Pro Plan**:
The one Plan the Admin marks as the one CaveText Pro comes with.
_Malay_: Pelan Pro
_Avoid_: Bundle, package

**Plan period**:
One stretch of a Learner's Plan, from its start to its end. A Plan assigned while another period is running starts when that one ends. When a Learner's last period ends, their Datasets are kept for 90 days, then deleted unless a new period has started.
_Malay_: Tempoh Pelan
_Avoid_: Subscription, term, cycle

**Storage allowance**:
How much Dataset data a Learner may keep while they have an active Plan period, such as 1 GB.
_Malay_: Had storan
_Avoid_: Quota, space, storage plan, subscription

**Template**:
A dashboard design with blanks for columns, such as Finance, Inventory, HR or General.
_Malay_: Templat
_Avoid_: Theme, layout, preset

**Dashboard question**:
The one thing a Template's Dashboard answers, shown under its name. General has none. Unrelated to the questions in an Exam.
_Malay_: Soalan Dashboard
_Avoid_: Goal, purpose, insight

**Sample**:
The Excel file Cave provides for a Template, showing the shape of data it expects. A Learner may edit it or upload their own data instead.
_Malay_: Sampel
_Avoid_: Example file, demo data

**Dataset**:
A Learner's Excel data, their own or an edited Sample, uploaded to the Lab. It belongs to exactly one Dashboard and is replaced by each new upload. Never a Gallery item.
_Malay_: Set data
_Avoid_: Upload, spreadsheet, data file

**Dashboard**:
A Template filled with one Learner's Dataset and their choice of column for each blank.
_Malay_: Dashboard (kept in English)
_Avoid_: Report, chart, app

**Deployment**:
A Dashboard put online for anyone with its link. It is awake while the Learner has an active Plan period, and updating its Dataset updates it. When the period ends it sleeps: its link shows it as inactive and nothing is deleted. It wakes, at the same link, when the Learner's next Plan period starts. Its link carries a name the Learner picks when deploying; the name never changes and is never given to another Deployment.
_Malay_: Deployment (kept in English)
_Avoid_: Publish (that is for Course content), hosting, site

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
_Avoid_: Template (that is for the Lab), prompt

**Folder**:
A named group a Learner puts their Snippets in.
_Malay_: Folder

**Snippet cap**:
How many Snippets a free Learner may keep: the early cap for an Early place, otherwise the late cap. Over it a Learner can still edit and delete, but not add. The Owner sets both caps and the number of early places.
_Malay_: Had Snippet
_Avoid_: Limit, quota

**Early place**:
A place within the first early places, a number the Owner sets. A Learner's place is handed out at their first CaveText sign-in and kept for good.
_Malay_: Tempat awal

**CaveText Pro**:
The paid yearly bundle, one level only, at a price the Admin sets. It raises the Snippet cap, opens the Master prompts, and comes with a Plan for the same year. Courses stay free.
_Malay_: CaveText Pro (kept in English)
_Avoid_: Subscription, storage service

**Pro payment**:
One paid order for CaveText Pro on the payment form. It gives a year of CaveText Pro, and its Plan, at once to the Learner whose CAVELMS email the order names, or failing that the payer's email. Each order counts once. One that names no Learner, pays less than the price or was paid on a form other than the current payment link is unmatched: it waits for an Admin to give Pro by hand and mark it done. Test Mode payments never count.
_Malay_: Bayaran Pro
_Avoid_: Subscription, purchase, transaction

**Master prompt**:
A ready prompt a CaveText Pro Learner inserts with one click or its shortcut. There are three. The User master prompt is the Learner's own context, written by an AI chatbot they asked what it knows about them and pasted into CaveText; it cannot be edited, only deleted and replaced, and it sits above a Snippet's task when inserted. The Image and Dashboard master prompts are written by the client, the same for every Pro Learner, and inserted as they are.
_Malay_: Master prompt (kept in English)
_Avoid_: Meta prompt, profile, system prompt, Prompt set
