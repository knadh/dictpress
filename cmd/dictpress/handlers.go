package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

// results represents a set of results.
type results struct {
	Entries []data.Entry `json:"entries"`

	Query struct {
		Query    string   `json:"query"`
		FromLang string   `json:"from_lang"`
		ToLang   string   `json:"to_lang"`
		Types    []string `json:"types"`
		Tags     []string `json:"tags"`
	} `json:"query"`

	// Pagination fields.
	paginator.Set
}

// glossary represents a set of glossary words.
type glossary struct {
	FromLang string              `json:"from_lang"`
	ToLang   string              `json:"to_lang"`
	Words    []data.GlossaryWord `json:"entries"`

	// Pagination fields.
	paginator.Set
}

// okResp represents the HTTP response wrapper.
type okResp struct {
	Data interface{} `json:"data"`
}

type httpResp struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// handleSearch performs a search and responds with JSON results.
func handleSearch(c echo.Context) error {
	isAuthed := c.Get(isAuthed) != nil

	_, out, err := doSearch(c, isAuthed)
	if err != nil {
		var s int

		// If out is nil, it's a non 500 "soft" error.
		if out != nil {
			s = http.StatusBadRequest
		} else {
			s = http.StatusInternalServerError
		}

		return echo.NewHTTPError(s, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// handleServeBundle serves concatenated JS or CSS files based on query parameters
func handleServeBundle(c echo.Context, bundleType string, staticDir string) error {
	files := c.QueryParams()["f"]
	if len(files) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no files specified")
	}

	var (
		contentType string
		ext         string
	)

	switch bundleType {
	case "js":
		contentType = "application/javascript"
		ext = ".js"
	case "css":
		contentType = "text/css"
		ext = ".css"
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid bundle type")
	}

	// Build the combined buf
	var buf bytes.Buffer
	for _, fname := range files {
		if filepath.Ext(fname) != ext {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid file extension")
		}

		fname = filepath.Clean(fname)
		if strings.Contains(fname, "..") {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid filename")
			continue
		}

		fullPath := filepath.Join(staticDir, fname)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("file not found: %s", fname))
		}

		buf.Write(data)
		buf.WriteString("\n")
	}

	return c.Blob(http.StatusOK, contentType, buf.Bytes())
}

// doSearch is a helper function that takes an HTTP query context,
// gets search params from it, performs a search and returns results.
func doSearch(c echo.Context, isAuthed bool) (data.Query, *results, error) {
	var (
		app = c.Get("app").(*App)

		fromLang = c.Param("fromLang")
		toLang   = c.Param("toLang")
		q        = strings.TrimSpace(c.Param("q"))

		qp  = c.Request().URL.Query()
		pg  = app.resultsPg.NewFromURL(qp)
		out = &results{}
	)

	// Query from /path/:query
	q, err := url.QueryUnescape(q)
	if err != nil {
		return data.Query{}, nil, fmt.Errorf("error parsing query: %v", err)
	}
	q = strings.TrimSpace(q)
	if q == "" {
		v, err := url.QueryUnescape(qp.Get("q"))
		if err != nil {
			return data.Query{}, nil, fmt.Errorf("error parsing query: %v", err)
		}
		q = strings.TrimSpace(v)
	}

	if q == "" {
		return data.Query{}, nil, errors.New("no query given")
	}

	if _, ok := app.data.Langs[fromLang]; !ok {
		return data.Query{}, nil, errors.New("unknown `from` language")
	}

	if toLang == "*" {
		toLang = ""
	} else {
		if _, ok := app.data.Langs[toLang]; !ok {
			return data.Query{}, nil, errors.New("unknown `to` language")
		}
	}

	// Search query.
	query := data.Query{
		FromLang: fromLang,
		ToLang:   toLang,
		Types:    qp["type"],
		Tags:     qp["tag"],
		Query:    q,
		Status:   data.StatusEnabled,
		Offset:   pg.Offset,
		Limit:    pg.Limit,
	}

	if err = validateSearchQuery(query, app.data.Langs); err != nil {
		return query, out, err
	}

	// Search and compose results.
	out = &results{
		Entries: []data.Entry{},
	}
	res, total, err := app.data.Search(query)
	if err != nil {
		app.lo.Printf("error querying db: %v", err)
		return query, nil, errors.New("error querying db")
	}

	if len(res) == 0 {
		return query, out, nil
	}

	// Load relations into the matches.
	if err := app.data.SearchAndLoadRelations(res, data.Query{
		ToLang: toLang,
		Offset: pg.Offset,
		Limit:  pg.Limit,
		Status: data.StatusEnabled,
	}); err != nil {
		app.lo.Printf("error querying db for defs: %v", err)
		return query, nil, errors.New("error querying db for definitions")
	}

	// If this is an un-authenticated query, hide the numerical IDs.
	if !isAuthed {
		for i := range res {
			res[i].ID = 0

			for j := range res[i].Relations {
				res[i].Relations[j].ID = 0
				res[i].Relations[j].Relation.ID = 0
			}
		}
	}

	pg.SetTotal(total)

	out.Query.FromLang = fromLang
	out.Query.ToLang = toLang
	out.Query.Types = qp["type"]
	out.Query.Tags = qp["tag"]
	out.Query.Query = q

	if out.Query.Types == nil {
		out.Query.Types = []string{}
	}
	if out.Query.Tags == nil {
		out.Query.Tags = []string{}
	}

	out.Entries = res
	out.Page = pg.Page
	out.PerPage = pg.PerPage
	out.TotalPages = pg.TotalPages
	out.Total = total

	return query, out, nil
}

// getGlossaryWords is a helper function that takes an HTTP query context,
// gets params from it and returns a glossary of words for a language.
func getGlossaryWords(lang, initial string, pg paginator.Set, app *App) (*glossary, error) {
	// HTTP response.
	out := &glossary{
		Words: []data.GlossaryWord{},
	}

	// Get glossary words.
	res, total, err := app.data.GetGlossaryWords(lang, initial, pg.Offset, pg.Limit)
	if err != nil {
		app.lo.Printf("error querying db: %v", err)
		return nil, errors.New("error querying db")
	}

	if len(res) == 0 {
		return out, nil
	}

	out.Words = res

	pg.SetTotal(total)
	out.Page = pg.Page
	out.PerPage = pg.PerPage
	out.TotalPages = pg.TotalPages
	out.Total = total

	return out, nil
}

// validateSearchQuery does basic validation and sanity checks
// on data.Query (useful for params coming from the outside world).
func validateSearchQuery(q data.Query, langs data.LangMap) error {
	for _, t := range q.Types {
		if _, ok := langs[q.FromLang].Types[t]; !ok {
			return fmt.Errorf("unknown type %s", t)
		}
	}

	return nil
}
