package app

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The kinds of next step, in the order a Learner meets them.
const (
	stepLesson      = "lesson"      // the first Lesson not done
	stepExam        = "exam"        // the first Exam not passed whose Result is not pending
	stepAwaiting    = "awaiting"    // a Result still to be released
	stepCertificate = "certificate" // the Certificate, once issued
	stepUpcoming    = "upcoming"    // a Live Course that hasn't ended, with nothing to do yet
	stepComplete    = "complete"    // nothing left to do
)

// nextStep is what an enrolled Learner should do next on a Course: one Kind, its label and where it leads. The
// Course page, the Lesson footer and My courses all show it.
type nextStep struct {
	Kind string
	Key  string // the label's text catalog key
	N    int    // the Lesson's number on the Course, from 1, for a Lesson step
	Exam string // the text catalog key of the Exam's name, for an Exam or Awaiting step; in an Exam step's label
	URL  string // empty for Upcoming and Complete
	c    course // the Course, whose dates an Upcoming step's label gives
}

// Label is the step's label in lang, such as "Continue to Lesson 3" or "Take the Written Exam".
func (s nextStep) Label(lang string) string {
	switch {
	case s.N > 0:
		return text.T(lang, s.Key, s.N)
	case s.Kind == stepExam:
		return text.T(lang, s.Key, text.T(lang, s.Exam))
	case s.Key == "next.runs":
		return text.T(lang, s.Key, s.c.StartOn(lang), s.c.EndOn(lang))
	case s.Kind == stepUpcoming:
		return text.T(lang, s.Key, s.c.StartOn(lang))
	}
	return text.T(lang, s.Key)
}

// nextStep works out the Learner's next step on a Course they have joined:
//  1. the first published Lesson not done;
//  2. else the first Exam (Multiple-choice, then Written) not passed whose Result is not pending;
//  3. else Awaiting Result while one is pending;
//  4. else their Certificate, once issued;
//  5. else, on a Live Course that hasn't ended, when it starts or runs;
//  6. else Complete.
//
// An Exam with no published question can't be sat, so it is skipped, as the Certificate rule skips it.
// ponytail: a few queries per Course; batch them if My courses grows past a few dozen Courses per Learner.
func (a *App) nextStep(ctx context.Context, l *learner, c course) (nextStep, error) {
	lessons, err := a.lessonsOf(ctx, c.ID, false)
	if err != nil {
		return nextStep{}, err
	}
	completed, err := a.completedLessons(ctx, l.ID, c.ID)
	if err != nil {
		return nextStep{}, err
	}
	anyDone := false
	for _, ls := range lessons {
		anyDone = anyDone || completed[ls.ID]
	}
	for i, ls := range lessons {
		if !completed[ls.ID] {
			key := "next.lesson"
			if !anyDone {
				key = "next.lesson-start"
			}
			return nextStep{Kind: stepLesson, Key: key, N: i + 1,
				URL: "/courses/" + strconv.FormatInt(c.ID, 10) + "/lessons/" + strconv.FormatInt(ls.ID, 10)}, nil
		}
	}
	exams, err := a.courseExams(ctx, l, c, len(lessons), len(lessons))
	if err != nil {
		return nextStep{}, err
	}
	var awaiting *nextStep
	for _, x := range exams {
		st := x.Status
		step := nextStep{Kind: stepExam, Exam: x.Name, URL: x.URL}
		switch {
		case st.Passed:
			continue
		case st.Pending != nil:
			if awaiting == nil {
				step.Kind, step.Key = stepAwaiting, "next.awaiting"
				awaiting = &step
			}
			continue
		case st.Open != nil:
			step.Key = "next.exam-resume"
		case c.Live() && c.Upcoming: // can't be taken before the start date, so the start date is the step
			continue
		case st.Blocker(text.Languages[0]) != "": // out of attempts, or a Live Course's Exam not open: its page says why
			step.Key = "next.exam-view"
		case st.Used > 0:
			step.Key = "next.exam-retake"
		default:
			step.Key = "next.exam"
		}
		return step, nil
	}
	if awaiting != nil {
		return *awaiting, nil
	}
	var certificateID string
	err = a.db.QueryRowContext(ctx, `SELECT id FROM certificates
		WHERE learner_id = $1 AND course_id = $2 AND revoked_at IS NULL`, l.ID, c.ID).Scan(&certificateID)
	switch {
	case errors.Is(err, sql.ErrNoRows) && c.Live() && !c.Ended:
		step := nextStep{Kind: stepUpcoming, Key: "next.runs", c: c}
		switch {
		case c.Upcoming:
			step.Key = "next.starts"
		case c.EndDate == c.StartDate:
			step.Key = "next.runs-day"
		}
		return step, nil
	case errors.Is(err, sql.ErrNoRows):
		return nextStep{Kind: stepComplete, Key: "next.complete"}, nil
	case err != nil:
		return nextStep{}, err
	}
	return nextStep{Kind: stepCertificate, Key: "next.certificate", URL: "/my-certificates"}, nil
}

// courseExam is one usable Exam of a Course and where the Learner stands on it.
type courseExam struct {
	Name   string // the text catalog key of the Exam's name
	URL    string
	Status examStatus
}

// badge is a status badge: its text catalog key and its badge class.
type badge struct{ Key, Class string }

// Badge is the Learner's standing on the Exam as My courses shows it. Awaiting Result comes from the same pending
// attempt that makes it the next step.
func (x courseExam) Badge() badge {
	switch st := x.Status; {
	case st.Passed:
		return badge{"my-exams.passed", "badge-open"}
	case st.Pending != nil:
		return badge{"next.awaiting", "badge-joined"}
	case st.Open != nil:
		return badge{"my-courses.exam-open", "badge-joined"}
	case st.Used > 0:
		return badge{"my-exams.failed", "badge-revoked"}
	}
	return badge{"my-exams.not-taken", "badge-closed"}
}

// courseExams reads where the Learner stands on each usable Exam of a Course they have joined, Multiple-choice
// first; done and total count their Lessons. An Exam with no published question isn't usable, so it isn't listed.
func (a *App) courseExams(ctx context.Context, l *learner, c course, done, total int) ([]courseExam, error) {
	var list []courseExam
	for _, format := range []string{multipleChoice, written} {
		e, err := a.loadExam(ctx, c.ID, format)
		if err != nil {
			return nil, err
		}
		if !e.Usable {
			continue
		}
		x := courseExam{Name: "exam.name", URL: e.Path(""),
			Status: examStatus{Enrolled: true, Course: c, Exam: e, Done: done, Total: total}}
		if e.Written() {
			x.Name = "exam.name.written"
		}
		if err := x.Status.readAttempts(ctx, a.db, l.ID, e.ID); err != nil {
			return nil, err
		}
		list = append(list, x)
	}
	return list, nil
}
