package main

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/go-i18n"
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
	PageType string
	PageID   string

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
	EnableSubmissions bool
	Langs             data.LangMap
	Dicts             data.Dicts
	L                 *i18n.I18n
	Consts            Consts

	AssetVer string
	Path     string
	Data     interface{}
}

// tplRenderer wraps a template.tplRenderer for echo.
type tplRenderer struct {
	tpls *template.Template
}

// Random hash that changes every time the program boots, to append as
// ?v=$hash to static assets in templates for cache-busting on program restarts.
var assetVer string

func init() {
	b := md5.Sum([]byte(time.Now().String()))
	assetVer = fmt.Sprintf("%x", b)[0:10]
}

// handleIndexPage renders the homepage.
func handleIndexPage(c echo.Context) error {
	return c.Render(http.StatusOK, "index", pageTpl{
		PageType: pageIndex,
	})
}

// handleSearchPage renders the search results page.
func handleSearchPage(c echo.Context) error {
	var (
		app = c.Get("app").(*App)
	)

	q, err := prepareQuery(c)
	if err != nil {
		return c.Render(http.StatusInternalServerError, "message", pageTpl{
			Title: "Error", Heading: "Error", Description: err.Error(),
		})
	}

	// Apply search query limits on relations and content items.
	q.MaxRelations = app.consts.SiteMaxEntryRelationsPerType
	q.MaxContentItems = app.consts.SiteMaxEntryContentItems

	res, err := doSearch(q, false, app.pgSite, app)
	if err != nil {
		return c.Render(http.StatusInternalServerError, "message", pageTpl{
			Title: "Error", Heading: "Error", Description: err.Error(),
		})
	}

	return c.Render(http.StatusOK, "search", pageTpl{
		PageType: pageSearch,
		Results:  res,
		Query:    &q,
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
			PageType: pageGlossary,
			Initial:  initial,
		})
	}

	// If there's no initial, pick the first one.
	if initial == "" || initial == "*" {
		for _, i := range initials {
			if i == "*" || i == "." {
				continue
			}
			return c.Redirect(http.StatusFound, fmt.Sprintf("/glossary/%s/%s/%s", fromLang, toLang, i))
		}
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
		PageType: pageGlossary,
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
		id  = strings.TrimRight(c.Param("page"), "/")
	)

	tpl, ok := app.sitePageTpls[id]
	if !ok {
		return c.Render(http.StatusNotFound, "message", pageTpl{
			Title:   "404",
			Heading: "Page not found",
		})
	}

	// Render the body.
	b := bytes.Buffer{}

	if err := tpl.ExecuteTemplate(&b, "page-"+id, tplData{
		Path:     c.Path(),
		AssetVer: assetVer,
		Consts:   app.consts,
		Langs:    app.data.Langs,
		Dicts:    app.data.Dicts,
		L:        app.i18n,
		Data: pageTpl{
			PageType: pageStatic,
			PageID:   id,
		},
	}); err != nil {
		return err
	}

	return c.HTMLBlob(http.StatusOK, b.Bytes())
}

// loadSite loads HTML site theme templates and any additional pages (in the `pages/` dir)
// in a map indexed by the page's template name in {{ define "page-$name" }}.
func loadSite(rootPath string, loadPages bool) (*template.Template, map[string]*template.Template, error) {
	theme := template.New("site").Funcs(sprig.FuncMap())

	// Go percentage encodes unicode characters printed in <a href>,
	// but the encoded values are in lowercase hex (for some reason)
	// See: https://github.com/golang/go/issues/33596
	theme.Funcs(template.FuncMap{"UnicodeURL": func(s string) template.URL {
		s = strings.ReplaceAll(s, " ", "+")
		return template.URL(url.PathEscape(s))
	}})

	if _, err := theme.ParseGlob(rootPath + "/*.html"); err != nil {
		return nil, nil, err
	}

	// Load arbitrary pages from (site_dir/pages/*.html).
	// For instance, "about" for site_dir/pages/about.html will be
	// rendered on site.com/pages/about where the template is defined
	// with the name {{ define "page-about" }}. All template name definitions
	// should be "page-*".
	if !loadPages {
		return theme, nil, nil
	}

	pages := make(map[string]*template.Template)
	files, err := filepath.Glob(path.Join(rootPath, "/pages", "*.html"))
	if err != nil {
		return nil, nil, err
	}

	// Iterate through all *.html files in the given directory
	for _, file := range files {
		copy, err := theme.Clone()
		if err != nil {
			return nil, nil, err
		}

		t, err := copy.ParseFiles(file)
		if err != nil {
			return nil, nil, err
		}

		// Get the name of individual templates ({{ define "page-$name" }}.
		name := ""
		for _, t := range t.Templates() {
			if t.Tree != nil && strings.HasPrefix(t.Tree.Name, "page-") {
				name = strings.TrimPrefix(t.Tree.Name, "page-")
				if old, ok := pages[name]; ok {
					return nil, nil, fmt.Errorf("template '%s' in %s already defined in %s", t.Tree.Name, t.Name(), old.Name())
				}
				break
			}
		}

		pages[name] = t
	}

	return theme, pages, nil
}

// Render executes and renders a template for echo.
func (t *tplRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	app := c.Get("app").(*App)

	return t.tpls.ExecuteTemplate(w, name, tplData{
		Path:     c.Path(),
		AssetVer: assetVer,
		Consts:   app.consts,
		Langs:    app.data.Langs,
		Dicts:    app.data.Dicts,
		L:        app.i18n,
		Data:     data,
	})
}
