package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Masterminds/sprig/v3"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/dictpress/tokenizers/indicphone"
	"github.com/knadh/goyesql"
	goyesqlx "github.com/knadh/goyesql/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func initConstants(ko *koanf.Koanf) Consts {
	c := Consts{
		Site:              ko.String("site"),
		RootURL:           ko.MustString("app.root_url"),
		AdminUsername:     ko.MustBytes("app.admin_username"),
		AdminPassword:     ko.MustBytes("app.admin_password"),
		EnableSubmissions: ko.Bool("app.enable_submissions"),
		EnableGlossary:    ko.Bool("glossary.enabled"),
		AdminAssets:       ko.Strings("app.admin_assets"),
	}

	if len(c.AdminUsername) < 6 {
		lo.Fatal("admin_username should be min 6 characters")
	}

	if len(c.AdminPassword) < 8 {
		lo.Fatal("admin_password should be min 8 characters")
	}

	return c
}

// initDB initializes a database connection.
func initDB(ko *koanf.Koanf) *sqlx.DB {
	db, err := sqlx.Connect("postgres",
		fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			ko.MustString("db.host"),
			ko.MustInt("db.port"),
			ko.MustString("db.user"),
			ko.MustString("db.password"),
			ko.MustString("db.db")))
	if err != nil {
		lo.Fatalf("error initializing DB: %v", err)
	}

	return db
}

// initFS initializes the stuffbin FileSystem to provide
// access to bunded static assets to the app.
func initFS() stuffbin.FileSystem {
	path, err := os.Executable()
	if err != nil {
		lo.Fatalf("error getting executable path: %v", err)
	}

	fs, err := stuffbin.UnStuff(path)
	if err == nil {
		return fs
	}

	// Running in local mode. Load the required static assets into
	// the in-memory stuffbin.FileSystem.
	lo.Printf("unable to initialize embedded filesystem: %v", err)
	lo.Printf("using local filesystem for static assets")

	files := []string{
		"config.sample.toml",
		"queries.sql",
		"schema.sql",
		"admin",
	}

	fs, err = stuffbin.NewLocalFS("/", files...)
	if err != nil {
		lo.Fatalf("failed to load local static files: %v", err)
	}

	return fs
}

func initQueries(fs stuffbin.FileSystem, db *sqlx.DB) *data.Queries {
	// Load SQL queries.
	qB, err := fs.Read("/queries.sql")
	if err != nil {
		lo.Fatalf("error reading queries.sql: %v", err)
	}

	qMap, err := goyesql.ParseBytes(qB)
	if err != nil {
		lo.Fatalf("error loading SQL queries: %v", err)
	}

	// Map queries to the query container.
	var q data.Queries
	if err := goyesqlx.ScanToStruct(&q, qMap, db.Unsafe()); err != nil {
		lo.Fatalf("no SQL queries loaded: %v", err)
	}

	return &q
}

func initAdminTemplates(fs stuffbin.FileSystem) *template.Template {
	// Init admin templates.
	tpls, err := stuffbin.ParseTemplatesGlob(sprig.FuncMap(), fs, "/admin/*.html")
	if err != nil {
		lo.Fatalf("error parsing admin templates: %v", err)
	}

	return tpls
}

// initTokenizers initializes all bundled tokenizers.
func initTokenizers(ko *koanf.Koanf) map[string]data.Tokenizer {
	return map[string]data.Tokenizer{
		"indicphone": indicphone.New(
			indicphone.Config{
				NumKNKeys: ko.MustInt("tokenizer.indicphone.kn.num_keys"),
				NumMLKeys: ko.MustInt("tokenizer.indicphone.ml.num_keys"),
			},
		),
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
	if app.consts.Site != "" {
		p.GET("/", handleIndexPage)
		p.GET("/dictionary/:fromLang/:toLang/:q", handleSearchPage)
		p.GET("/dictionary/:fromLang/:toLang", handleSearchPage)
		p.GET("/p/:page", handleStaticPage)

		if app.consts.EnableGlossary {
			p.GET("/glossary/:fromLang/:toLang/:initial", handleGlossaryPage)
		}

		// Static files with custom bundle handling
		srv.GET("/static/*", func(c echo.Context) error {
			staticDir := filepath.Join(app.consts.Site, "static")

			switch c.Param("*") {
			case "_bundle.js":
				return handleServeBundle(c, "js", staticDir)
			case "_bundle.css":
				return handleServeBundle(c, "css", staticDir)
			default:
				// Normal static file serving
				fs := http.StripPrefix("/static", http.FileServer(http.Dir(staticDir)))
				return echo.WrapHandler(fs)(c)
			}
		})

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
		p.POST("/api/submissions", handleNewSubmission)
		p.POST("/api/submissions/comments", handleNewComments)

		if app.consts.Site != "" {
			p.GET("/submit", handleSubmissionPage)
			p.POST("/submit", handleSubmissionPage)
		}
	}

	// Admin handlers and APIs.
	a.GET("/api/entries/:fromLang/:toLang", handleSearch)
	a.GET("/api/entries/:fromLang/:toLang/:q", handleSearch)
	a.GET("/admin/static/*", echo.WrapHandler(app.fs.FileServer()))
	a.GET("/admin", adminPage("index"))
	a.GET("/admin/search", adminPage("search"))
	a.GET("/admin/pending", adminPage("pending"))

	a.GET("/api/stats", handleGetStats)
	a.GET("/api/entries/pending", handleGetPendingEntries)
	a.GET("/api/entries/comments", handleGetComments)
	a.DELETE("/api/entries/comments/:commentID", handleDeletecomments)
	a.DELETE("/api/entries/pending", handleDeletePending)
	a.GET("/api/entries/:id", handleGetEntry)
	a.GET("/api/entries/:id/parents", handleGetParentEntries)
	a.POST("/api/entries", handleInsertEntry)
	a.PUT("/api/entries/:id", handleUpdateEntry)
	a.DELETE("/api/entries/:id", handleDeleteEntry)
	a.DELETE("/api/entries/:fromID/relations/:relID", handleDeleteRelation)
	a.POST("/api/entries/:fromID/relations/:toID", handleAddRelation)
	a.PUT("/api/entries/:id/relations/weights", handleReorderRelations)
	a.PUT("/api/entries/:id/relations/:relID", handleUpdateRelation)
	a.PUT("/api/entries/:id/submission", handleApproveSubmission)
	a.DELETE("/api/entries/:id/submission", handleRejectSubmission)

	// 404 pages.
	srv.RouteNotFound("/api/*", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusNotFound, "Unknown endpoint")
	})
	srv.RouteNotFound("/*", func(c echo.Context) error {
		return c.Render(http.StatusNotFound, "message", pageTpl{
			Title:   "404 Page not found",
			Heading: "404 Page not found",
		})
	})

	return srv
}

// initLangs loads language configuration into a given *App instance.
func initLangs(ko *koanf.Koanf) data.LangMap {
	var (
		tks = initTokenizers(ko)
		out = make(data.LangMap)
	)

	// Language configuration.
	for _, l := range ko.MapKeys("lang") {
		lang := data.Lang{ID: l, Types: make(map[string]string)}
		if err := ko.UnmarshalWithConf("lang."+l, &lang, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error loading languages: %v", err)
		}

		// Does the language use a bundled tokenizer?
		if lang.TokenizerType == "custom" {
			t, ok := tks[lang.TokenizerName]
			if !ok {
				lo.Fatalf("unknown custom tokenizer '%s'", lang.TokenizerName)
			}
			lang.Tokenizer = t
		}

		// Load external plugin.
		lo.Printf("language: %s", l)
		out[l] = lang
	}

	if len(out) == 0 {
		lo.Fatal("0 languages defined in config")
	}

	return out
}

// initDicts loads language->language dictionary map.
func initDicts(langs data.LangMap, ko *koanf.Koanf) data.Dicts {
	var (
		out   = make(data.Dicts, 0)
		dicts [][]string
	)

	if err := ko.Unmarshal("app.dicts", &dicts); err != nil {
		lo.Fatalf("error unmarshalling app.dict in config: %v", err)
	}

	// Language configuration.
	for _, pair := range dicts {
		if len(pair) != 2 {
			lo.Fatalf("app.dicts should have language pairs: %v", pair)
		}

		var (
			fromID = pair[0]
			toID   = pair[1]
		)
		from, ok := langs[fromID]
		if !ok {
			lo.Fatalf("unknown language '%s' defined in app.dicts config", fromID)
		}

		to, ok := langs[toID]
		if !ok {
			lo.Fatalf("unknown language '%s' defined in app.dicts config", toID)
		}

		out = append(out, [2]data.Lang{from, to})
	}

	if len(out) == 0 {
		lo.Fatal("0 dicts defined in config")
	}

	return out
}
