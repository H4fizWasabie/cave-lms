package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// chatSite is a Course "Go Basics" made by the Admin Abah, with Ada (IC set) and Ben joined; the returned site is
// logged in as Ada, admin as Abah and ben as Ben.
func chatSite(t *testing.T) (ada, admin, ben *site, id string) {
	t.Helper()
	admin = newSite(t)
	admin.loginAsNewAdmin("Abah", "admin@example.com", "correct horse battery")
	admin.createCourse("Go Basics", "Learn Go from scratch.", "")
	id = admin.onlyCourseID()
	ada = admin.second()
	ada.loginAsNewLearner("Ada", "ada@example.com", "correct horse battery")
	ada.setIC("900101-14-5678")
	expectStatus(t, must(ada.post("/courses/"+id+"/join", nil)), "", http.StatusSeeOther, "")
	ben = admin.second()
	ben.loginAsNewLearner("Ben", "ben@example.com", "correct horse battery")
	expectStatus(t, must(ben.post("/courses/"+id+"/join", nil)), "", http.StatusSeeOther, "")
	return ada, admin, ben, id
}

// must drops a body, for a call whose status is all that matters.
func must(res *http.Response, _ string) *http.Response { return res }

var messageAnchor = regexp.MustCompile(`#m(\d+)$`)

// say posts a Message to Course id's Chat and returns the new Message's ID, read from where the post redirects.
func (s *site) say(id, body string) string {
	s.t.Helper()
	res, page := s.post("/courses/"+id+"/chat", url.Values{"body": {body}})
	expectStatus(s.t, res, page, http.StatusSeeOther, "")
	m := messageAnchor.FindStringSubmatch(res.Header.Get("Location"))
	if m == nil {
		s.t.Fatalf("posting a Message redirected to %q, want the Chat at the Message", res.Header.Get("Location"))
	}
	return m[1]
}

func (s *site) chatCount(id string) int {
	s.t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM chat_messages WHERE course_id = $1`, id).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

func TestEnrolledLearnersAndTheAdminReadAndPostInACoursesChat(t *testing.T) {
	ada, admin, ben, id := chatSite(t)

	res, body := ada.get("/courses/" + id)
	expectStatus(t, res, body, http.StatusOK, `href="/courses/`+id+`/chat"`)
	mid := ada.say(id, "Where do I start?\nLesson 1?")
	res, _ = ada.post("/courses/"+id+"/chat", url.Values{"body": {"x"}})
	if loc := res.Header.Get("Location"); !strings.HasPrefix(loc, "/courses/"+id+"/chat") {
		t.Fatalf("post redirected to %q, want the Chat", loc)
	}
	admin.say(id, "Start with Variables.")

	res, body = ben.get("/courses/" + id + "/chat")
	expectStatus(t, res, body, http.StatusOK, `id="m`+mid+`"`)
	expectInOrder(t, body, "Ada", "Where do I start?\nLesson 1?", "Abah", "Admin", "Start with Variables.")
	if strings.Contains(body, "ada@example.com") || strings.Contains(body, "900101") {
		t.Fatalf("the Chat shows an email or IC: %s", body)
	}

	_, body = ben.getAsking("/courses/"+id+"/chat", "ms")
	expectLanguage(t, body, "ms", "Sembang")
}

func TestOthersAreRefusedACoursesChat(t *testing.T) {
	ada, admin, _, id := chatSite(t)
	cara := admin.second()
	cara.loginAsNewLearner("Cara", "cara@example.com", "correct horse battery")

	res, body := cara.get("/courses/" + id + "/chat")
	expectStatus(t, res, body, http.StatusForbidden, "")
	res, body = cara.post("/courses/"+id+"/chat", url.Values{"body": {"hi"}})
	expectStatus(t, res, body, http.StatusForbidden, "")
	if strings.Contains(body, `href="/courses/`+id+`/chat"`) {
		t.Fatalf("a Learner not on the Course sees the Chat tab")
	}

	visitor := admin.second()
	res, _ = visitor.get("/courses/" + id + "/chat")
	if loc := res.Header.Get("Location"); res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(loc, "/login") {
		t.Fatalf("Visitor got %d to %q, want the login page", res.StatusCode, loc)
	}

	// Leaving ends access, but the Learner's Messages stay with their name.
	ada.say(id, "Bye all")
	expectStatus(t, must(ada.post("/courses/"+id+"/leave", nil)), "", http.StatusSeeOther, "")
	res, body = ada.get("/courses/" + id + "/chat")
	expectStatus(t, res, body, http.StatusForbidden, "")
	_, body = admin.get("/courses/" + id + "/chat")
	expectInOrder(t, body, "Ada", "Bye all")
}

func TestALiveCourseHasItsOwnChat(t *testing.T) {
	ada, admin, _, id := chatSite(t)
	ada.say(id, "Only on the original")
	copyID := admin.copiedCourseID(id, "Go Basics for Acme", "2026-11-02", "2026-11-03", "2")
	_, body := admin.get("/courses/" + copyID + "/chat")
	if strings.Contains(body, "Only on the original") {
		t.Fatalf("a Live Course's Chat shows its original's Messages")
	}
}

func TestBadMessagesAreRefusedAndNotSaved(t *testing.T) {
	ada, _, _, id := chatSite(t)
	for _, c := range []struct{ body, want string }{
		{"  \n ", "Write a Message"},
		{strings.Repeat("a", 1001), "1,000 characters"},
	} {
		res, body := ada.post("/courses/"+id+"/chat", url.Values{"body": {c.body}})
		expectStatus(t, res, body, http.StatusUnprocessableEntity, c.want)
	}
	if n := ada.chatCount(id); n != 0 {
		t.Fatalf("%d Messages saved, want none", n)
	}
	ada.say(id, strings.Repeat("é", 1000))
}

func TestMessagesAreShownAsEscapedPlainText(t *testing.T) {
	ada, _, _, id := chatSite(t)
	ada.say(id, `<script>alert(1)</script> see https://evil.example/x`)
	_, body := ada.get("/courses/" + id + "/chat")
	if strings.Contains(body, "<script>alert") || strings.Contains(body, `href="https://evil.example`) {
		t.Fatalf("a Message is not escaped plain text: %s", body)
	}
	expectInOrder(t, body, "&lt;script&gt;alert(1)&lt;/script&gt; see https://evil.example/x")
}

func TestTheLatestMessagesShowOldestFirstWithEarlierOnesAPageBack(t *testing.T) {
	ada, admin, _, id := chatSite(t)
	for i := 1; i <= 55; i++ {
		admin.say(id, fmt.Sprintf("Note %02d.", i))
	}
	_, body := ada.get("/courses/" + id + "/chat")
	expectInOrder(t, body, "Note 06.", "Note 07.", "Note 55.")
	if strings.Contains(body, "Note 05.") {
		t.Fatalf("the Chat shows more than the latest 50 Messages")
	}
	m := regexp.MustCompile(`href="(/courses/` + id + `/chat\?before=\d+)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no Show earlier link: %s", body)
	}
	_, body = ada.get(m[1])
	expectInOrder(t, body, "Note 01.", "Note 05.")
	if strings.Contains(body, "Note 06.") || strings.Contains(body, `href="/courses/`+id+`/chat?before=`) {
		t.Fatalf("the earlier page shows later Messages or another earlier link: %s", body)
	}
}

func TestALearnerDeletesOnlyTheirOwnMessage(t *testing.T) {
	ada, _, ben, id := chatSite(t)
	mid := ada.say(id, "Oops, wrong Course")

	res, body := ben.post("/courses/"+id+"/chat/"+mid+"/delete", nil)
	expectStatus(t, res, body, http.StatusForbidden, "")

	res, body = ada.post("/courses/"+id+"/chat/"+mid+"/delete", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasSuffix(loc, "#m"+mid) {
		t.Fatalf("delete redirected to %q, want the Chat at the Message", loc)
	}
	_, body = ben.get("/courses/" + id + "/chat")
	expectInOrder(t, body, "Ada", "Message deleted")
	if strings.Contains(body, "Oops") {
		t.Fatalf("a deleted Message's text is still sent")
	}
}

func TestPostingIsLimitedToTenMessagesAMinuteForLearnersButNotTheAdmin(t *testing.T) {
	ada, admin, _, id := chatSite(t)
	for i := range 10 {
		ada.say(id, fmt.Sprintf("Message %d", i))
	}
	res, body := ada.post("/courses/"+id+"/chat", url.Values{"body": {"One too many"}})
	expectStatus(t, res, body, http.StatusTooManyRequests, "sending too fast")
	if n := ada.chatCount(id); n != 10 {
		t.Fatalf("%d Messages saved, want 10", n)
	}
	for i := range 11 {
		admin.say(id, fmt.Sprintf("Announcement %d", i))
	}
}

func TestDeletingALearnerDeletesTheirMessages(t *testing.T) {
	ada, _, _, id := chatSite(t)
	ada.say(id, "Hello")
	if _, err := ada.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	if n := ada.chatCount(id); n != 0 {
		t.Fatalf("%d Messages left after deleting their writer, want none", n)
	}
}

func TestThePrivacyNoticeTellsLearnersWhoSeesTheirChatMessages(t *testing.T) {
	s := newSite(t)
	_, body := s.getAsking("/privacy", "en")
	expectLanguage(t, body, "en", "Chat Messages")
	_, body = s.getAsking("/privacy", "ms")
	expectLanguage(t, body, "ms", "Mesej Sembang")
}

// reply posts a reply to Message mid in Course id's Chat and returns the reply's ID.
func (s *site) reply(id, mid, body string) string {
	s.t.Helper()
	res, page := s.post("/courses/"+id+"/chat", url.Values{"body": {body}, "reply_to": {mid}})
	expectStatus(s.t, res, page, http.StatusSeeOther, "")
	return messageAnchor.FindStringSubmatch(res.Header.Get("Location"))[1]
}

func TestAReplyQuotesItsOriginalAndLinksToIt(t *testing.T) {
	ada, _, ben, id := chatSite(t)
	mid := ada.say(id, "How long is the Exam?\nI have a meeting at 3.")

	_, body := ben.get("/courses/" + id + "/chat")
	expectInOrder(t, body, `href="/courses/`+id+`/chat?reply=`+mid+`#compose"`)
	_, body = ben.get("/courses/" + id + "/chat?reply=" + mid)
	expectInOrder(t, body, `id="compose"`, "How long is the Exam?", `name="reply_to" value="`+mid+`"`)

	rid := ben.reply(id, mid, "One hour.")
	_, body = ada.get("/courses/" + id + "/chat")
	_, reply, _ := strings.Cut(body, `id="m`+rid+`"`)
	expectInOrder(t, reply, `href="/courses/`+id+`/chat?before=`, `#m`+mid+`"`, "Ada", "How long is the Exam?", "One hour.")
	if strings.Contains(reply, "meeting at 3") {
		t.Fatalf("a reply quotes more than the original's first line: %s", reply)
	}
}

func TestAReplyToADeletedMessageShowsItRemoved(t *testing.T) {
	ada, _, ben, id := chatSite(t)
	mid := ada.say(id, "Secret plans")
	rid := ben.reply(id, mid, "Nice")
	ada.post("/courses/"+id+"/chat/"+mid+"/delete", nil)

	_, body := ben.get("/courses/" + id + "/chat")
	_, reply, _ := strings.Cut(body, `id="m`+rid+`"`)
	expectInOrder(t, reply, "Message removed", "Nice")
	if strings.Contains(body, "Secret plans") {
		t.Fatalf("a deleted original's text is still sent")
	}
	res, body := ben.post("/courses/"+id+"/chat", url.Values{"body": {"Again"}, "reply_to": {mid}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
}

func TestARepliesOriginalMustBeInTheSameCourse(t *testing.T) {
	ada, admin, _, id := chatSite(t)
	admin.createCourse("Rust Basics", "Learn Rust.", "")
	other := admin.courseIDByTitle("Rust Basics")
	mid := admin.say(other, "Rust only")
	res, body := ada.post("/courses/"+id+"/chat", url.Values{"body": {"Sneaky"}, "reply_to": {mid}})
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	if n := ada.chatCount(id); n != 0 {
		t.Fatalf("%d Messages saved, want none", n)
	}
}

func TestRepliesCountTowardTheMessageLimit(t *testing.T) {
	ada, _, ben, id := chatSite(t)
	mid := ben.say(id, "Question")
	for i := range 10 {
		ada.reply(id, mid, fmt.Sprintf("Answer %d", i))
	}
	res, body := ada.post("/courses/"+id+"/chat", url.Values{"body": {"More"}, "reply_to": {mid}})
	expectStatus(t, res, body, http.StatusTooManyRequests, "")
}

// The six Reactions and one that isn't, as escapes: the code holds no emoji.
const (
	thumbsUp = "\U0001F44D"
	heart    = "\u2764\uFE0F"
	laugh    = "\U0001F602"
	pray     = "\U0001F64F"
	clap     = "\U0001F44F"
	clown    = "\U0001F921"
)

// shown is how a Message shows the count of Reaction emoji, marked when the viewer's own.
func shown(emoji string, mine bool, n int) string {
	return fmt.Sprintf(`value="%s" aria-pressed="%t">%s %d<`, emoji, mine, emoji, n)
}

// react puts emoji on Message mid in Course id's Chat, or takes it off when it is already there.
func (s *site) react(id, mid, emoji string) (*http.Response, string) {
	return s.post("/courses/"+id+"/chat/"+mid+"/react", url.Values{"emoji": {emoji}})
}

// messageHTML is Message mid's part of Course id's Chat page.
func (s *site) messageHTML(id, mid string) string {
	s.t.Helper()
	_, body := s.get("/courses/" + id + "/chat")
	_, m, ok := strings.Cut(body, `id="m`+mid+`"`)
	if !ok {
		s.t.Fatalf("Message %s is not on the Chat page: %s", mid, body)
	}
	m, _, _ = strings.Cut(m, "</li>")
	return m
}

func (s *site) reactionCount() int {
	s.t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM chat_reactions`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

func TestEachPersonHasAtMostOneReactionPerMessage(t *testing.T) {
	ada, admin, ben, id := chatSite(t)
	mid := admin.say(id, "The Exam opens tomorrow.")

	res, body := ada.react(id, mid, thumbsUp)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasSuffix(loc, "#m"+mid) {
		t.Fatalf("reacting redirected to %q, want the Chat at the Message", loc)
	}
	ben.react(id, mid, thumbsUp)
	expectInOrder(t, ada.messageHTML(id, mid), shown(thumbsUp, true, 2))

	ada.react(id, mid, heart) // changes Ada's
	m := ada.messageHTML(id, mid)
	expectInOrder(t, m, shown(thumbsUp, false, 1), shown(heart, true, 1))

	ada.react(id, mid, heart) // takes it off
	m = ben.messageHTML(id, mid)
	expectInOrder(t, m, shown(thumbsUp, true, 1))
	if strings.Contains(m, heart+" 1") {
		t.Fatalf("a removed Reaction is still counted: %s", m)
	}
	if n := ada.reactionCount(); n != 1 {
		t.Fatalf("%d Reactions saved, want 1", n)
	}
}

func TestOnlyTheSixReactionsOnVisibleMessagesByChatMembersAreTaken(t *testing.T) {
	ada, admin, ben, id := chatSite(t)
	mid := ben.say(id, "Hello")
	res, body := ada.react(id, mid, clown)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")

	cara := admin.second()
	cara.loginAsNewLearner("Cara", "cara@example.com", "correct horse battery")
	res, body = cara.react(id, mid, thumbsUp)
	expectStatus(t, res, body, http.StatusForbidden, "")

	ben.post("/courses/"+id+"/chat/"+mid+"/delete", nil)
	res, body = ada.react(id, mid, thumbsUp)
	expectStatus(t, res, body, http.StatusUnprocessableEntity, "")
	if n := ada.reactionCount(); n != 0 {
		t.Fatalf("%d Reactions saved, want none", n)
	}
}

func TestReactingIsLimitedToThirtyAMinuteForLearnersButNotTheAdmin(t *testing.T) {
	ada, admin, _, id := chatSite(t)
	mid := admin.say(id, "React away")
	for i := range 30 {
		res, body := ada.react(id, mid, []string{thumbsUp, laugh}[i%2])
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}
	res, body := ada.react(id, mid, pray)
	expectStatus(t, res, body, http.StatusTooManyRequests, "")
	if m := ada.messageHTML(id, mid); strings.Contains(m, pray+" 1") {
		t.Fatalf("the Reaction over the limit was saved")
	}
	for i := range 31 {
		res, body := admin.react(id, mid, []string{thumbsUp, laugh}[i%2])
		expectStatus(t, res, body, http.StatusSeeOther, "")
	}
}

func TestDeletingALearnerDeletesTheirReactions(t *testing.T) {
	ada, admin, _, id := chatSite(t)
	mid := admin.say(id, "Hello")
	ada.react(id, mid, clap)
	if _, err := ada.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	if n := ada.reactionCount(); n != 0 {
		t.Fatalf("%d Reactions left after deleting their giver, want none", n)
	}
}

func (s *site) report(id, mid, reason string) (*http.Response, string) {
	return s.post("/courses/"+id+"/chat/"+mid+"/report", url.Values{"reason": {reason}})
}

func (s *site) openReports() int {
	s.t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM chat_reports WHERE closed_at IS NULL`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

func TestALearnerReportsAMessageOnceAndItStaysVisible(t *testing.T) {
	ada, admin, ben, id := chatSite(t)
	mid := ben.say(id, "Buy cheap watches")

	res, body := ada.report(id, mid, "Spam")
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if m := ada.messageHTML(id, mid); !strings.Contains(m, "Buy cheap watches") || !strings.Contains(m, "Reported") {
		t.Fatalf("a Reported Message is not shown as visible and reported: %s", m)
	}
	res, body = ada.report(id, mid, "Again")
	expectStatus(t, res, body, http.StatusConflict, "already reported")

	res, body = admin.get("/admin/reports")
	expectStatus(t, res, body, http.StatusOK, "")
	expectInOrder(t, body, `href="/admin/reports" aria-current="page"`, "Buy cheap watches", "Ben", "Go Basics", "Ada", "Spam")
	_, body = admin.getAsking("/admin/reports", "ms")
	expectLanguage(t, body, "ms", "Laporan")
}

func TestTheEleventhReportInAnHourIsRefused(t *testing.T) {
	ada, admin, _, id := chatSite(t)
	var mids []string
	for i := range 11 {
		mids = append(mids, admin.say(id, fmt.Sprintf("Note %d", i)))
	}
	for _, mid := range mids[:10] {
		expectStatus(t, must(ada.report(id, mid, "")), "", http.StatusSeeOther, "")
	}
	res, body := ada.report(id, mids[10], "")
	expectStatus(t, res, body, http.StatusTooManyRequests, "")
	if n := ada.openReports(); n != 10 {
		t.Fatalf("%d Reports saved, want 10", n)
	}
}

func TestHidingAMessageRemovesItForLearnersAndClosesItsReports(t *testing.T) {
	ada, admin, ben, id := chatSite(t)
	mid := ben.say(id, "Something nasty")
	rid := ada.reply(id, mid, "Not nice")
	ada.report(id, mid, "Rude")
	cara := admin.second()
	cara.loginAsNewLearner("Cara", "cara@example.com", "correct horse battery")
	cara.post("/courses/"+id+"/join", nil)
	cara.report(id, mid, "")

	res, body := admin.post("/admin/chat/"+mid+"/hide", url.Values{"back": {"reports"}})
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); loc != "/admin/reports" {
		t.Fatalf("hiding from the Reports page went to %q", loc)
	}
	if n := ada.openReports(); n != 0 {
		t.Fatalf("%d Reports still open after hiding their Message", n)
	}
	_, body = ada.get("/courses/" + id + "/chat")
	if strings.Contains(body, "Something nasty") {
		t.Fatalf("a hidden Message's text reaches a Learner")
	}
	expectInOrder(t, ada.messageHTML(id, mid), "Message removed")
	expectInOrder(t, ada.messageHTML(id, rid), "Message removed", "Not nice")

	m := admin.messageHTML(id, mid)
	expectInOrder(t, m, "Something nasty", `action="/admin/chat/`+mid+`/unhide"`)
	if !strings.Contains(adminLi(admin, id, mid), "is-hidden") {
		t.Fatalf("the Admin does not see the hidden Message greyed out")
	}

	res, body = admin.post("/admin/chat/"+mid+"/unhide", nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if loc := res.Header.Get("Location"); !strings.HasSuffix(loc, "#m"+mid) {
		t.Fatalf("unhiding from the Chat went to %q, want the Chat at the Message", loc)
	}
	expectInOrder(t, ada.messageHTML(id, mid), "Something nasty")
}

// adminLi is the opening tag of Message mid's item on the Chat page as s sees it.
func adminLi(s *site, id, mid string) string {
	_, body := s.get("/courses/" + id + "/chat")
	i := strings.Index(body, `id="m`+mid+`"`)
	return body[strings.LastIndex(body[:i], "<li"):i]
}

func TestDismissingAReportClosesOnlyThatOne(t *testing.T) {
	ada, admin, ben, id := chatSite(t)
	mid := ben.say(id, "Edgy joke")
	ada.report(id, mid, "Not funny")
	_, body := admin.get("/admin/reports")
	m := regexp.MustCompile(`action="(/admin/reports/\d+/dismiss)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no Dismiss on the Reports page: %s", body)
	}
	res, body := admin.post(m[1], nil)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	if n := ada.openReports(); n != 0 {
		t.Fatalf("%d Reports open after dismissing, want none", n)
	}
	expectInOrder(t, ada.messageHTML(id, mid), "Edgy joke")
}

func TestLearnersCannotHideOrSeeReports(t *testing.T) {
	ada, _, ben, id := chatSite(t)
	mid := ben.say(id, "Hi")
	res, body := ada.post("/admin/chat/"+mid+"/hide", nil)
	expectStatus(t, res, body, http.StatusForbidden, "")
	if strings.Contains(ada.messageHTML(id, mid), "/hide") {
		t.Fatalf("a Learner is offered Hide")
	}
}

func TestDeletingALearnerDeletesTheirReports(t *testing.T) {
	ada, _, ben, id := chatSite(t)
	mid := ben.say(id, "Hi")
	ada.report(id, mid, "")
	if _, err := ada.db.Exec(`DELETE FROM learners WHERE email = 'ada@example.com'`); err != nil {
		t.Fatal(err)
	}
	if n := ada.openReports(); n != 0 {
		t.Fatalf("%d Reports left after deleting their maker, want none", n)
	}
}

func TestTheAdminAndWritersCannotReportAndARepeatReportCostsNothing(t *testing.T) {
	ada, admin, ben, id := chatSite(t)
	mid := ben.say(id, "Hi")
	expectStatus(t, must(admin.report(id, mid, "")), "", http.StatusUnprocessableEntity, "")
	expectStatus(t, must(ben.report(id, mid, "")), "", http.StatusUnprocessableEntity, "")
	ada.report(id, mid, "")
	for range 12 {
		expectStatus(t, must(ada.report(id, mid, "")), "", http.StatusConflict, "")
	}
	other := admin.say(id, "Another")
	expectStatus(t, must(ada.report(id, other, "")), "", http.StatusSeeOther, "")

	// A Message its writer deletes leaves no Report open.
	ben.post("/courses/"+id+"/chat/"+mid+"/delete", nil)
	if n := ada.openReports(); n != 1 {
		t.Fatalf("%d Reports open, want only the one on the Message still there", n)
	}
}
