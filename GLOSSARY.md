# Cave LMS

CAVELMS is the product's public name ("Learn AI • Build Skills • Create Impact", powered by The Cave Consult). "Cave LMS" is the repo's working name. The home page uses the client's own marketing copy word for word, so it says "online classes" for Courses and "programmes" for The Cave Consult's past in-person programmes, which are not Courses.

A website where anyone can register, join self-paced online courses made of lessons, sit an exam and earn a certificate, and browse a gallery of videos, ebooks, pictures and promotional material. Paying Learners also build and deploy dashboards from sample spreadsheets in the Lab.

## Language

**Visitor**:
A person who has not logged in.
_Avoid_: Guest, anonymous user

**Learner**:
A person with an account on the site. Anyone can become one.
_Avoid_: User, member, student

**Admin**:
A person who runs the site: builds Courses, uploads Gallery items, sells Plans and takes down Deployments. Can use the Lab without a Plan.
_Avoid_: Instructor, teacher, uploader

**Owner**:
The one Admin who can see the System log, named by an email address set on the server.
_Avoid_: Super admin, root

**System log**:
What the site records about its own running: errors and start-ups, kept 30 days. Only the Owner sees it.
_Avoid_: Audit log, activity log, history

**Registration**:
Creating a Learner account. Open to anyone, with email verification.
_Avoid_: Sign-up, enrollment

**Course**:
A self-paced, fully online programme the Admin builds from ordered Lessons, with an optional Exam. Learners can join it.
_Avoid_: Programme, class, module

**Enrollment**:
A Learner's membership in a Course. Created when the Learner joins, with no Admin approval.
_Avoid_: Registration, subscription, signup

**Gallery**:
The collection of everything the Admin uploads.
_Avoid_: Library, media library

**Gallery item**:
One uploaded thing in the Gallery.
_Avoid_: Asset, media, file

**Format**:
What a Gallery item is: video, ebook or picture.
_Avoid_: Type, kind

**Promotional material**:
Any Gallery item the Admin flags as promotional, whatever its Format. Promotional material is public by default.
_Avoid_: Ad, marketing asset

**Visibility**:
Who may see a Gallery item: everyone, any logged-in Learner, or only Learners enrolled in a given Course.
_Avoid_: Permission, access level

**Lesson**:
One step of a Course: a Gallery item or text, with a title and an optional Objective. A Course is its ordered Lessons.
_Avoid_: Chapter, unit, module

**Objective**:
One sentence saying what a Learner can do after a Lesson.
_Avoid_: Learning outcome, goal, aim

**Course Outline**:
The ordered titles and Objectives of a Course's Lessons, before their content is written.
_Avoid_: Syllabus, curriculum (and "plan", which means a Plan)

**Brief**:
The Admin's free-text description of a Course (topic, audience, tone) that the AI writes from.
_Avoid_: Prompt, instructions

**Draft**:
Course content the Admin has not yet published, such as anything the AI writes. Learners never see a Draft.
_Avoid_: Unpublished, pending, suggestion

**Exam**:
An auto-graded multiple-choice test belonging to a Course, with a pass mark.
_Avoid_: Quiz, test, assessment

**Certificate**:
A PDF of completion issued automatically when a Learner has finished a Course's Lessons and passed its Exam. It has a unique ID anyone can check.
_Avoid_: Diploma, award

### Lab

**Lab**:
The area where Learners build Dashboards. Open to a Learner with an active Plan who is enrolled in a Course the Admin has marked as including the Lab. Always open to the Admin.
_Avoid_: Workspace, studio, builder

**Plan**:
Paid access to the Lab for a set period, with limits such as how many Deployments a Learner may make in that period. Building Dashboards is unlimited. The Admin assigns it to a Learner; a Learner without one sees only the demo. Courses stay free to join.
_Avoid_: Tier, subscription, package, free plan

**Plan period**:
One stretch of a Learner's Plan, from its start to its end, with its own Deployment allowance. A Plan assigned while another period is running starts when that one ends. Unused Deployments expire with the period.
_Avoid_: Subscription, term, cycle

**Template**:
A dashboard design with blanks for columns, such as Finance, Inventory, HR or General.
_Avoid_: Theme, layout, preset

**Dashboard question**:
The one thing a Template's Dashboard answers, shown under its name. General has none. Unrelated to the questions in an Exam.
_Avoid_: Goal, purpose, insight

**Sample**:
The Excel file Cave provides for a Template. Learners only ever work with Samples, never their own data.
_Avoid_: Example file, demo data

**Dataset**:
A Learner's edited copy of a Sample, uploaded to the Lab. It belongs to exactly one Dashboard and is replaced by each new upload. Never a Gallery item.
_Avoid_: Upload, spreadsheet, data file

**Dashboard**:
A Template filled with one Learner's Dataset and their choice of column for each blank.
_Avoid_: Report, chart, app

**Deployment**:
A Dashboard put online for anyone with its link, once, for 7 days. Updating its Dataset updates it while it is awake. Afterwards it sleeps: its link shows it as inactive and nothing is deleted, and it cannot be deployed again.
_Avoid_: Publish (that is for Course content), hosting, site

**Takedown**:
The Admin putting a Deployment to sleep early, with a recorded reason. The Dashboard itself stays.
_Avoid_: Ban, removal, unpublish

**CaveText**:
The browser extension that turns a typed shortcut in an AI chat box into a saved prompt. A Learner signs in to it with their CAVELMS account.
_Avoid_: Snippet app, plugin

**Snippet**:
A saved prompt in CaveText: a name, a shortcut and a body, sitting in a Folder. Each Learner keeps their own, up to their Snippet cap.
_Avoid_: Template (that is for the Lab), prompt

**Folder**:
A named group a Learner puts their Snippets in.

**Snippet cap**:
How many Snippets a free Learner may keep: the early cap for an Early place, otherwise the late cap. Over it a Learner can still edit and delete, but not add. The Owner sets both caps and the number of early places.
_Avoid_: Limit, quota

**Early place**:
A place within the first early places, a number the Owner sets. A Learner's place is handed out at their first CaveText sign-in and kept for good.

**CaveText Pro**:
A paid yearly upgrade to CaveText, one level only, bought separately from Courses and from Plans. It raises the Snippet cap and opens the Master prompts.
_Avoid_: Plan (that is for the Lab), subscription

**Master prompt**:
A ready prompt a CaveText Pro Learner inserts with one click or its shortcut. There are three. The User master prompt is the Learner's own context, written by an AI chatbot they asked what it knows about them and pasted into CaveText; it cannot be edited, only deleted and replaced, and it sits above a Snippet's task when inserted. The Image and Dashboard master prompts are written by the client, the same for every Pro Learner, and inserted as they are.
_Avoid_: Meta prompt, profile, system prompt, Prompt set
