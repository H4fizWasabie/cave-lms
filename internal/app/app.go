package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/dataset"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/migrations"
	"github.com/H4fizWasabie/cave-lms/internal/storage"
	"github.com/H4fizWasabie/cave-lms/internal/text"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed templates/*.html
var appFiles embed.FS

//go:embed static
var staticFiles embed.FS

// Postgres allows 100 connections shared with the other apps on the VPS; unlimited would let a burst take them all.
const (
	maxDBConns     = 25
	dbConnLifetime = 30 * time.Minute
)

type App struct {
	labReads             atomic.Int64 // how many times Lab access was read from the database, for tests
	db                   *sql.DB
	handler              http.Handler
	mailer               mailer.Mailer
	writer               aiwriter.Writer // writes Exam questions and Lesson text and marks Written answers
	pages                map[string]*template.Template
	publicBaseURL        string
	store                *storage.Disk
	dataset              *dataset.Dataset
	datasets             storage.Store // where Dataset data and Submission files live (ADR 0008)
	maxUpload            int64
	limiter              rateLimiter
	chatHub              chatHub       // tells open Chat streams of new and changed Messages
	ownerEmail           string        // the Owner's, from OWNER_EMAIL; empty means no Owner
	caveTextExtensionIDs []string      // the extensions the CaveText connect page may send tokens to
	bclWebhookKey        string        // the secret in BCL's webhook address; empty refuses every webhook
	bclTestPayments      bool          // count BCL Test Mode orders and ignore live ones (staging)
	bclAPIToken          string        // a BCL API Read token to confirm orders with; empty confirms none
	bclAPIURL            string        // BCL's API, without a trailing slash
	production           bool          // the live site: hides the home page's placeholders (CAVE_ENV)
	done                 chan struct{} // closed by Close, to stop the daily System log prune and the Result release
	// releasing is held by a run of releaseResults, so two runs never release at once.
	// ponytail: an in-process lock, right for one process only; take a Postgres advisory lock instead if the app
	// ever runs more than one instance.
	releasing sync.Mutex
	// uploading holds the IDs of Learners with a Submission upload in flight, so each sends one at a time and holds
	// at most one request's files (5 x 20 MB) in memory.
	// ponytail: per process; many Learners at once still each hold up to 100 MB. Streaming files to storage instead of
	// reading each whole in checkSubmissionFile would remove the ceiling.
	uploading sync.Map
	// cleaning holds a turn for each Clean-up running, so at most maxCleanUps uploads are read at once; others wait.
	// ponytail: per process, right while the app runs as one; reading in the Learner's browser lifts the ceiling.
	cleaning       chan struct{}
	cleanUpWaitLog time.Duration    // a wait for a turn longer than this is written to the System log
	now            func() time.Time // the clock; tests set it
	routes         []string         // every registered pattern, so the admin-route test can walk them
}

// page is the data the pre-Lab templates receive. Newer pages get their own struct embedding pageBase.
type page struct {
	chrome
	PublicBaseURL string
	Learner       *learner
	Title         string
	Message       string
	Error         string
	ErrorField    string // the id of the form field Error is about, shown next to it; "" puts it above the form
	Name          string
	Username      string
	Email         string
	OfferResend   bool
	OfferLogin    bool
	OfferHome     bool // the error page: Go to the home page, and Try again at Retry when it is set; render sets it on a 403 or 404
	Retry         string
	BackToCourse  string // a 403 or 404 inside a Course: its page, offered before Go to the home page
	Items         []galleryItem
	Item          *galleryItem
	Format        string // Gallery filters
	Category      string
	Categories    []string
	OfferReset    bool
	Token         string
	Courses       []course
	Course        *course
	Exam          *exam
	Exams         []exam      // the Course's usable Exams, Multiple-choice first, on its page
	Step          *nextStep   // the enrolled Learner's next step, on the Course page and a done Lesson's page
	Status        *examStatus // the Learner's Exam
	Attempt       *examAttempt
	Question      *examQuestion
	Enrolled      bool
	Done          int // Lessons the Learner has completed
	Total         int // published Lessons in the Course
	Lessons       []lesson
	Lesson        *lesson
	Place         *lessonPlace // where the Lesson stands in its Course, on its page
	Assignments   []assignment // the Course's, on its page for an enrolled Learner or the Admin
	Enrollees     []enrollee
	Certificates  []certificate
	Certificate   *certificate // the one a verification page shows, or a passed Result leads to
	NeedsIC       bool         // the Learner has no IC yet, so no Certificate can be issued to them
	PassedNoIC    bool         // NeedsIC, and the Learner already passed the Course's Exam: only the Certificate waits
	Query         string       // what the Admin's Certificate search or the Course list's search was asked for
	Settings      *certificateSettings
	Next          string         // where the login page goes after logging in
	DuplicateOf   int64          // the Course the Course form duplicates into a Live Course, 0 when none
	AILanguage    string         // the language the Lesson form's Generate writes in
	Organisations []organisation // the Live Course form's choices
}

func (p *page) learnerField() **learner { return &p.Learner }

// pageBase holds what layout.html reads. A page struct embeds it
// and is passed to render as a pointer, so render can fill in the Learner.
type pageBase struct {
	chrome
	Learner *learner
	Title   string
	Message string
	Error   string
}

func (p *pageBase) learnerField() **learner { return &p.Learner }

func New(ctx context.Context, cfg config.Config, mailer mailer.Mailer, writer aiwriter.Writer) (_ *App, err error) {
	if mailer == nil || writer == nil {
		return nil, fmt.Errorf("mailer and AI writer are required")
	}
	pages, err := sitePages()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(maxDBConns)
	db.SetMaxIdleConns(maxDBConns)
	db.SetConnMaxLifetime(dbConnLifetime)
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()
	if err = db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	store, err := storage.Open(cfg.StorageDir)
	if err != nil {
		return nil, err
	}
	var datasets storage.Store = store
	if cfg.R2Bucket != "" {
		datasets = storage.NewR2(cfg.R2AccountID, cfg.R2Bucket, cfg.R2AccessKeyID, cfg.R2SecretAccessKey)
	}
	if err = migrations.Up(ctx, db); err != nil {
		return nil, err
	}

	a := &App{db: db, mailer: mailer, writer: writer, pages: pages, publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"), store: store, datasets: datasets, maxUpload: cfg.MaxUploadBytes, ownerEmail: cfg.OwnerEmail, caveTextExtensionIDs: cfg.CaveTextExtensionIDs, bclWebhookKey: cfg.BCLWebhookKey, bclTestPayments: cfg.BCLTestPayments, bclAPIToken: cfg.BCLAPIToken, bclAPIURL: cfg.BCLAPIURL, production: cfg.Production, done: make(chan struct{}), now: time.Now,
		cleaning: make(chan struct{}, maxCleanUps), cleanUpWaitLog: cleanUpWaitLog}
	a.dataset = dataset.New(db, datasets, func() time.Time { return a.now() })
	if len(cfg.EmailAllowlist) > 0 {
		a.mailer = allowlistMailer{next: mailer, allowed: cfg.EmailAllowlist, hold: a.logInfo}
	}
	a.dailyPrune(ctx)
	a.checkBCLToken(ctx)
	go a.pruneDaily(a.done)
	go a.releaseEveryMinute(a.done)
	mux := routeMux{http.NewServeMux(), &a.routes}
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("/", a.notFound) // any other address
	mux.Handle("GET /static/", staticHandler())
	mux.HandleFunc("GET /privacy", a.privacy)
	mux.HandleFunc("GET /about", a.about)
	mux.HandleFunc("GET /register", a.registerForm)
	mux.HandleFunc("POST /register", a.limited(sharedAddressAttempts, a.register))
	mux.HandleFunc("GET /verify", a.verify)
	mux.HandleFunc("POST /verify/resend", a.limited(sharedAddressAttempts, a.resendVerification))
	mux.HandleFunc("GET /login", a.loginForm)
	mux.HandleFunc("POST /login", a.login) // limits its own failures, see login
	mux.HandleFunc("GET /forgot-password", a.forgotPasswordForm)
	mux.HandleFunc("POST /forgot-password", a.limited(sharedAddressAttempts, a.forgotPassword))
	mux.HandleFunc("GET /reset-password", a.resetPasswordForm)
	mux.HandleFunc("POST /reset-password", a.resetPassword) // limits its own bad links, see resetPassword
	mux.HandleFunc("POST /logout", a.logout)
	mux.HandleFunc("POST /language", a.switchLanguage)
	mux.HandleFunc("GET /courses", a.courses)
	mux.HandleFunc("GET /courses/{id}", a.showCourse)
	mux.HandleFunc("POST /courses/{id}/join", a.requireLearner(a.joinCourse))
	mux.HandleFunc("GET /courses/{id}/leave", a.requireLearner(a.confirmLeaveCourse))
	mux.HandleFunc("POST /courses/{id}/leave", a.requireLearner(a.leaveCourse))
	mux.HandleFunc("GET /my-courses", a.requireLearner(a.myCourses))
	mux.HandleFunc("GET /courses/{id}/chat", a.requireLearner(a.chat))
	mux.HandleFunc("POST /courses/{id}/chat", a.requireLearner(a.postMessage))
	mux.HandleFunc("GET /courses/{id}/chat/{mid}/delete", a.requireLearner(a.confirmDeleteMessage))
	mux.HandleFunc("POST /courses/{id}/chat/{mid}/delete", a.requireLearner(a.deleteMessage))
	mux.HandleFunc("GET /courses/{id}/chat/events", a.requireLearner(a.chatEvents))
	mux.HandleFunc("POST /courses/{id}/chat/{mid}/react", a.requireLearner(a.react))
	mux.HandleFunc("POST /courses/{id}/chat/{mid}/report", a.requireLearner(a.reportMessage))
	mux.HandleFunc("GET /community", a.community)
	mux.HandleFunc("POST /community", a.requireLearner(a.postMessage))
	mux.HandleFunc("POST /community/join", a.requireLearner(a.setMember(true)))
	mux.HandleFunc("GET /community/leave", a.requireLearner(a.confirmLeaveCommunity))
	mux.HandleFunc("POST /community/leave", a.requireLearner(a.setMember(false)))
	mux.HandleFunc("GET /community/{mid}/delete", a.requireLearner(a.confirmDeleteMessage))
	mux.HandleFunc("POST /community/{mid}/delete", a.requireLearner(a.deleteMessage))
	mux.HandleFunc("GET /community/events", a.requireLearner(a.chatEvents))
	mux.HandleFunc("POST /community/{mid}/react", a.requireLearner(a.react))
	mux.HandleFunc("POST /community/{mid}/report", a.requireLearner(a.reportMessage))
	mux.HandleFunc("GET /admin/chat/{mid}/hide", a.admin(a.confirmHide))
	mux.HandleFunc("POST /admin/chat/{mid}/hide", a.admin(a.setHidden(true)))
	mux.HandleFunc("POST /admin/chat/{mid}/unhide", a.admin(a.setHidden(false)))
	mux.HandleFunc("GET /admin/reports", a.admin(a.adminReports))
	mux.HandleFunc("POST /admin/reports/{id}/dismiss", a.admin(a.dismissReport))
	mux.HandleFunc("GET /lab", a.lab)
	// The public demo and the Samples are gone (ADR 0019); old links land on the Plans.
	toPricing := func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/pricing", http.StatusSeeOther) }
	mux.HandleFunc("GET /lab/demo/", toPricing)
	mux.HandleFunc("GET /lab/samples/", toPricing)
	// Every Lab screen is a.labScreen: a GET without the Lab goes to /pricing. Every change answers 403 (a.labOpen).
	mux.HandleFunc("GET /lab/new", a.labScreen(a.newDashboardForm))
	mux.HandleFunc("POST /lab/dashboards", a.requireLearner(a.createDashboard))
	mux.HandleFunc("POST /lab/dashboards/build", a.requireLearner(a.buildDashboard))
	mux.HandleFunc("GET /lab/dashboards/{id}", a.labScreen(a.dashboardTab("")))
	mux.HandleFunc("GET /lab/dashboards/{id}/preview", a.labScreen(a.dashboardTab("preview")))
	mux.HandleFunc("GET /lab/dashboards/{id}/data", a.labScreen(a.dashboardTab("data")))
	mux.HandleFunc("POST /lab/dashboards/{id}/dataset", a.withDashboard(a.uploadDataset))
	mux.HandleFunc("POST /lab/dashboards/{id}/page", a.withDashboard(a.uploadPage))
	mux.HandleFunc("POST /lab/dashboards/{id}/pages/{pg}/restore", a.withDashboard(a.restorePage))
	mux.HandleFunc("POST /lab/dashboards/{id}/dataset/confirm", a.withDashboard(a.confirmWork))
	mux.HandleFunc("GET /lab/dashboards/{id}/parts/{part}/delete", a.labScreen(a.withDashboard(a.confirmDeletePart)))
	mux.HandleFunc("POST /lab/dashboards/{id}/parts/{part}/delete", a.withDashboard(a.deletePart))
	mux.HandleFunc("GET /lab/dashboards/{id}/parts/{part}/replace", a.labScreen(a.withDashboard(a.replacePartForm)))
	mux.HandleFunc("POST /lab/dashboards/{id}/parts/{part}/replace", a.withDashboard(a.uploadPartReplacement))
	mux.HandleFunc("POST /lab/dashboards/{id}/theme", a.withDashboard(a.saveTheme))
	mux.HandleFunc("GET /lab/dashboards/{id}/view/{$}", a.labScreen(a.dashboardFile("index.html")))
	mux.HandleFunc("GET /lab/dashboards/{id}/view/data/{key}", a.labScreen(a.dashboardFile("data")))
	mux.HandleFunc("GET /lab/dashboards/{id}/view/charts.js", a.labScreen(a.dashboardFile("charts.js")))
	mux.HandleFunc("GET /lab/dashboards/{id}/deploy", a.labScreen(a.withDashboard(a.deployTab)))
	mux.HandleFunc("POST /lab/dashboards/{id}/deploy", a.withDashboard(a.deployDashboard))
	mux.HandleFunc("GET /lab/dashboards/{id}/rename", a.labScreen(a.dashboardTab("rename")))
	mux.HandleFunc("POST /lab/dashboards/{id}/rename", a.withDashboard(a.renameDashboard))
	mux.HandleFunc("GET /lab/dashboards/{id}/delete", a.labScreen(a.withDashboard(a.confirmDeleteDashboard)))
	mux.HandleFunc("POST /lab/dashboards/{id}/delete", a.withDashboard(a.deleteDashboard))
	mux.HandleFunc("GET /"+deploymentPrefix+"/{rest...}", a.deploymentRoute)
	mux.HandleFunc("GET /admin", a.admin(a.adminHome))
	mux.HandleFunc("GET /admin/learners", a.admin(a.adminLearners))
	mux.HandleFunc("GET /admin/gallery/new", a.admin(a.galleryUploadForm))
	mux.HandleFunc("POST /admin/gallery", a.admin(a.galleryUpload))
	mux.HandleFunc("GET /admin/gallery", a.admin(a.adminGallery))
	mux.HandleFunc("GET /admin/gallery/{id}/edit", a.admin(a.editGalleryForm))
	mux.HandleFunc("POST /admin/gallery/{id}", a.admin(a.updateGalleryItem))
	mux.HandleFunc("GET /admin/gallery/{id}/delete", a.admin(a.confirmDeleteGalleryItem))
	mux.HandleFunc("POST /admin/gallery/{id}/delete", a.admin(a.deleteGalleryItem))
	mux.HandleFunc("GET /gallery", a.gallery)
	mux.HandleFunc("GET /gallery/{id}", a.galleryItemPage)
	mux.HandleFunc("GET /gallery/{id}/file", a.galleryFile)
	mux.HandleFunc("GET /gallery/{id}/thumb", a.galleryThumb)
	mux.HandleFunc("GET /programmes", a.programmes)
	mux.HandleFunc("GET /programmes/pictures/{key}", a.programmePicture)
	mux.HandleFunc("GET /course-pictures/{key}", a.coursePicture)
	mux.HandleFunc("GET /certificate-background/{key}", a.certificateBackground)
	mux.HandleFunc("GET /admin/programmes", a.admin(a.adminProgrammes))
	mux.HandleFunc("GET /admin/programmes/new", a.admin(a.newProgrammeForm))
	mux.HandleFunc("POST /admin/programmes", a.admin(a.createProgramme))
	mux.HandleFunc("GET /admin/programmes/{id}/edit", a.admin(a.editProgrammeForm))
	mux.HandleFunc("POST /admin/programmes/{id}", a.admin(a.updateProgramme))
	mux.HandleFunc("GET /admin/programmes/{id}/delete", a.admin(a.confirmDeleteProgramme))
	mux.HandleFunc("POST /admin/programmes/{id}/delete", a.admin(a.deleteProgramme))
	mux.HandleFunc("GET /admin/organisations", a.admin(a.adminOrganisations))
	mux.HandleFunc("GET /admin/organisations/new", a.admin(a.newOrganisationForm))
	mux.HandleFunc("POST /admin/organisations", a.admin(a.createOrganisation))
	mux.HandleFunc("GET /admin/organisations/{id}/edit", a.admin(a.editOrganisationForm))
	mux.HandleFunc("GET /admin/organisations/{id}/logo", a.admin(a.organisationLogo))
	mux.HandleFunc("POST /admin/organisations/{id}", a.admin(a.updateOrganisation))
	mux.HandleFunc("GET /admin/organisations/{id}/delete", a.admin(a.confirmDeleteOrganisation))
	mux.HandleFunc("POST /admin/organisations/{id}/delete", a.admin(a.deleteOrganisation))
	mux.HandleFunc("GET /admin/courses", a.admin(a.adminCourses))
	mux.HandleFunc("GET /admin/courses/new", a.admin(a.newCourseForm))
	mux.HandleFunc("POST /admin/courses", a.admin(a.createCourse))
	mux.HandleFunc("GET /admin/courses/{id}", a.admin(a.adminCourse))
	mux.HandleFunc("GET /admin/courses/{id}/learners/{learnerID}/remove", a.admin(a.confirmRemoveLearner))
	mux.HandleFunc("POST /admin/courses/{id}/learners/{learnerID}/remove", a.admin(a.removeLearner))
	mux.HandleFunc("POST /admin/courses/{id}/learners/{learnerID}/ic", a.admin(a.correctIC))
	mux.HandleFunc("POST /admin/courses/{id}/learners/{learnerID}/certificate", a.admin(a.giveCertificate))
	mux.HandleFunc("GET /admin/courses/{id}/edit", a.admin(a.editCourseForm))
	mux.HandleFunc("POST /admin/courses/{id}", a.admin(a.updateCourse))
	mux.HandleFunc("POST /admin/courses/{id}/open", a.admin(a.setCourseOpen(true)))
	mux.HandleFunc("POST /admin/courses/{id}/close", a.admin(a.setCourseOpen(false)))
	mux.HandleFunc("POST /admin/courses/{id}/publish", a.admin(a.publishCourse))
	mux.HandleFunc("GET /admin/courses/{id}/duplicate", a.admin(a.duplicateCourseForm))
	mux.HandleFunc("POST /admin/courses/{id}/duplicate", a.admin(a.duplicateCourse))
	mux.HandleFunc("GET /courses/{id}/lessons/{lessonID}", a.requireLearner(a.showLesson))
	mux.HandleFunc("GET /lesson-images/{key}", a.requireLearner(a.lessonImage))
	mux.HandleFunc("POST /courses/{id}/lessons/{lessonID}/complete", a.requireLearner(a.setLessonComplete(true)))
	mux.HandleFunc("POST /courses/{id}/lessons/{lessonID}/incomplete", a.requireLearner(a.setLessonComplete(false)))
	mux.HandleFunc("GET /admin/courses/{id}/lessons", a.admin(a.adminLessons))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/up", a.admin(a.moveLesson(-1)))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/down", a.admin(a.moveLesson(1)))
	mux.HandleFunc("GET /admin/lessons/{lessonID}/edit", a.admin(a.editLessonForm))
	mux.HandleFunc("POST /admin/lessons/{lessonID}", a.admin(a.updateLesson))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/generate", a.admin(a.generateLessonText))
	mux.HandleFunc("GET /admin/lessons/{lessonID}/delete", a.admin(a.confirmDeleteLesson))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/delete", a.admin(a.deleteLesson))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/publish", a.admin(a.setLessonPublished(true)))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/unpublish", a.admin(a.setLessonPublished(false)))
	mux.HandleFunc("POST /admin/courses/{id}/lessons/publish-all", a.admin(a.publishAllLessons))
	mux.HandleFunc("GET /admin/courses/{id}/lessons/new", a.admin(a.newLessonForm))
	mux.HandleFunc("POST /admin/courses/{id}/lessons", a.admin(a.createLesson))
	mux.HandleFunc("POST /admin/courses/{id}/lessons/generate", a.admin(a.generateNewLesson))
	mux.HandleFunc("GET /admin/courses/{id}/assignments", a.admin(a.adminAssignments))
	mux.HandleFunc("GET /admin/courses/{id}/assignments/new", a.admin(a.newAssignmentForm))
	mux.HandleFunc("POST /admin/courses/{id}/assignments", a.admin(a.createAssignment))
	mux.HandleFunc("GET /admin/assignments/{aid}", a.admin(a.adminSubmissions))
	mux.HandleFunc("POST /admin/assignments/{aid}/learners/{learnerID}/comment", a.admin(a.saveSubmissionComment))
	mux.HandleFunc("GET /admin/assignments/{aid}/edit", a.admin(a.editAssignmentForm))
	mux.HandleFunc("POST /admin/assignments/{aid}", a.admin(a.updateAssignment))
	mux.HandleFunc("POST /admin/assignments/{aid}/up", a.admin(a.moveAssignment(-1)))
	mux.HandleFunc("POST /admin/assignments/{aid}/down", a.admin(a.moveAssignment(1)))
	mux.HandleFunc("POST /admin/assignments/{aid}/publish", a.admin(a.setAssignmentPublished(true)))
	mux.HandleFunc("POST /admin/assignments/{aid}/unpublish", a.admin(a.setAssignmentPublished(false)))
	mux.HandleFunc("GET /admin/assignments/{aid}/delete", a.admin(a.confirmDeleteAssignment))
	mux.HandleFunc("POST /admin/assignments/{aid}/delete", a.admin(a.deleteAssignment))
	mux.HandleFunc("GET /courses/{id}/assignments/{aid}", a.requireLearner(a.showAssignment))
	mux.HandleFunc("POST /courses/{id}/assignments/{aid}/files", a.requireLearner(a.uploadSubmission))
	mux.HandleFunc("POST /workbooks", a.requireLearner(a.uploadWorkbook))
	mux.HandleFunc("GET /workbooks/{id}", a.requireLearner(a.downloadWorkbook))
	mux.HandleFunc("GET /workbooks/{id}/delete", a.requireLearner(a.labScreen(a.confirmDeleteWorkbook)))
	mux.HandleFunc("POST /workbooks/{id}/delete", a.requireLearner(a.deleteWorkbook))
	mux.HandleFunc("GET /submission-files/{fid}", a.requireLearner(a.downloadSubmissionFile))
	mux.HandleFunc("GET /submission-files/{fid}/delete", a.requireLearner(a.confirmDeleteSubmissionFile))
	mux.HandleFunc("POST /submission-files/{fid}/delete", a.requireLearner(a.deleteSubmissionFile))
	mux.HandleFunc("GET /admin/courses/{id}/delete", a.admin(a.confirmDeleteCourse))
	mux.HandleFunc("POST /admin/courses/{id}/delete", a.admin(a.deleteCourse))
	mux.HandleFunc("GET /admin/courses/{id}/exam", a.admin(a.adminExam))
	mux.HandleFunc("POST /admin/courses/{id}/exam", a.admin(a.saveExam))
	mux.HandleFunc("GET /admin/courses/{id}/exam/questions/new", a.admin(a.newExamQuestionForm))
	mux.HandleFunc("POST /admin/courses/{id}/exam/questions", a.admin(a.createExamQuestion))
	mux.HandleFunc("GET /admin/courses/{id}/exam/questions/{qid}/edit", a.admin(a.editExamQuestionForm))
	mux.HandleFunc("POST /admin/courses/{id}/exam/questions/{qid}", a.admin(a.updateExamQuestion))
	mux.HandleFunc("POST /admin/courses/{id}/exam/questions/{qid}/up", a.admin(a.moveExamQuestion(false)))
	mux.HandleFunc("POST /admin/courses/{id}/exam/questions/{qid}/down", a.admin(a.moveExamQuestion(true)))
	mux.HandleFunc("GET /admin/courses/{id}/exam/questions/{qid}/delete", a.admin(a.confirmDeleteExamQuestion))
	mux.HandleFunc("POST /admin/courses/{id}/exam/questions/{qid}/delete", a.admin(a.deleteExamQuestion))
	mux.HandleFunc("POST /admin/courses/{id}/exam/questions/{qid}/publish", a.admin(a.publishExamQuestion))
	mux.HandleFunc("POST /admin/courses/{id}/exam/publish-drafts", a.admin(a.publishExamDrafts))
	mux.HandleFunc("GET /admin/question-bank", a.admin(a.adminQuestionBank))
	mux.HandleFunc("GET /admin/question-bank/{qid}/edit", a.admin(a.editBankQuestionForm))
	mux.HandleFunc("POST /admin/question-bank/{qid}", a.admin(a.updateBankQuestion))
	mux.HandleFunc("GET /admin/question-bank/{qid}/delete", a.admin(a.confirmDeleteBankQuestion))
	mux.HandleFunc("POST /admin/question-bank/{qid}/delete", a.admin(a.deleteBankQuestion))
	mux.HandleFunc("POST /admin/question-bank/{qid}/publish", a.admin(a.publishBankQuestion))
	mux.HandleFunc("POST /admin/courses/{id}/exam/from-bank", a.admin(a.addFromBank))
	mux.HandleFunc("POST /admin/courses/{id}/exam/generate", a.admin(a.generateQuestions))
	mux.HandleFunc("GET /admin/results", a.admin(a.adminResults))
	mux.HandleFunc("GET /admin/results/{attemptID}", a.admin(a.adminAttempt))
	mux.HandleFunc("POST /admin/results/{attemptID}/another-attempt", a.admin(a.giveAnotherAttempt))
	mux.HandleFunc("POST /admin/results/{attemptID}/marks", a.admin(a.saveMarks))
	mux.HandleFunc("GET /courses/{id}/exam", a.requireLearner(a.learnerExam))
	mux.HandleFunc("POST /courses/{id}/exam/attempts", a.requireLearner(a.startAttempt))
	mux.HandleFunc("GET /courses/{id}/exam/ic", a.requireLearner(a.examIC))
	mux.HandleFunc("POST /courses/{id}/exam/ic", a.requireLearner(a.setExamIC))
	mux.HandleFunc("GET /courses/{id}/exam/attempts/{attemptID}", a.requireLearner(a.showAttempt))
	mux.HandleFunc("POST /courses/{id}/exam/attempts/{attemptID}/submit", a.requireLearner(a.submitAttempt))
	mux.HandleFunc("GET /courses/{id}/exam/answer-sheet", a.requireLearner(a.downloadAnswerSheet))
	mux.HandleFunc("GET /my-certificates", a.requireLearner(a.myCertificates))
	mux.HandleFunc("GET /my-exams", a.requireLearner(a.myExams))
	mux.HandleFunc("GET /my-folder", a.requireLearner(a.myFolderRedirect))
	mux.HandleFunc("GET /my-folder/{username}", a.requireLearner(a.myFolder))
	mux.HandleFunc("GET /my-membership", a.requireLearner(a.myMembership))
	mux.HandleFunc("GET /my-profile", a.requireLearner(a.myProfile))
	mux.HandleFunc("POST /my-profile/ic", a.requireLearner(a.setMyIC))
	mux.HandleFunc("POST /my-profile/username", a.requireLearner(a.setMyUsername))
	mux.HandleFunc("POST /my-profile/picture", a.requireLearner(a.setMyPicture))
	mux.HandleFunc("POST /my-profile/picture/remove", a.requireLearner(a.removeMyPicture))
	mux.HandleFunc("GET /learner-pictures/{key}", a.requireLearner(a.learnerPicture))
	mux.HandleFunc("GET /certificates/{id}/pdf", a.downloadCertificate)
	mux.HandleFunc("GET /admin/certificate-settings", a.admin(a.certificateSettingsForm))
	mux.HandleFunc("GET /admin/certificate-settings/background", a.admin(a.certificateBackground))
	mux.HandleFunc("GET /certificates", a.verifyCertificateForm)
	mux.HandleFunc("POST /certificates", a.searchCertificates)
	mux.HandleFunc("GET /certificates/{id}", a.verifyCertificate)
	mux.HandleFunc("GET /admin/certificates", a.admin(a.adminCertificates))
	mux.HandleFunc("GET /admin/certificates/{id}/revoke", a.admin(a.confirmRevokeCertificate))
	mux.HandleFunc("POST /admin/certificates/{id}/revoke", a.admin(a.revokeCertificate))
	mux.HandleFunc("POST /admin/certificates/{id}/reissue", a.admin(a.reissueCertificate))
	mux.HandleFunc("POST /admin/certificate-settings", a.admin(a.saveCertificateSettings))
	mux.HandleFunc("GET /admin/plans", a.admin(a.adminPlans))
	mux.HandleFunc("GET /admin/plans/new", a.admin(a.newPlanForm))
	mux.HandleFunc("POST /admin/plans", a.admin(a.createPlan))
	mux.HandleFunc("GET /admin/plans/{id}/edit", a.admin(a.editPlanForm))
	mux.HandleFunc("POST /admin/plans/{id}", a.admin(a.updatePlan))
	mux.HandleFunc("POST /admin/plans/{id}/lab", a.admin(a.markLabPlan))
	mux.HandleFunc("GET /admin/plans/periods", a.admin(a.adminPlanPeriods))
	mux.HandleFunc("POST /admin/plans/periods", a.admin(a.assignPlan))
	mux.HandleFunc("GET /admin/plans/periods/{id}/end", a.admin(a.confirmEndPlanPeriod))
	mux.HandleFunc("POST /admin/plans/periods/{id}/end", a.admin(a.endPlanPeriod))
	mux.HandleFunc("GET /admin/deployments", a.admin(a.adminDeployments))
	mux.HandleFunc("POST /admin/deployments/{id}/takedown", a.admin(a.takeDown))
	mux.HandleFunc("GET /admin/log", a.requireOwner(a.systemLog))
	mux.HandleFunc("GET /admin/cavetext/caps", a.requireOwner(a.caveTextCapsForm))
	mux.HandleFunc("POST /admin/cavetext/caps", a.requireOwner(a.saveCaveTextCaps))
	mux.HandleFunc("GET /admin/cavetext", a.admin(a.adminCaveTextLearners))
	mux.HandleFunc("POST /admin/cavetext/learners/{id}/pro", a.admin(a.giveCaveTextPro))
	mux.HandleFunc("POST /admin/cavetext/learners/{id}/pro-plan", a.admin(a.giveCaveTextProPlan))
	mux.HandleFunc("POST /admin/cavetext/learners/{id}/combo", a.admin(a.giveCaveTextCombo))
	mux.HandleFunc("POST /admin/cavetext/learners/{id}/ebook-email", a.admin(a.emailCaveTextEbookLink))
	mux.HandleFunc("GET /admin/cavetext/learners/{id}/end-pro", a.admin(a.confirmEndCaveTextPro))
	mux.HandleFunc("POST /admin/cavetext/learners/{id}/end-pro", a.admin(a.endCaveTextPro))
	mux.HandleFunc("POST /admin/cavetext/payments/done", a.admin(a.paymentDone))
	mux.HandleFunc("GET /admin/prices", a.admin(a.adminPrices))
	mux.HandleFunc("POST /admin/prices", a.admin(a.savePrices))
	mux.HandleFunc("GET /admin/promos", a.admin(a.adminPromos))
	mux.HandleFunc("POST /admin/promos", a.admin(a.addPromo))
	mux.HandleFunc("GET /admin/promos/{id}/end", a.admin(a.confirmEndPromo))
	mux.HandleFunc("POST /admin/promos/{id}/end", a.admin(a.endPromo))
	mux.HandleFunc("GET /admin/cavetext/settings", a.admin(a.caveTextSettingsForm))
	mux.HandleFunc("POST /admin/cavetext/settings", a.admin(a.saveCaveTextSettings))
	mux.HandleFunc("POST /admin/cavetext/master-prompts", a.admin(a.saveCaveTextMasterPrompts))
	mux.HandleFunc("GET /cavetext", a.caveTextHome)
	mux.HandleFunc("GET /pricing", a.pricing)
	mux.HandleFunc("GET /plans", func(w http.ResponseWriter, r *http.Request) { // the Pricing page's old address
		to := "/pricing"
		if r.URL.RawQuery != "" {
			to += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, to, http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /cavetext/connect", a.caveTextConnect)
	mux.HandleFunc("GET /cavetext/api/me", a.caveTextAPI(a.caveTextMe))
	mux.HandleFunc("POST /cavetext/bcl/{key}", a.bclWebhook)
	mux.HandleFunc("POST /cavetext/api/sign-out", a.caveTextAPI(a.caveTextSignOut))
	mux.HandleFunc("GET /cavetext/api/snippets", a.caveTextAPI(a.caveTextSnippets))
	mux.HandleFunc("POST /cavetext/api/snippets", a.caveTextAPI(a.createCaveTextSnippet))
	mux.HandleFunc("PUT /cavetext/api/snippets", a.caveTextAPI(a.replaceCaveTextSnippets))
	mux.HandleFunc("PUT /cavetext/api/snippets/{id}", a.caveTextAPI(a.updateCaveTextSnippet))
	mux.HandleFunc("DELETE /cavetext/api/snippets/{id}", a.caveTextAPI(a.deleteCaveTextSnippet))
	mux.HandleFunc("PUT /cavetext/api/user-master-prompt", a.caveTextAPI(a.saveUserMasterPrompt))
	mux.HandleFunc("DELETE /cavetext/api/user-master-prompt", a.caveTextAPI(a.deleteUserMasterPrompt))
	a.handler = rejectCrossOrigin(mux)
	return a, nil
}

// sitePolicy is the Content-Security-Policy of every response; a Dashboard's files replace it with their own.
// Inline styles stay for the view-transition names; images may come from https for pictures in Lessons.
const sitePolicy = "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'self'"

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", sitePolicy)
	lookup := sync.OnceValues(func() (*learner, error) { return a.lookUpLearner(r) })
	ctx := context.WithValue(r.Context(), learnerKey{}, lookup)
	ctx = context.WithValue(ctx, labMemoKey{}, &labMemo{acc: map[int64]labAccess{}})
	a.handler.ServeHTTP(w, r.WithContext(ctx))
}

func (a *App) Close() error {
	close(a.done)
	return a.db.Close()
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	promo, err := a.promotionalItemsFor(r.Context(), learner)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "home", &homePage{pageBase: pageBase{Learner: learner}, PublicBaseURL: a.publicBaseURL, Items: promo, Placeholders: !a.production})
}

// homePage is the home page's data.
type homePage struct {
	pageBase
	PublicBaseURL string
	Items         []galleryItem // promotional Gallery items the viewer may see
	Placeholders  bool          // show marked placeholders for numbers the client has not supplied (never in production)
}

// about is The Cave Consult's company background, in the client's words.
func (a *App) about(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "about", page{})
}

// privacy is the PDPA notice; its facts are in docs/adr/0006.
func (a *App) privacy(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "privacy", page{})
}

// staticVersions maps each file under static/ to a short hash of its content, so a changed file gets a new URL.
var staticVersions = func() map[string]string {
	versions := map[string]string{}
	_ = fs.WalkDir(staticFiles, "static", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := staticFiles.ReadFile(name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		versions[strings.TrimPrefix(name, "static/")] = hex.EncodeToString(sum[:])[:12]
		return nil
	})
	return versions
}()

// staticURL is the fingerprinted address of a file under static/, for templates: {{static "cavelms.css"}}.
func staticURL(name string) string {
	return "/static/" + name + "?v=" + staticVersions[name]
}

// staticHandler serves the embedded stylesheet, fonts and images under /static/, without directory listings.
// A fingerprinted request (?v=) never changes, so browsers may keep it for a year; anything else is kept an hour.
func staticHandler() http.Handler {
	files, _ := fs.Sub(staticFiles, "static")
	server := http.StripPrefix("/static/", http.FileServerFS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Has("v") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		server.ServeHTTP(w, r)
	})
}

// adminSidebar is what the admin-side template reads: the current section, the page's language, and whether
// to show the System log, which only the Owner may know of.
type adminSidebar struct {
	Section string
	Owner   bool
	Lang    string
	Here    string // the page's address, for the language switch to come back to
}

// adminSide is the template function behind {{template "admin-side" (side "courses" $.Learner $.Lang $.Here)}}.
func adminSide(section string, l *learner, lang, here string) adminSidebar {
	return adminSidebar{Section: section, Owner: l != nil && l.IsOwner, Lang: lang, Here: here}
}

// SectionKey is the catalog key of the current section's sidebar label, which the phone menu's button shows.
func (s adminSidebar) SectionKey() string {
	switch s.Section {
	case "design":
		return "admin.side.cert-details"
	case "cavetext-settings":
		return "admin.side.cavetext-set"
	}
	return "admin.side." + s.Section
}

// busyLoader is what the cave-loader template reads: what it says to screen readers, and the lines it rotates,
// separated by "|".
type busyLoader struct{ Label, Lines string }

// loader is the template function behind {{template "cave-loader" (loader label lines)}}.
func loader(label, lines string) busyLoader { return busyLoader{label, lines} }

// courseTabs is what the course-tabs template reads: the Course, the current tab and the page's language.
type courseTabs struct {
	ID        int64
	Tab, Lang string
}

// courseTab is the template function behind {{template "course-tabs" (courseTab .Course.ID "chat" .Lang)}}.
func courseTab(id int64, tab, lang string) courseTabs { return courseTabs{id, tab, lang} }

// galleryTile is what the gallery-tile template reads: an item and the page's language.
type galleryTile struct {
	galleryItem
	Lang string
}

// tile is the template function behind {{template "gallery-tile" (tile . $.Lang)}}.
func tile(item galleryItem, lang string) galleryTile { return galleryTile{item, lang} }

// courseCard is what the course-card and course-cover templates read: a Course and the page's language.
type courseCard struct {
	course
	Lang string
}

// card is the template function behind {{template "course-card" (card . $.Lang)}}.
func card(c course, lang string) courseCard { return courseCard{c, lang} }

// sitePages parses the pages once for the process: they never change, and Apps made side by side, as tests do, share them
// instead of each holding a copy.
var sitePages = sync.OnceValues(parsePages)

// parsePages pairs each page template with layout.html and the partials in _*.html, keyed by file name without
// extension.
func parsePages() (map[string]*template.Template, error) {
	layout, err := template.New("layout.html").Funcs(template.FuncMap{"static": staticURL, "side": adminSide, "loader": loader, "courseTab": courseTab, "difficulties": func() []string { return difficulties }, "tile": tile, "card": card, "certPreview": newCertPreview, "submissionLimits": submissionLimits, "t": text.T, "tn": text.N, "size": sizeText, "inc": func(i int) int { return i + 1 }, "args": func(v ...any) []any { return v },
		"writtenMarks": func() int { return writtenMarks }, "maxWrittenAnswer": func() int { return maxWrittenAnswer }, "maxKeyPoints": func() int { return maxKeyPoints },
		"maxMessageLength": func() int { return maxMessageLength }, "maxReportReason": func() int { return maxReportReason }, "resultDelayMinutes": func() int { return resultDelayMinutes }}).ParseFS(appFiles, "templates/layout.html", "templates/_*.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	files, err := fs.Glob(appFiles, "templates/*.html")
	if err != nil {
		return nil, err
	}
	pages := map[string]*template.Template{}
	for _, file := range files {
		name := strings.TrimSuffix(path.Base(file), ".html")
		if name == "layout" || strings.HasPrefix(name, "_") { // _name.html holds partials, parsed with the layout
			continue
		}
		t, err := template.Must(layout.Clone()).ParseFS(appFiles, file)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", file, err)
		}
		pages[name] = t
	}
	return pages, nil
}

// render writes a page inside layout.html. data is a page, or a pointer to a struct embedding pageBase.
// The header needs the Learner, so it is looked up here when the handler didn't.
func (a *App) render(w http.ResponseWriter, r *http.Request, status int, name string, data any) {
	if p, ok := data.(page); ok {
		data = &p
	}
	if p, ok := data.(*page); ok && name == "message" && (status == http.StatusNotFound || status == http.StatusForbidden) {
		p.OfferHome = true // every not-found or refused page leads somewhere
	}
	withLearner, ok := data.(interface{ learnerField() **learner })
	if !ok {
		a.serverError(w, r, fmt.Errorf("render %s: %T is neither a page nor a pointer to a struct embedding pageBase", name, data))
		return
	}
	if field := withLearner.learnerField(); *field == nil {
		learner, err := a.currentLearner(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		*field = learner
	}
	if c, ok := data.(interface{ chromeField() *chrome }); ok {
		lang, err := a.language(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if *c.chromeField(), err = a.newChrome(r, lang); err != nil {
			a.serverError(w, r, err)
			return
		}
		// A message page (not found, refused, an error) has no sidebar, so it keeps the site's header to lead somewhere.
		l := *withLearner.learnerField()
		c.chromeField().Shell = l != nil && l.IsAdmin && strings.HasPrefix(r.URL.Path, "/admin") && name != "message"
		if l != nil {
			acc, err := a.labAccessOf(r.Context(), l)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			c.chromeField().Lab = acc.Open()
		}
	}
	var buf bytes.Buffer
	if err := a.pages[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// rejectCrossOrigin is the CSRF protection for every form: browsers mark
// cross-site requests with Sec-Fetch-Site or Origin, and those are refused.
// The CaveText API is exempt: it is called from the extension and takes a bearer token, never a cookie.
func rejectCrossOrigin(next http.Handler) http.Handler {
	p := http.NewCrossOriginProtection()
	p.AddInsecureBypassPattern("/cavetext/api/")
	p.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
	}))
	return p.Handler(next)
}
