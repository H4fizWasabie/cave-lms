package app

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
	"github.com/H4fizWasabie/cave-lms/internal/storage"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql templates/*.html
var appFiles embed.FS

type App struct {
	db            *sql.DB
	handler       http.Handler
	mailer        mailer.Mailer
	pages         map[string]*template.Template
	publicBaseURL string
	store         *storage.Disk
	maxUpload     int64
	limiter       rateLimiter
}

// page is the data every template receives.
type page struct {
	PublicBaseURL string
	Learner       *learner
	Title         string
	Message       string
	Error         string
	Name          string
	Email         string
	OfferResend   bool
	OfferLogin    bool
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
	Status        *examStatus // the Learner's Exam
	Attempt       *examAttempt
	Question      *examQuestion
	Enrolled      bool
	Done          int // Lessons the Learner has completed
	Total         int // published Lessons in the Course
	Lessons       []lesson
	Lesson        *lesson
	Enrollees     []enrollee
	Certificates  []certificate
	Settings      *certificateSettings
}

func New(ctx context.Context, cfg config.Config, mailer mailer.Mailer) (_ *App, err error) {
	if mailer == nil {
		return nil, fmt.Errorf("mailer is required")
	}
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
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
	goose.SetBaseFS(appFiles)
	if err = goose.SetDialect("postgres"); err != nil {
		return nil, fmt.Errorf("set migration dialect: %w", err)
	}
	if err = goose.UpContext(ctx, db, "migrations"); err != nil {
		return nil, fmt.Errorf("apply database migrations: %w", err)
	}

	a := &App{db: db, mailer: mailer, pages: pages, publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"), store: store, maxUpload: cfg.MaxUploadBytes}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("GET /register", a.registerForm)
	mux.HandleFunc("POST /register", a.limited(a.register))
	mux.HandleFunc("GET /verify", a.verify)
	mux.HandleFunc("POST /verify/resend", a.resendVerification)
	mux.HandleFunc("GET /login", a.loginForm)
	mux.HandleFunc("POST /login", a.limited(a.login))
	mux.HandleFunc("GET /forgot-password", a.forgotPasswordForm)
	mux.HandleFunc("POST /forgot-password", a.forgotPassword)
	mux.HandleFunc("GET /reset-password", a.resetPasswordForm)
	mux.HandleFunc("POST /reset-password", a.resetPassword)
	mux.HandleFunc("POST /logout", a.logout)
	mux.HandleFunc("GET /courses", a.courses)
	mux.HandleFunc("GET /courses/{id}", a.showCourse)
	mux.HandleFunc("POST /courses/{id}/join", a.requireLearner(a.joinCourse))
	mux.HandleFunc("POST /courses/{id}/leave", a.requireLearner(a.leaveCourse))
	mux.HandleFunc("GET /my-courses", a.requireLearner(a.myCourses))
	mux.HandleFunc("GET /admin", a.admin(a.adminHome))
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
	mux.HandleFunc("GET /admin/courses", a.admin(a.adminCourses))
	mux.HandleFunc("GET /admin/courses/new", a.admin(a.newCourseForm))
	mux.HandleFunc("POST /admin/courses", a.admin(a.createCourse))
	mux.HandleFunc("GET /admin/courses/{id}", a.admin(a.adminCourse))
	mux.HandleFunc("POST /admin/courses/{id}/learners/{learnerID}/remove", a.admin(a.removeLearner))
	mux.HandleFunc("GET /admin/courses/{id}/edit", a.admin(a.editCourseForm))
	mux.HandleFunc("POST /admin/courses/{id}", a.admin(a.updateCourse))
	mux.HandleFunc("POST /admin/courses/{id}/open", a.admin(a.setCourseOpen(true)))
	mux.HandleFunc("POST /admin/courses/{id}/close", a.admin(a.setCourseOpen(false)))
	mux.HandleFunc("GET /courses/{id}/lessons/{lessonID}", a.requireLearner(a.showLesson))
	mux.HandleFunc("POST /courses/{id}/lessons/{lessonID}/complete", a.requireLearner(a.setLessonComplete(true)))
	mux.HandleFunc("POST /courses/{id}/lessons/{lessonID}/incomplete", a.requireLearner(a.setLessonComplete(false)))
	mux.HandleFunc("GET /admin/courses/{id}/lessons", a.admin(a.adminLessons))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/up", a.admin(a.moveLesson(-1)))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/down", a.admin(a.moveLesson(1)))
	mux.HandleFunc("GET /admin/lessons/{lessonID}/edit", a.admin(a.editLessonForm))
	mux.HandleFunc("POST /admin/lessons/{lessonID}", a.admin(a.updateLesson))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/delete", a.admin(a.deleteLesson))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/publish", a.admin(a.setLessonPublished(true)))
	mux.HandleFunc("POST /admin/lessons/{lessonID}/unpublish", a.admin(a.setLessonPublished(false)))
	mux.HandleFunc("POST /admin/courses/{id}/lessons/publish-all", a.admin(a.publishAllLessons))
	mux.HandleFunc("GET /admin/courses/{id}/lessons/new", a.admin(a.newLessonForm))
	mux.HandleFunc("POST /admin/courses/{id}/lessons", a.admin(a.createLesson))
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
	mux.HandleFunc("POST /admin/courses/{id}/exam/questions/{qid}/delete", a.admin(a.deleteExamQuestion))
	mux.HandleFunc("POST /admin/courses/{id}/exam/questions/{qid}/publish", a.admin(a.publishExamQuestion))
	mux.HandleFunc("GET /courses/{id}/exam", a.requireLearner(a.learnerExam))
	mux.HandleFunc("POST /courses/{id}/exam/attempts", a.requireLearner(a.startAttempt))
	mux.HandleFunc("GET /courses/{id}/exam/attempts/{attemptID}", a.requireLearner(a.showAttempt))
	mux.HandleFunc("POST /courses/{id}/exam/attempts/{attemptID}/submit", a.requireLearner(a.submitAttempt))
	mux.HandleFunc("GET /my-certificates", a.requireLearner(a.myCertificates))
	mux.HandleFunc("GET /certificates/{id}/pdf", a.downloadCertificate)
	mux.HandleFunc("GET /admin/certificate-settings", a.admin(a.certificateSettingsForm))
	mux.HandleFunc("POST /admin/certificate-settings", a.admin(a.saveCertificateSettings))
	a.handler = rejectCrossOrigin(mux)
	return a, nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.handler.ServeHTTP(w, r)
}

func (a *App) Close() error {
	return a.db.Close()
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	learner, err := a.currentLearner(r)
	if err != nil {
		serverError(w, err)
		return
	}
	promo, err := a.galleryItemsFor(r.Context(), learner, true)
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, http.StatusOK, "home", page{PublicBaseURL: a.publicBaseURL, Learner: learner, Items: promo})
}

// parsePages pairs each page template with layout.html, keyed by file name without extension.
func parsePages() (map[string]*template.Template, error) {
	layout, err := template.ParseFS(appFiles, "templates/layout.html")
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
		if name == "layout" {
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

func (a *App) render(w http.ResponseWriter, status int, name string, data page) {
	var buf bytes.Buffer
	if err := a.pages[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func serverError(w http.ResponseWriter, err error) {
	log.Print(err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// rejectCrossOrigin is the CSRF protection for every form: browsers mark
// cross-site requests with Sec-Fetch-Site or Origin, and those are refused.
// ponytail: hand port of Go 1.25's http.CrossOriginProtection; switch to it once go.mod reaches 1.25.
func rejectCrossOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && crossOrigin(r) {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func crossOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return false
	case "":
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false // not a browser form post
		}
		u, err := url.Parse(origin)
		return err != nil || u.Host != r.Host
	default:
		return true
	}
}
