package main

import (
	"bytes"
	"database/sql"
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

	Query data.Query `json:"query"`

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
	Data any `json:"data"`
}

// handleSearch performs a search and responds with JSON results.
func handleSearch(c echo.Context) error {
	var (
		app      = c.Get("app").(*App)
		isAuthed = c.Get(isAuthed) != nil
	)

	// Prepare the query.
	query, err := prepareQuery(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	out, err := doSearch(query, isAuthed, app.pgAPI, app)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// handleGetEntryPublic returns an entry by its guid.
func handleGetEntryPublic(c echo.Context) error {
	var (
		app  = c.Get("app").(*App)
		guid = c.Param("guid")
	)

	e, err := app.data.GetEntry(0, guid)
	if err != nil {
		if err == sql.ErrNoRows {
			return echo.NewHTTPError(http.StatusBadRequest, "entry not found")
		}

		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	e.Relations = make([]data.Entry, 0)

	out := []data.Entry{e}
	if err := app.data.SearchAndLoadRelations(out, data.Query{}); err != nil {
		app.lo.Printf("error loading relations: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error loading relations")
	}

	for i := range out {
		out[i].ID = 0

		for j := range out[i].Relations {
			out[i].Relations[j].ID = 0
			out[i].Relations[j].Relation.ID = 0
		}
	}

	return c.JSON(http.StatusOK, okResp{out[0]})
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

// prepareQuery extracts and validates search parameters from the HTTP context.
// Returns a filled data.Query struct ready for searching.
func prepareQuery(c echo.Context) (data.Query, error) {
	var (
		app = c.Get("app").(*App)

		fromLang = c.Param("fromLang")
		toLang   = c.Param("toLang")
		qStr     = strings.TrimSpace(c.Param("q"))
	)

	// Scan query params.
	var q data.Query
	if err := c.Bind(&q); err != nil {
		return data.Query{}, fmt.Errorf("error parsing query: %v", err)
	}

	// Query string from /path/:query
	qStr, err := url.QueryUnescape(qStr)
	if err != nil {
		return data.Query{}, fmt.Errorf("error parsing query: %v", err)
	}
	qStr = strings.TrimSpace(qStr)
	if qStr == "" {
		v, err := url.QueryUnescape(q.Query)
		if err != nil {
			return data.Query{}, fmt.Errorf("error parsing query: %v", err)
		}
		qStr = strings.TrimSpace(v)
	}
	if qStr == "" {
		return data.Query{}, errors.New("no query given")
	}

	// Languages not in path?
	if fromLang == "" {
		fromLang = q.FromLang
	}
	if toLang == "" {
		toLang = q.ToLang
	}

	// Check languages.
	if _, ok := app.data.Langs[fromLang]; !ok {
		return data.Query{}, errors.New("unknown `from` language")
	}
	if toLang == "*" {
		toLang = ""
	} else {
		if _, ok := app.data.Langs[toLang]; !ok {
			return data.Query{}, errors.New("unknown `to` language")
		}
	}

	// Check types.
	for _, t := range q.Types {
		if _, ok := app.data.Langs[fromLang].Types[t]; !ok {
			return data.Query{}, fmt.Errorf("unknown type %s", t)
		}
	}

	// Final query.
	q.Query = qStr
	q.FromLang = fromLang
	q.ToLang = toLang
	q.Status = data.StatusEnabled

	if q.Types == nil {
		q.Types = []string{}
	}
	if q.Tags == nil {
		q.Tags = []string{}
	}

	return q, nil
}

// doSearch takes a prepared query and performs the search, returning results.
func doSearch(q data.Query, isAuthed bool, pgn *paginator.Paginator, app *App) (*results, error) {
	// Pagination.
	pg := pgn.New(q.Page, q.PerPage)
	q.Offset = pg.Offset
	q.Limit = pg.Limit

	// Search and compose results.
	out := &results{Entries: []data.Entry{}}
	res, total, err := app.data.Search(q)
	if err != nil {
		app.lo.Printf("error querying db: %v", err)
		return nil, errors.New("error querying db")
	}

	if len(res) == 0 {
		out.Query = q

		return out, nil
	}

	// Load relations into the matches.
	if err := app.data.SearchAndLoadRelations(res, q); err != nil {
		return nil, errors.New("error querying db for definitions")
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

	// Calculate pagination.
	pg.SetTotal(total)

	out.Query = q
	out.Entries = res
	out.Page = pg.Page
	out.PerPage = pg.PerPage
	out.TotalPages = pg.TotalPages
	out.Total = total

	return out, nil
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
