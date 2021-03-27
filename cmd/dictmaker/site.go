package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/knadh/dictmaker/internal/search"
	"github.com/knadh/paginator"
)

const (
	pageIndex    = "/"
	pageSearch   = "search"
	pageGlossary = "glossary"
	pageStatic   = "static"
)

type sitePage struct {
	Path        string
	Page        string
	Title       string
	Description string
	MetaTags    string

	Query    *search.Query
	Results  *Results
	Glossary *Glossary
	Initial  string
	Initials []string
	Pg       *paginator.Set
	PgBar    template.HTML
}

// siteMsg contains the context for the "message" template.
// This is used for rendering adhoc messages including errors.
type siteMsg struct {
	Path        string
	Page        string
	Title       string
	Heading     string
	Description string
	Query       *search.Query
}

// handleIndexPage renders the homepage.
func handleIndexPage(w http.ResponseWriter, r *http.Request) {
	app, _ := r.Context().Value("app").(*App)
	sendTpl(http.StatusOK, "index", app.site, sitePage{
		Path: r.RequestURI,
		Page: pageIndex,
	}, w)
}

// handleSearchPage renders the search results page.
func handleSearchPage(w http.ResponseWriter, r *http.Request) {
	app, _ := r.Context().Value("app").(*App)

	query, out, err := doSearch(r, app)
	if err != nil {
		app.logger.Printf("error searching: %v", err)
		sendTpl(http.StatusInternalServerError, "message", app.site, siteMsg{
			Title:       "Error",
			Heading:     "Error",
			Description: err.Error(),
		}, w)

		return
	}

	// Render the results.
	sendTpl(http.StatusOK, "search", app.site, sitePage{
		Path:    r.RequestURI,
		Page:    pageSearch,
		Results: out,
		Query:   &query,
	}, w)
}

// handleGlossaryPage renders the alphabet glossary page.
func handleGlossaryPage(w http.ResponseWriter, r *http.Request) {
	var (
		app      = r.Context().Value("app").(*App)
		fromLang = chi.URLParam(r, "fromLang")
		toLang   = chi.URLParam(r, "toLang")
		initial  = chi.URLParam(r, "initial")
		pg       = app.glossaryPg.NewFromURL(r.URL.Query())
	)

	// Get the alphabets.
	initials, err := app.search.GetInitials(fromLang)
	if err != nil {
		app.logger.Printf("error getting initials: %v", err)
		sendTpl(http.StatusInternalServerError, "message", app.site, siteMsg{
			Title:       "Error",
			Heading:     "Error",
			Description: "Error fetching glossary initials.",
		}, w)
	}

	if len(initials) == 0 {
		// No glossary initials found.
		sendTpl(http.StatusOK, "glossary", app.site, sitePage{
			Path:    r.RequestURI,
			Page:    pageGlossary,
			Initial: initial,
		}, w)
	}

	// If there's no initial, pick the first one.
	if initial == "" {
		http.Redirect(w, r, fmt.Sprintf("/glossary/%s/%s/%s", fromLang, toLang, initials[0]),
			http.StatusFound)
		return
	}

	// Get words.
	gloss, err := getGlossaryWords(fromLang, initial, pg, app)
	if err != nil {
		app.logger.Printf("error getting glossary words: %v", err)
		sendTpl(http.StatusInternalServerError, "message", app.site, siteMsg{
			Title:       "Error",
			Heading:     "Error",
			Description: "Error fetching glossary words.",
		}, w)

		return
	}

	gloss.FromLang = fromLang
	gloss.ToLang = toLang
	pg.SetTotal(gloss.Total)

	// Render the results.
	sendTpl(http.StatusOK, "glossary", app.site, sitePage{
		Path:     r.RequestURI,
		Page:     pageGlossary,
		Initial:  initial,
		Initials: initials,
		Glossary: gloss,
		Pg:       &pg,
		PgBar:    template.HTML(pg.HTML("?page=%d")),
	}, w)
}

// handleStaticPage renders an arbitrary static page.
func handleStaticPage(w http.ResponseWriter, r *http.Request) {
	var (
		app = r.Context().Value("app").(*App)
		id  = "page-" + chi.URLParam(r, "page")
	)

	if app.site.Lookup(id) == nil {
		sendErrorResponse("Page not found", http.StatusNotFound, nil, w)
		return
	}

	sendTpl(http.StatusOK, id, app.site, sitePage{
		Path: r.RequestURI,
		Page: pageStatic,
	}, w)
}

func wrap(app *App, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), interface{}("app"), app)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
