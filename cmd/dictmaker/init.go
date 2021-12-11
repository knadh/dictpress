package main

import (
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictmaker/internal/data"
	"github.com/knadh/dictmaker/tokenizers/indicphone"
	"github.com/knadh/koanf"
	"github.com/knadh/stuffbin"
)

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
		"config.toml.sample",
		"queries.sql",
		"schema.sql",
	}

	fs, err = stuffbin.NewLocalFS("/", files...)
	if err != nil {
		logger.Fatalf("failed to initialize local file for assets: %v", err)
	}

	return fs
}

// loadSiteTheme loads a theme from a directory.
func loadSiteTheme(path string, loadPages bool) (*template.Template, error) {
	t := template.New("theme")

	// Helper functions.
	t = t.Funcs(template.FuncMap{"JoinStrings": strings.Join})
	t = t.Funcs(template.FuncMap{"ToUpper": strings.ToUpper})
	t = t.Funcs(template.FuncMap{"ToLower": strings.ToLower})
	t = t.Funcs(template.FuncMap{"Title": strings.Title})

	// Go percentage encodes unicode characters printed in <a href>,
	// but the encoded values are in lowercase hex (for some reason)
	// See: https://github.com/golang/go/issues/33596
	t = t.Funcs(template.FuncMap{"UnicodeURL": func(s string) template.URL {
		return template.URL(url.PathEscape(s))
	}})

	_, err := t.ParseGlob(path + "/*.html")
	if err != nil {
		return t, err
	}

	// Load arbitrary pages from (site_dir/pages/*.html).
	// For instance, "about" for site_dir/pages/about.html will be
	// rendered on site.com/pages/about where the template is defined
	// with the name {{ define "page-about" }}. All template name definitions
	// should be "page-*".
	if loadPages {
		if _, err := t.ParseGlob(path + "/pages/*.html"); err != nil {
			return t, err
		}
	}

	return t, nil
}

// initAdminTemplates loads admin UI HTML templates.
func initAdminTemplates(path string) *template.Template {
	t, err := template.New("admin").ParseGlob(path + "/*.html")
	if err != nil {
		logger.Fatalf("error loading admin templates: %v", err)
	}
	return t
}

// initTokenizers initializes all bundled tokenizers.
func initTokenizers() map[string]data.Tokenizer {
	return map[string]data.Tokenizer{
		"indicphone": indicphone.New(),
	}
}

// initHandlers registers HTTP handlers.
func initHandlers(r *chi.Mux, app *App) {
	r.Use(middleware.StripSlashes)

	// Dictionary site HTML views.
	if app.constants.Site != "" {
		r.Get("/", wrap(app, handleIndexPage))
		r.Get("/dictionary/{fromLang}/{toLang}/{q}", wrap(app, handleSearchPage))
		r.Get("/dictionary/{fromLang}/{toLang}", wrap(app, handleGlossaryPage))
		r.Get("/glossary/{fromLang}/{toLang}/{initial}", wrap(app, handleGlossaryPage))
		r.Get("/glossary/{fromLang}/{toLang}", wrap(app, handleGlossaryPage))
		r.Get("/pages/{page}", wrap(app, handleStaticPage))

		// Static files.
		fs := http.StripPrefix("/static", http.FileServer(
			http.Dir(filepath.Join(app.constants.Site, "static"))))
		r.Get("/static/*", fs.ServeHTTP)
	} else {
		// API greeting if there's no site.
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			sendResponse("welcome to dictmaker", http.StatusOK, w)
		})
	}

	// Admin handlers.
	r.Get("/admin/static/*", http.StripPrefix("/admin/static", http.FileServer(http.Dir("admin/static"))).ServeHTTP)
	r.Get("/admin", wrap(app, adminPage("index")))
	r.Get("/admin/search", wrap(app, adminPage("search")))
	r.Get("/admin/entries/{id}", wrap(app, adminPage("entry")))

	// APIs.
	r.Get("/api/config", wrap(app, handleGetConfig))
	r.Get("/api/stats", wrap(app, handleGetStats))
	r.Post("/api/entries", wrap(app, handleInsertEntry))
	r.Get("/api/entries/{id}", wrap(app, handleGetEntry))
	r.Get("/api/entries/{id}/parents", wrap(app, handleGetParentEntries))
	r.Delete("/api/entries/{id}", wrap(app, handleDeleteEntry))
	r.Delete("/api/entries/{fromID}/relations/{toID}", wrap(app, handleDeleteRelation))
	r.Post("/api/entries/{fromID}/relations/{toID}", wrap(app, handleAddRelation))
	r.Put("/api/entries/{id}/relations/weights", wrap(app, handleReorderRelations))
	r.Put("/api/entries/{id}/relations/{relID}", wrap(app, handleUpdateRelation))
	r.Put("/api/entries/{id}", wrap(app, handleUpdateEntry))
	r.Get("/api/dictionary/{fromLang}/{toLang}/{q}", wrap(app, handleSearch))
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
	b, err := fs.Read("config.toml.sample")
	if err != nil {
		return fmt.Errorf("error reading sample config (is binary stuffed?): %v", err)
	}

	if err := ioutil.WriteFile("config.toml", b, 0644); err != nil {
		return err
	}

	return nil
}
