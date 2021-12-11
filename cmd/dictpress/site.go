package main

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
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

	Query    *data.Query
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
	Query       *data.Query
}

// tplRenderer wraps a template.tplRenderer for echo.
type tplRenderer struct {
	tpls    *template.Template
	RootURL string
}

// tplData is the data container that is injected
// into public templates for accessing data.
type tplData struct {
	RootURL string
	Data    interface{}
}

// handleIndexPage renders the homepage.
func handleIndexPage(c echo.Context) error {
	return c.Render(http.StatusOK, "index", sitePage{
		Path: c.Path(),
		Page: pageIndex,
	})
}

// handleSearchPage renders the search results page.
func handleSearchPage(c echo.Context) error {
	app := c.Get("app").(*App)

	query, out, err := doSearch(c)
	if err != nil {
		app.logger.Printf("error searching: %v", err)
		return c.Render(http.StatusInternalServerError, "message", siteMsg{
			Title:       "Error",
			Heading:     "Error",
			Description: err.Error(),
		})
	}

	// Render the results.
	return c.Render(http.StatusOK, "search", sitePage{
		Path:    c.Path(),
		Page:    pageSearch,
		Results: out,
		Query:   &query,
	})
}

// handleGlossaryPage renders the alphabet glossary page.
func handleGlossaryPage(c echo.Context) error {
	var (
		app      = c.Get("app").(*App)
		fromLang = c.Param("fromLang")
		toLang   = c.Param("toLang")
		initial  = c.Param("initial")
		pg       = app.glossaryPg.NewFromURL(c.Request().URL.Query())
	)

	// Get the alphabets.
	initials, err := app.data.GetInitials(fromLang)
	if err != nil {
		app.logger.Printf("error getting initials: %v", err)
		return c.Render(http.StatusInternalServerError, "message", siteMsg{
			Title:       "Error",
			Heading:     "Error",
			Description: "Error fetching glossary initials.",
		})
	}

	if len(initials) == 0 {
		// No glossary initials found.
		return c.Render(http.StatusOK, "glossary", sitePage{
			Path:    c.Path(),
			Page:    pageGlossary,
			Initial: initial,
		})
	}

	// If there's no initial, pick the first one.
	if initial == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/glossary/%s/%s/%s", fromLang, toLang, initials[0]))
	}

	// Get words.
	gloss, err := getGlossaryWords(fromLang, initial, pg, app)
	if err != nil {
		app.logger.Printf("error getting glossary words: %v", err)
		return c.Render(http.StatusInternalServerError, "message", siteMsg{
			Title:       "Error",
			Heading:     "Error",
			Description: "Error fetching glossary words.",
		})
	}

	gloss.FromLang = fromLang
	gloss.ToLang = toLang
	pg.SetTotal(gloss.Total)

	// Render the results.
	return c.Render(http.StatusOK, "glossary", sitePage{
		Path:     c.Path(),
		Page:     pageGlossary,
		Initial:  initial,
		Initials: initials,
		Glossary: gloss,
		Pg:       &pg,
		PgBar:    template.HTML(pg.HTML("?page=%d")),
	})
}

// handleStaticPage renders an arbitrary static page.
func handleStaticPage(c echo.Context) error {
	var (
		app = c.Get("app").(*App)
		id  = "page-" + c.Param("page")
	)

	if app.siteTpl.Lookup(id) == nil {
		return c.Render(http.StatusNotFound, "message", siteMsg{
			Title:   "404",
			Heading: "Page not found",
		})
	}

	return c.Render(http.StatusOK, id, sitePage{
		Path: c.Path(),
		Page: pageStatic,
	})
}

// loadSite loads HTML site theme templates.
func loadSite(path string, loadPages bool) (*template.Template, error) {
	t := template.New("site")

	// Helper functions.
	t.Funcs(template.FuncMap{
		"JoinStrings": strings.Join,
		"ToUpper":     strings.ToUpper,
		"ToLower":     strings.ToLower,
		"Title":       strings.Title,
	})

	// Go percentage encodes unicode characters printed in <a href>,
	// but the encoded values are in lowercase hex (for some reason)
	// See: https://github.com/golang/go/issues/33596
	t.Funcs(template.FuncMap{"UnicodeURL": func(s string) template.URL {
		return template.URL(url.PathEscape(s))
	}})

	if _, err := t.ParseGlob(path + "/*.html"); err != nil {
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

// Render executes and renders a template for echo.
func (t *tplRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.tpls.ExecuteTemplate(w, name, data)
}
