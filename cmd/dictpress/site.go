package main

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Masterminds/sprig/v3"
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

type pageTpl struct {
	PageName    string
	Title       string
	Heading     string
	Description string
	MetaTags    string

	Query    *data.Query
	Results  *results
	Glossary *glossary
	Initial  string
	Initials []string
	Pg       *paginator.Set
	PgBar    template.HTML
}

// tplData is the data container that is injected
// into public templates for accessing data.
type tplData struct {
	// These are available in the template as .Page, .Data etc.
	RootURL           string
	EnableSubmissions bool
	Langs             data.LangMap

	Path string
	Data interface{}
}

// tplRenderer wraps a template.tplRenderer for echo.
type tplRenderer struct {
	tpls *template.Template
}

// handleIndexPage renders the homepage.
func handleIndexPage(c echo.Context) error {
	return c.Render(http.StatusOK, "index", pageTpl{
		PageName: pageIndex,
	})
}

// handleSearchPage renders the search results page.
func handleSearchPage(c echo.Context) error {
	query, res, err := doSearch(c, false)
	if err != nil {
		return c.Render(http.StatusInternalServerError, "message", pageTpl{
			Title:       "Error",
			Heading:     "Error",
			Description: err.Error(),
		})
	}

	return c.Render(http.StatusOK, "search", pageTpl{
		PageName: pageSearch,
		Results:  res,
		Query:    &query,
	})
}

// handleSubmissionPage renders the new entry submission page.
func handleSubmissionPage(c echo.Context) error {
	if c.Request().Method == http.MethodPost {
		if err := handleNewSubmission(c); err != nil {
			e := err.(*echo.HTTPError)
			return c.Render(e.Code, "message", pageTpl{
				Title:       "Error",
				Heading:     "Error",
				Description: fmt.Sprintf("%s", e.Message),
			})
		}

		return c.Render(http.StatusOK, "message", pageTpl{
			Title:       "Submitted",
			Heading:     "Submitted",
			Description: "Your entry has been submitted for review.",
		})
	}

	return c.Render(http.StatusOK, "submit-entry", pageTpl{
		Title: "Submit a new entry",
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
		app.lo.Printf("error getting initials: %v", err)
		return c.Render(http.StatusInternalServerError, "message", pageTpl{
			Title:       "Error",
			Heading:     "Error",
			Description: "Error fetching glossary initials.",
		})
	}

	if len(initials) == 0 {
		// No glossary initials found.
		return c.Render(http.StatusOK, "glossary", pageTpl{
			PageName: pageGlossary,
			Initial:  initial,
		})
	}

	// If there's no initial, pick the first one.
	if initial == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/glossary/%s/%s/%s", fromLang, toLang, initials[0]))
	}

	// Get words.
	gloss, err := getGlossaryWords(fromLang, initial, pg, app)
	if err != nil {
		app.lo.Printf("error getting glossary words: %v", err)
		return c.Render(http.StatusInternalServerError, "message", pageTpl{
			Title:       "Error",
			Heading:     "Error",
			Description: "Error fetching glossary words.",
		})
	}

	gloss.FromLang = fromLang
	gloss.ToLang = toLang
	pg.SetTotal(gloss.Total)

	// Render the results.
	return c.Render(http.StatusOK, "glossary", pageTpl{
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
		return c.Render(http.StatusNotFound, "message", pageTpl{
			Title:   "404",
			Heading: "Page not found",
		})
	}

	return c.Render(http.StatusOK, id, pageTpl{
		PageName: pageStatic,
	})
}

// loadSite loads HTML site theme templates.
func loadSite(path string, loadPages bool) (*template.Template, error) {
	t := template.New("site").Funcs(sprig.FuncMap())

	// Go percentage encodes unicode characters printed in <a href>,
	// but the encoded values are in lowercase hex (for some reason)
	// See: https://github.com/golang/go/issues/33596
	t.Funcs(template.FuncMap{"UnicodeURL": func(s string) template.URL {
		s = strings.ReplaceAll(s, " ", "+")
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
	app := c.Get("app").(*App)

	return t.tpls.ExecuteTemplate(w, name, tplData{
		Path:              c.Path(),
		RootURL:           app.constants.RootURL,
		EnableSubmissions: app.constants.EnableSubmissions,
		Langs:             app.data.Langs,
		Data:              data,
	})
}
