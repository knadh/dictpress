package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	mrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"unicode"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/dictpress/tokenizers/indicphone"
	"github.com/knadh/koanf"
	"github.com/knadh/stuffbin"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func initConstants(ko *koanf.Koanf) constants {
	c := constants{
		Site:          ko.String("site"),
		RootURL:       ko.MustString("app.root_url"),
		AdminUsername: ko.MustBytes("app.admin_username"),
		AdminPassword: ko.MustBytes("app.admin_password"),
	}

	if len(c.AdminUsername) < 6 {
		logger.Fatal("admin_username should be min 6 characters")
	}

	if len(c.AdminPassword) < 8 {
		logger.Fatal("admin_password should be min 8 characters")
	}

	return c
}

// initDB initializes a database connection.
func initDB(host string, port int, user, pwd, dbName string) *sqlx.DB {
	db, err := sqlx.Connect("postgres",
		fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, pwd, dbName))
	if err != nil {
		logger.Fatalf("error intiializing DB: %v", err)
	}

	return db
}

// initFS initializes the stuffbin FileSystem to provide
// access to bunded static assets to the app.
func initFS() stuffbin.FileSystem {
	path, err := os.Executable()
	if err != nil {
		logger.Fatalf("error getting executable path: %v", err)
	}

	fs, err := stuffbin.UnStuff(path)
	if err == nil {
		return fs
	}

	// Running in local mode. Load the required static assets into
	// the in-memory stuffbin.FileSystem.
	logger.Printf("unable to initialize embedded filesystem: %v", err)
	logger.Printf("using local filesystem for static assets")

	files := []string{
		"config.sample.toml",
		"queries.sql",
		"schema.sql",
		"admin",
	}

	fs, err = stuffbin.NewLocalFS("/", files...)
	if err != nil {
		logger.Fatalf("failed to load local static files: %v", err)
	}

	return fs
}

func initAdminTemplates(app *App) *template.Template {
	// Init admin templates.
	tpls, err := stuffbin.ParseTemplatesGlob(nil, app.fs, "/admin/*.html")
	if err != nil {
		logger.Fatalf("error parsing e-mail notif templates: %v", err)
	}
	return tpls
}

// initTokenizers initializes all bundled tokenizers.
func initTokenizers() map[string]data.Tokenizer {
	return map[string]data.Tokenizer{
		"indicphone": indicphone.New(),
	}
}

func initHTTPServer(app *App, ko *koanf.Koanf) *echo.Echo {
	srv := echo.New()
	srv.Debug = true
	srv.HideBanner = true

	// Register app (*App) to be injected into all HTTP handlers.
	srv.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("app", app)
			return next(c)
		}
	})

	var (
		// Public handlers with no auth.
		p = srv.Group("")

		// Admin handlers with auth.
		a = srv.Group("", middleware.BasicAuth(basicAuth))
	)

	// Dictionary site HTML views.
	if app.constants.Site != "" {
		p.GET("/", handleIndexPage)
		p.GET("/dictionary/:fromLang/:toLang/:q", handleSearchPage)
		p.GET("/dictionary/:fromLang/:toLang", handleGlossaryPage)
		p.GET("/glossary/:fromLang/:toLang/:initial", handleGlossaryPage)
		p.GET("/glossary/:fromLang/:toLang", handleGlossaryPage)
		p.GET("/pages/:page", handleStaticPage)

		// Static files.
		fs := http.StripPrefix("/static", http.FileServer(
			http.Dir(filepath.Join(app.constants.Site, "static"))))
		srv.GET("/static/*", echo.WrapHandler(fs))

	} else {
		// API greeting if there's no site.
		p.GET("/", func(c echo.Context) error {
			return c.JSON(http.StatusOK, okResp{"welcome"})
		})
	}

	// Public APIs.
	p.GET("/api/config", handleGetConfig)
	p.GET("/api/dictionary/:fromLang/:toLang/:q", handleSearch)

	// Public user submission APIs.
	if ko.Bool("app.enable_submissions") {
		p.POST("/api/submissions/new", handleNewSubmission)
		p.POST("/api/submissions/comments", handleNewComments)
		p.GET("/submit", handleSubmissionPage)
		p.POST("/submit", handleNewSubmission)
	}

	// Admin handlers and APIs.
	a.GET("/admin/static/*", echo.WrapHandler(app.fs.FileServer()))
	a.GET("/admin", adminPage("index"))
	a.GET("/admin/search", adminPage("search"))
	a.GET("/admin/pending", adminPage("pending"))

	a.GET("/api/stats", handleGetStats)
	a.GET("/api/entries/pending", handleGetPendingEntries)
	a.GET("/api/entries/comments", handleGetComments)
	a.DELETE("/api/entries/comments/:commentID", handleDeletecomments)
	a.GET("/api/entries/:id", handleGetEntry)
	a.GET("/api/entries/:id/parents", handleGetParentEntries)
	a.POST("/api/entries", handleInsertEntry)
	a.PUT("/api/entries/:id", handleUpdateEntry)
	a.DELETE("/api/entries/:id", handleDeleteEntry)
	a.DELETE("/api/entries/:fromID/relations/:toID", handleDeleteRelation)
	a.POST("/api/entries/:fromID/relations/:toID", handleAddRelation)
	a.PUT("/api/entries/:id/relations/weights", handleReorderRelations)
	a.PUT("/api/entries/:id/relations/:relID", handleUpdateRelation)
	a.PUT("/api/entries/:id/submission", handleApproveSubmission)
	a.DELETE("/api/entries/:id/submission", handleRejectSubmission)

	return srv
}

// initLangs loads language configuration into a given *App instance.
func initLangs(ko *koanf.Koanf) data.LangMap {
	var (
		tks = initTokenizers()
		out = make(data.LangMap)
	)

	// Language configuration.
	for _, l := range ko.MapKeys("lang") {
		lang := data.Lang{Types: make(map[string]string)}
		if err := ko.UnmarshalWithConf("lang."+l, &lang, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			logger.Fatalf("error loading languages: %v", err)
		}

		// Does the language use a bundled tokenizer?
		if lang.TokenizerType == "custom" {
			t, ok := tks[lang.TokenizerName]
			if !ok {
				logger.Fatalf("unknown custom tokenizer '%s'", lang.TokenizerName)
			}
			lang.Tokenizer = t
		}

		// Load external plugin.
		logger.Printf("language: %s", l)
		out[l] = lang
	}

	return out
}

func generateNewFiles() error {
	if _, err := os.Stat("config.toml"); !os.IsNotExist(err) {
		return errors.New("config.toml exists. Remove it to generate a new one")
	}

	// Initialize the static file system into which all
	// required static assets (.sql, .js files etc.) are loaded.
	fs := initFS()

	// Generate config file.
	b, err := fs.Read("config.sample.toml")
	if err != nil {
		return fmt.Errorf("error reading sample config (is binary stuffed?): %v", err)
	}

	// Inject a random password.
	p := make([]byte, 12)
	rand.Read(p)
	pwd := []byte(fmt.Sprintf("%x", p))

	for i, c := range pwd {
		if mrand.Intn(4) == 1 {
			pwd[i] = byte(unicode.ToUpper(rune(c)))
		}
	}

	b = bytes.Replace(b, []byte("dictpress_admin_password"), pwd, -1)

	if err := ioutil.WriteFile("config.toml", b, 0644); err != nil {
		return err
	}

	return nil
}
