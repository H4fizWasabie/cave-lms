package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// communitySite is chatSite with Ada joined to the Community Chat and Ben not.
func communitySite(t *testing.T) (ada, admin, ben *site, id string) {
	t.Helper()
	ada, admin, ben, id = chatSite(t)
	expectStatus(t, must(ada.post("/community/join", nil)), "", http.StatusSeeOther, "")
	return ada, admin, ben, id
}

func TestAVisitorIsToldWhatTheCommunityChatIsAndAskedToLogIn(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	res, body := s.getAsking("/community", "en")
	expectStatus(t, res, body, http.StatusOK, "Community Chat")
	expectInOrder(t, content(body), `href="/login?next=%2Fcommunity"`)
	expectNone(t, content(body), `action="/community/join"`)
	_, body = s.getAsking("/community", "ms")
	expectLanguage(t, body, "ms", "Sembang Komuniti")
}

func TestALearnerWhoHasNotJoinedSeesJoinButNoMessagesAndIsRefusedTheChat(t *testing.T) {
	t.Parallel()
	ada, admin, ben, _ := communitySite(t)
	mid := ada.say(community, "Members only talk")

	res, body := ben.get("/community")
	expectStatus(t, res, body, http.StatusOK, `action="/community/join"`)
	expectInOrder(t, body, "never your email address or IC")
	expectNone(t, body, "Members only talk", `data-stream=`)

	for _, path := range []string{"/community", "/community/" + mid + "/react", "/community/" + mid + "/report", "/community/" + mid + "/delete"} {
		res, body := ben.post(path, url.Values{"body": {"hi"}, "emoji": {thumbsUp}})
		expectStatus(t, res, body, http.StatusForbidden, "")
	}
	res, body = ben.get("/community/events")
	expectStatus(t, res, body, http.StatusForbidden, "")
	if res, _ := admin.second().get("/community/events"); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("a Visitor's Community stream got %d, want sent to log in", res.StatusCode)
	}
}

func TestALearnerJoinsLeavesAndJoinsTheCommunityChatAgain(t *testing.T) {
	t.Parallel()
	ada, admin, _, _ := communitySite(t)
	res, body := ada.get("/community")
	expectStatus(t, res, body, http.StatusOK, `data-stream="/community/events"`)
	expectInOrder(t, body, `href="/community/leave"`)
	ada.say(community, "Bye all")

	res, body = ada.post("/community/leave", nil)
	expectRedirect(t, res, body, "/community/leave")
	_, body = ada.get("/community")
	expectInOrder(t, body, "Bye all")
	res, body = ada.get("/community/leave")
	expectStatus(t, res, body, http.StatusOK, "Leave the Community Chat? Your Messages stay. You can join again at any time.")
	expectAll(t, body, `href="/community"`)

	res, body = ada.post("/community/leave", confirming)
	expectStatus(t, res, body, http.StatusSeeOther, "")
	_, body = ada.get("/community")
	expectInOrder(t, body, `action="/community/join"`)
	expectNone(t, body, "Bye all")
	expectInOrder(t, admin.messageHTML(community, ada.lastMessageID()), "Bye all")

	expectStatus(t, must(ada.post("/community/join", nil)), "", http.StatusSeeOther, "")
	expectStatus(t, must(ada.post("/community/join", nil)), "", http.StatusSeeOther, "") // twice is harmless
	_, body = ada.get("/community")
	expectInOrder(t, body, "Bye all")
}

func TestAMemberPostsRepliesReactsDeletesAndReportsInTheCommunityChat(t *testing.T) {
	t.Parallel()
	ada, admin, ben, id := communitySite(t)
	expectStatus(t, must(ben.post("/community/join", nil)), "", http.StatusSeeOther, "")
	mid := ben.say(community, "Hello from Go Basics")
	rid := ada.reply(community, mid, "Hi Ben")
	expectInOrder(t, ada.messageHTML(community, rid), `href="/community?before=`, "Ben", "Hello from Go Basics", "Hi Ben")

	expectStatus(t, must(ada.react(community, mid, laugh)), "", http.StatusSeeOther, "")
	expectInOrder(t, ben.messageHTML(community, mid), shown(laugh, false, 1))

	gone := ada.say(community, "Oops")
	expectStatus(t, must(ben.post("/community/"+gone+"/delete", nil)), "", http.StatusForbidden, "")
	res, body := ada.post("/community/"+gone+"/delete", nil)
	expectRedirect(t, res, body, "/community/"+gone+"/delete")
	expectInOrder(t, ben.messageHTML(community, gone), "Oops")
	res, body = ada.get("/community/" + gone + "/delete")
	expectStatus(t, res, body, http.StatusOK, "Delete your Message?")
	expectAll(t, body, `href="/community#m`+gone+`"`)
	expectStatus(t, must(ada.post("/community/"+gone+"/delete", confirming)), "", http.StatusSeeOther, "")
	expectInOrder(t, ben.messageHTML(community, gone), "Message deleted")

	expectStatus(t, must(ada.report(community, mid, "Spam")), "", http.StatusSeeOther, "")
	_, body = admin.get("/admin/reports")
	expectInOrder(t, body, `href="/community?before=`, "Hello from Go Basics", "Ben", "Community Chat", "Ada", "Spam")
	_, body = admin.getAsking("/admin/reports", "ms")
	expectInOrder(t, body, "Sembang Komuniti")

	// The Course's Chat is apart: neither shows the other's Messages.
	_, body = ada.get(chatURL(id))
	expectNone(t, body, "Hello from Go Basics")
}

func TestTheAdminReadsPostsAndHidesInTheCommunityChatWithoutJoining(t *testing.T) {
	t.Parallel()
	ada, admin, _, _ := communitySite(t)
	mid := ada.say(community, "Something nasty")
	res, body := admin.get("/community")
	expectStatus(t, res, body, http.StatusOK, "Something nasty")
	expectNone(t, body, `href="/community/leave"`)
	aid := admin.say(community, "Welcome all")
	expectInOrder(t, ada.messageHTML(community, aid), "Abah", "Admin", "Welcome all")

	expectStatus(t, must(admin.post("/admin/chat/"+mid+"/hide", confirming)), "", http.StatusSeeOther, "")
	expectInOrder(t, ada.messageHTML(community, mid), "Message removed")
	res, _ = admin.post("/admin/chat/"+mid+"/unhide", nil)
	if loc := res.Header.Get("Location"); loc != "/community#m"+mid {
		t.Fatalf("unhiding went to %q, want the Community Chat at the Message", loc)
	}
	expectInOrder(t, ada.messageHTML(community, mid), "Something nasty")
}

func TestCommunityPostingIsLimitedPerChat(t *testing.T) {
	t.Parallel()
	ada, _, _, id := communitySite(t)
	for i := range 10 {
		ada.say(community, fmt.Sprintf("Message %d", i))
	}
	res, body := ada.post("/community", url.Values{"body": {"One too many"}})
	expectStatus(t, res, body, http.StatusTooManyRequests, "sending too fast")
	ada.say(id, "The Course's Chat has its own count")
}

func TestTheCommunityStreamCarriesMessagesWithoutEmailOrIC(t *testing.T) {
	t.Parallel()
	ada, admin, _, _ := communitySite(t)
	stream := ada.openStream(community)
	mid := ada.say(community, "Live hello")
	e := stream.next()
	expectInOrder(t, e, `id="m`+mid+`"`, "Ada", "Live hello")
	_, body := admin.get("/community")
	for _, page := range []string{e, body} {
		if strings.Contains(page, "ada@example.com") || strings.Contains(page, "900101") {
			t.Fatalf("the Community Chat shows an email or IC: %s", page)
		}
	}
}

func TestTheHeaderLinksToTheCommunityChat(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.getAsking("/", "en")
	expectInOrder(t, header(t, body), ">Certification<", `href="/community">Community<`)
	_, body = s.getAsking("/", "ms")
	expectInOrder(t, header(t, body), ">Pensijilan<", `href="/community">Komuniti<`)
}

func TestThePrivacyNoticeTellsWhoSeesCommunityChatMessages(t *testing.T) {
	t.Parallel()
	s := newSite(t)
	_, body := s.getAsking("/privacy", "en")
	expectInOrder(t, body, "The Community Chat, if you join it")
	_, body = s.getAsking("/privacy", "ms")
	expectInOrder(t, body, "Sembang Komuniti, jika anda menyertainya")
}

// lastMessageID is the ID of the newest Chat Message.
func (s *site) lastMessageID() string {
	s.t.Helper()
	var id string
	if err := s.db.QueryRow(`SELECT max(id)::text FROM chat_messages`).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}
