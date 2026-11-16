package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

// endAccessIn makes every Plan period and CaveText Pro period of the site end days from now (negative: ago), the Plan
// period 30 days long and the Pro year 365.
func (s *site) endAccessIn(days float64) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE plan_periods SET ends_at = now() + make_interval(secs => $1), starts_at = now() + make_interval(secs => $1) - interval '30 days'`, days*86400); err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE cavetext_pro_periods SET ends_at = now() + make_interval(secs => $1), starts_at = now() + make_interval(secs => $1) - interval '365 days'`, days*86400); err != nil {
		s.t.Fatal(err)
	}
}

// renewalEmails lists the emails sent to the address that are renewal reminders (or the deletion warning), in order.
func (s *site) renewalEmails(to string) []mailer.Message {
	var got []mailer.Message
	for _, m := range s.emailsTo(to) {
		if strings.Contains(m.Subject, "CAVELMS access") || strings.Contains(m.Subject, "Akses CAVELMS") ||
			strings.Contains(m.Subject, "Lab data") || strings.Contains(m.Subject, "Data Lab") {
			got = append(got, m)
		}
	}
	return got
}

func reminderDate(days float64) string {
	return time.Now().In(malaysiaTime).Add(time.Duration(days * 24 * float64(time.Hour))).Format("2 January 2006")
}

// lateInTheUTCDay is days from today in UTC at 20:00 UTC, already the next day in Malaysia.
func lateInTheUTCDay(days int) time.Time {
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, days).Add(20 * time.Hour)
}

func TestARenewalReminderSaysTheEndInMalaysiaTime(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("en", "/")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	ends := lateInTheUTCDay(20)
	for _, periods := range []string{"plan_periods", "cavetext_pro_periods"} {
		if _, err := s.db.Exec(`UPDATE `+periods+` SET ends_at = $1`, ends); err != nil {
			t.Fatal(err)
		}
	}
	s.prune()
	got := s.renewalEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("%d reminders, want 1", len(got))
	}
	day := ends.In(malaysiaTime).Format("2 January 2006")
	expectAll(t, got[0].Body, "Lab Plan: ends on "+day, "CaveText Pro and Ebook access: ends on "+day)
}

func TestTheDeletionWarningSaysItsDatesInMalaysiaTime(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	ada.readyDashboard("Kewangan")
	ada.switchLanguage("en", "/")
	ended := lateInTheUTCDay(-85)
	if _, err := admin.db.Exec(`UPDATE plan_periods SET starts_at = $1::timestamptz - interval '30 days', ends_at = $1`, ended); err != nil {
		t.Fatal(err)
	}
	admin.prune()
	got := admin.renewalEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("%d warnings, want 1", len(got))
	}
	myt := ended.In(malaysiaTime)
	expectAll(t, got[0].Body, myt.Format("2 January 2006"), myt.AddDate(0, 0, 90).Format("2 January 2006"))
}

// A COMBO buyer gets one email per stage that lists the Lab Plan and CaveText Pro with Ebook access, each at most once.
func TestAComboBuyerGetsOneReminderPerStageListingEveryItem(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("en", "/")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))

	s.endAccessIn(100)
	s.prune()
	if n := len(s.renewalEmails("ada@example.com")); n != 0 {
		t.Fatalf("100 days out: %d reminders, want none", n)
	}

	s.endAccessIn(29.5)
	s.prune()
	s.prune() // a second run the same day sends nothing more
	got := s.renewalEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("30 days out: %d reminders, want 1", len(got))
	}
	expectAll(t, got[0].Subject+got[0].Body, "Hi Ada Lovelace", "30 days", "Lab Plan: ends on "+reminderDate(29.5),
		"CaveText Pro and Ebook access: ends on "+reminderDate(29.5), "https://cave.example.invalid/pricing", "https://cave.example.invalid/my-membership")

	s.endAccessIn(6.5)
	s.prune()
	s.prune()
	if got = s.renewalEmails("ada@example.com"); len(got) != 2 || !strings.Contains(got[1].Subject, "7 days") {
		t.Fatalf("7 days out: reminders = %+v, want the 30-day one then the 7-day one", got)
	}

	s.endAccessIn(-0.5)
	s.prune()
	s.prune()
	if got = s.renewalEmails("ada@example.com"); len(got) != 3 || !strings.Contains(got[2].Subject, "has ended") {
		t.Fatalf("after it ended: reminders = %+v, want a third, the ended one", got)
	}
	expectAll(t, got[2].Body, "Lab Plan: ended on "+reminderDate(-0.5), "https://cave.example.invalid/pricing")
}

// A Learner whose end is 5 days away only gets the 7-day reminder, and the Malay one comes in the saved language.
func TestALaterStageSkipsTheEarlierOnesAndFollowsTheSavedLanguage(t *testing.T) {
	t.Parallel()
	s, _ := ebookSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("ms", "/")
	s.bclPay(couponProPlan("LINK-1", "ada@example.com", "249.00", "150.00"))
	s.endAccessIn(5)
	s.prune()
	got := s.renewalEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("%d reminders, want only the 7-day one", len(got))
	}
	expectAll(t, got[0].Subject+got[0].Body, "Hai Ada Lovelace", "7 hari", "Pelan Lab", "CaveText Pro dan akses Ebook", "/pricing", "/my-membership")
}

func TestPlainProHasNoEbookAccessInItsReminder(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("en", "/")
	s.givePro("ada@example.com")
	s.endAccessIn(20)
	s.prune()
	got := s.renewalEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("%d reminders, want 1", len(got))
	}
	expectAll(t, got[0].Body, "CaveText Pro: ends on")
	expectNone(t, got[0].Body, "Ebook", "Lab Plan")
}

func TestARenewedLearnerNeedsNoReminderAndTheNextYearGetsItsOwn(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	ada, _ := s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	ada.switchLanguage("en", "/")
	s.givePro("ada@example.com")
	s.endAccessIn(20)
	s.givePro("ada@example.com") // renewed: a second year waits after the first
	s.prune()
	if n := len(s.renewalEmails("ada@example.com")); n != 0 {
		t.Fatalf("a renewed Learner got %d reminders, want none", n)
	}
	s.agePro(365) // a year later the renewed year has 20 days left
	s.prune()
	if n := len(s.renewalEmails("ada@example.com")); n != 1 {
		t.Fatalf("the next year got %d reminders, want its own 1", n)
	}
}

func TestOtherPlansAndLongEndedAccessGetNoReminder(t *testing.T) {
	t.Parallel()
	admin, _ := labLearner(t) // Ada holds a hand-given Plan that is not the Lab Plan
	admin.endAccessIn(20)
	admin.prune()
	if n := len(admin.renewalEmails("ada@example.com")); n != 0 {
		t.Fatalf("a Learner on another Plan got %d reminders, want none", n)
	}

	s := labPlanSite(t)
	s.newCaveTextLearner("Bea Lim", "bea@example.com")
	s.givePro("bea@example.com")
	s.endAccessIn(-30) // ended a month before this deploy
	s.prune()
	if n := len(s.renewalEmails("bea@example.com")); n != 0 {
		t.Fatalf("access that ended long ago got %d reminders, want none", n)
	}
}

// The reminder is recorded before it is sent: a failed send is logged and never retried, and nothing is sent twice.
func TestAFailedReminderIsNotSentAgain(t *testing.T) {
	t.Parallel()
	s := labPlanSite(t)
	s.newCaveTextLearner("Ada Lovelace", "ada@example.com")
	s.givePro("ada@example.com")
	s.endAccessIn(20)
	s.mail.FailWith(errors.New("mail server down"))
	s.prune()
	s.mail.FailWith(nil)
	s.prune()
	if n := len(s.renewalEmails("ada@example.com")); n != 0 {
		t.Fatalf("%d reminders after a failed send, want none sent again", n)
	}
	if line := s.lastLogLine("mail server down"); !strings.Contains(line, "renewal reminder") {
		t.Fatalf("System log line = %q", line)
	}
}

func TestOneWeekBeforeLabDataIsDeletedTheLearnerIsWarnedOnce(t *testing.T) {
	t.Parallel()
	admin, ada := labLearner(t)
	ada.readyDashboard("Kewangan")
	ada.switchLanguage("en", "/")
	admin.ageAllPeriods(30 + 80) // ended 80 days ago
	admin.prune()
	if n := len(admin.renewalEmails("ada@example.com")); n != 0 {
		t.Fatalf("at 80 days %d warnings, want none", n)
	}
	admin.ageAllPeriods(5) // 85 days: 5 days left
	admin.prune()
	admin.prune()
	got := admin.renewalEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("at 85 days %d warnings, want 1", len(got))
	}
	expectAll(t, got[0].Body, "Workbooks", "Datasets", reminderDate(5), "https://cave.example.invalid/pricing", "https://cave.example.invalid/my-membership")
}

func TestNoDeletionWarningWithoutDataToLoseOrAfterRenewing(t *testing.T) {
	t.Parallel()
	admin, _ := labLearner(t) // Ada has no Workbook, Dataset or Dashboard
	admin.ageAllPeriods(30 + 85)
	admin.prune()
	if n := len(admin.renewalEmails("ada@example.com")); n != 0 {
		t.Fatalf("a Learner with nothing to lose got %d warnings, want none", n)
	}

	admin2, ada2 := labLearner(t)
	ada2.readyDashboard("Kewangan")
	admin2.ageAllPeriods(30 + 85)
	admin2.assignPlan("ada@example.com", admin2.onlyPlanID()) // renewed in time
	admin2.prune()
	if n := len(admin2.renewalEmails("ada@example.com")); n != 0 {
		t.Fatalf("a renewed Learner got %d warnings, want none", n)
	}
}

// teacherEmails lists the emails sent to the address that are Teachers Portal reminders, in order.
func (s *site) teacherEmails(to string) []mailer.Message {
	var got []mailer.Message
	for _, m := range s.emailsTo(to) {
		if strings.Contains(m.Subject, "Teachers Portal access") || strings.Contains(m.Subject, "Akses Teachers Portal") {
			got = append(got, m)
		}
	}
	return got
}

// A Teacher on the trial gets one email 3 days before it ends and one once it has ended, each linking to the pay page and
// My membership.
func TestATeacherTrialGetsOneReminderThreeDaysBeforeItEndsAndOneOnceItHasEnded(t *testing.T) {
	t.Parallel()
	s := newTeacher(t)
	s.switchLanguage("en", "/")

	s.ageTrial(10) // 4 days left
	s.prune()
	if n := len(s.teacherEmails("ada@example.com")); n != 0 {
		t.Fatalf("4 days out: %d reminders, want none", n)
	}

	s.ageTrial(11) // 3 days left
	s.prune()
	s.prune() // a second run the same day sends nothing more
	got := s.teacherEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("3 days out: %d reminders, want 1", len(got))
	}
	expectAll(t, got[0].Subject+got[0].Body, "Hi Ada", "3 days", "Teacher trial: ends on "+reminderDate(3),
		"https://cave.example.invalid/teachers", "https://cave.example.invalid/my-membership")

	s.ageTrial(14)
	s.prune()
	s.prune()
	if got = s.teacherEmails("ada@example.com"); len(got) != 2 || !strings.Contains(got[1].Subject, "has ended") {
		t.Fatalf("after it ended: reminders = %+v, want a second, the ended one", got)
	}
	expectAll(t, got[1].Body, "Teacher trial: ended on", "https://cave.example.invalid/teachers")
	if n := len(s.renewalEmails("ada@example.com")); n != 0 {
		t.Fatalf("a Teacher got %d Lab reminders, want none", n)
	}
}

func TestATeacherTrialReminderFollowsTheSavedLanguage(t *testing.T) {
	t.Parallel()
	s := newTeacher(t)
	s.switchLanguage("ms", "/")
	s.ageTrial(11)
	s.prune()
	got := s.teacherEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("%d reminders, want 1", len(got))
	}
	expectAll(t, got[0].Subject+got[0].Body, "Hai Ada", "3 hari", "Percubaan percuma", "/teachers", "/my-membership")
}

// endTeacherProIn makes every Teachers Portal Pro period of the site end days from now (negative: ago).
func (s *site) endTeacherProIn(days float64) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE teacher_pro_periods SET ends_at = now() + make_interval(secs => $1), starts_at = now() + make_interval(secs => $1) - interval '1 day'`, days*86400); err != nil {
		s.t.Fatal(err)
	}
}

// proTeacher makes an Admin site with Ada, a Teacher whose trial has ended and who holds a month or a year of Teachers
// Portal Pro, given by hand.
func proTeacher(t *testing.T, length string) *site {
	t.Helper()
	s := caveTextAdminSite(t)
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)
	ada.saveTeacherProfile("Sains", "Y4")
	ada.switchLanguage("en", "/")
	s.ageTrial(30)
	s.giveTeacherPro("ada@example.com", length)
	return s
}

// A yearly Teachers Portal Pro is reminded 30 days, 7 days before it ends and once it has ended.
func TestAYearOfTeachersPortalProIsRemindedAt30And7DaysAndOnceEnded(t *testing.T) {
	t.Parallel()
	s := proTeacher(t, "year")

	s.endTeacherProIn(100)
	s.prune()
	if n := len(s.teacherEmails("ada@example.com")); n != 0 {
		t.Fatalf("100 days out: %d reminders, want none", n)
	}

	s.endTeacherProIn(29.5)
	s.prune()
	s.prune()
	got := s.teacherEmails("ada@example.com")
	if len(got) != 1 {
		t.Fatalf("30 days out: %d reminders, want 1", len(got))
	}
	expectAll(t, got[0].Subject+got[0].Body, "Hi Ada", "30 days", "Teachers Portal Pro: ends on "+reminderDate(29.5),
		"https://cave.example.invalid/teachers", "https://cave.example.invalid/my-membership")

	s.endTeacherProIn(6.5)
	s.prune()
	if got = s.teacherEmails("ada@example.com"); len(got) != 2 || !strings.Contains(got[1].Subject, "7 days") {
		t.Fatalf("7 days out: reminders = %+v, want the 30-day one then the 7-day one", got)
	}

	s.endTeacherProIn(-0.5)
	s.prune()
	if got = s.teacherEmails("ada@example.com"); len(got) != 3 || !strings.Contains(got[2].Subject, "has ended") {
		t.Fatalf("after it ended: reminders = %+v, want a third, the ended one", got)
	}
	expectAll(t, got[2].Body, "Teachers Portal Pro: ended on "+reminderDate(-0.5), "https://cave.example.invalid/teachers")
}

// A Teacher who has paid for Teachers Portal Pro during the trial loses nothing when it ends, so gets no trial reminder.
func TestATeacherWhoHasPaidDuringTheTrialGetsNoTrialReminder(t *testing.T) {
	t.Parallel()
	s := caveTextAdminSite(t)
	ada := s.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", teachersPassword)
	ada.saveTeacherProfile("Sains", "Y4")
	ada.switchLanguage("en", "/")
	s.giveTeacherPro("ada@example.com", "month") // starts when the trial ends
	s.ageTrial(11)
	s.prune()
	s.ageTrial(15)
	s.prune()
	if n := len(s.teacherEmails("ada@example.com")); n != 0 {
		t.Fatalf("a Teacher with Pro waiting got %d trial reminders, want none", n)
	}
}

// Renewing moves the end, so a renewed Teacher gets no reminder until the last period nears its end, and then its own.
func TestARenewedTeacherNeedsNoReminderAndTheNextPeriodGetsItsOwn(t *testing.T) {
	t.Parallel()
	s := proTeacher(t, "month")
	s.endTeacherProIn(6)
	s.giveTeacherPro("ada@example.com", "month") // renewed: a second month waits after the first
	s.prune()
	if n := len(s.teacherEmails("ada@example.com")); n != 0 {
		t.Fatalf("a renewed Teacher got %d reminders, want none", n)
	}
	if _, err := s.db.Exec(`UPDATE teacher_pro_periods SET starts_at = starts_at - interval '1 month', ends_at = ends_at - interval '1 month'`); err != nil {
		t.Fatal(err)
	}
	s.prune() // the second month now has 6 days left
	if n := len(s.teacherEmails("ada@example.com")); n != 1 {
		t.Fatalf("the next period got %d reminders, want its own 1", n)
	}
}

// A monthly Teachers Portal Pro is reminded 7 days before it ends and once it has ended, and not 30 days before.
func TestAMonthOfTeachersPortalProIsRemindedAt7DaysAndOnceEndedOnly(t *testing.T) {
	t.Parallel()
	s := proTeacher(t, "month")

	s.endTeacherProIn(20)
	s.prune()
	if n := len(s.teacherEmails("ada@example.com")); n != 0 {
		t.Fatalf("20 days out: %d reminders, want none", n)
	}

	s.endTeacherProIn(6.5)
	s.prune()
	s.prune()
	got := s.teacherEmails("ada@example.com")
	if len(got) != 1 || !strings.Contains(got[0].Subject, "7 days") {
		t.Fatalf("7 days out: reminders = %+v, want the 7-day one", got)
	}

	s.endTeacherProIn(-0.5)
	s.prune()
	if got = s.teacherEmails("ada@example.com"); len(got) != 2 || !strings.Contains(got[1].Subject, "has ended") {
		t.Fatalf("after it ended: reminders = %+v, want a second, the ended one", got)
	}
}
