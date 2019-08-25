package main

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"plugin"
	"strings"

	"github.com/go-chi/chi"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictmaker/search"
)

// connectDB initializes a database connection.
func connectDB(host string, port int, user, pwd, dbName string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres",
		fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, pwd, dbName))
	if err != nil {
		return nil, err
	}

	return db, nil
}

// loadTheme loads a theme from a directory.
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

// loadTokenizerPlugin loads a tokenizer plugin that implements search.Tokenizer
// from the given path.
func loadTokenizerPlugin(path string) (search.Tokenizer, error) {
	plg, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error loading tokenizer plugin '%s': %v", path, err)
	}

	newFunc, err := plg.Lookup("New")
	if err != nil {
		return nil, fmt.Errorf("New() function not found in plugin '%s': %v", path, err)
	}

	f, ok := newFunc.(func() (search.Tokenizer, error))
	if !ok {
		return nil, fmt.Errorf("New() function is of invalid type in plugin '%s'", path)
	}

	// Initialize the plugin.
	p, err := f()
	if err != nil {
		return nil, fmt.Errorf("error initializing provider plugin '%s': %v", path, err)
	}

	return p, err
}

// registerHandlers registers HTTP handlers.
func registerHandlers(r *chi.Mux, app *App) {
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
	r.Get("/api/dictionary/{fromLang}/{toLang}/{q}", wrap(app, handleSearch))
}

// loadLanguages loads language configuration into a given *App instance.
func loadLanguages(app *App) error {
	// Language configuration.
	for _, l := range ko.MapKeys("lang") {
		var lang Lang
		ko.Unmarshal("lang."+l, &lang)

		// Load external plugin.
		logger.Printf("language: %s", l)
		if lang.TokenizerType == "plugin" {
			tk, err := loadTokenizerPlugin(lang.TokenizerName)
			if err != nil {
				return err
			}
			lang.Tokenizer = tk

			// Tokenizations for search queries are looked up by the tokenizer
			// ID() returned by the plugin and not the filename in the config.
			lang.TokenizerName = tk.ID()
			logger.Printf("loaded tokenizer %s", lang.TokenizerName)
		}
		app.lang[l] = lang
	}
	return nil
}
