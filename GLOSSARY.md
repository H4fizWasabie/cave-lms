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
The site owner (the client), the only person who builds Courses or uploads Gallery items.
_Avoid_: Instructor, teacher, uploader

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
The area where Learners build Dashboards. Open to a Learner with an active Plan who is enrolled in a Course the Admin has marked as including the Lab.
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
