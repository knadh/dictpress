package main

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

// Results represents a set of results.
type Results struct {
	FromLang string       `json:"-"`
	ToLang   string       `json:"-"`
	Entries  []data.Entry `json:"entries"`

	// Pagination fields.
	paginator.Set
}

// Glossary represents a set of glossary words.
type Glossary struct {
	FromLang string              `json:"-"`
	ToLang   string              `json:"-"`
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
	_, out, err := doSearch(c)
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

// handleGetEntry returns an entry by its guid.
func handleGetEntry(c echo.Context) error {
	var (
		app   = c.Get("app").(*App)
		id, _ = strconv.Atoi(c.Param("id"))
	)

	e, err := app.data.GetEntry(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return echo.NewHTTPError(http.StatusBadRequest, "entry not found")
		}

		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	e.Relations = make(data.Entries, 0)

	entries := data.Entries{e}
	if err := entries.SearchAndLoadRelations(data.Query{}, app.queries.SearchRelations); err != nil {
		app.logger.Printf("error loading relations: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error loading relations")
	}

	return c.JSON(http.StatusOK, okResp{entries[0]})
}

// handleGetParentEntries returns the parent entries of an entry by its guid.
func handleGetParentEntries(c echo.Context) error {
	var (
		app   = c.Get("app").(*App)
		id, _ = strconv.Atoi(c.Param("id"))
	)

	out, err := app.data.GetParentEntries(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// doSearch is a helper function that takes an HTTP query context,
// gets search params from it, performs a search and returns results.
func doSearch(c echo.Context) (data.Query, *Results, error) {
	var (
		app = c.Get("app").(*App)

		fromLang = c.Param("fromLang")
		toLang   = c.Param("toLang")
		q        = strings.TrimSpace(c.Param("q"))

		qp  = c.Request().URL.Query()
		pg  = app.resultsPg.NewFromURL(qp)
		out = &Results{}
	)

	q, err := url.QueryUnescape(q)
	if err != nil {
		return data.Query{}, nil, fmt.Errorf("error parsing query: %v", err)
	}
	q = strings.TrimSpace(q)

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
	out = &Results{
		Entries: data.Entries{},
	}
	res, total, err := app.data.Search(query)
	if err != nil {
		if err == sql.ErrNoRows {
			return query, out, nil
		}

		app.logger.Printf("error querying db: %v", err)
		return query, nil, errors.New("error querying db")
	}

	if len(res) == 0 {
		return query, out, nil
	}

	// Load relations into the matches.
	if err := res.SearchAndLoadRelations(data.Query{
		ToLang: toLang,
		Offset: pg.Offset,
		Limit:  pg.Limit,
		Status: data.StatusEnabled,
	}, app.queries.SearchRelations); err != nil {
		app.logger.Printf("error querying db for defs: %v", err)
		return query, nil, errors.New("error querying db for definitions")
	}

	// Replace nulls with [].
	for i := range res {
		if res[i].Relations == nil {
			res[i].Relations = data.Entries{}
		}
	}

	out.Entries = res

	pg.SetTotal(total)
	out.Page = pg.Page
	out.PerPage = pg.PerPage
	out.TotalPages = pg.TotalPages
	out.Total = total

	return query, out, nil
}

// getGlossaryWords is a helper function that takes an HTTP query context,
// gets params from it and returns a glossary of words for a language.
func getGlossaryWords(lang, initial string, pg paginator.Set, app *App) (*Glossary, error) {
	// HTTP response.
	out := &Glossary{
		Words: []data.GlossaryWord{},
	}

	// Get glossary words.
	res, total, err := app.data.GetGlossaryWords(lang, initial, pg.Offset, pg.Limit)
	if err != nil {
		if err == sql.ErrNoRows {
			return out, nil
		}

		app.logger.Printf("error querying db: %v", err)

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
	if q.Query == "" {
		return errors.New("empty search query")
	}

	for _, t := range q.Types {
		if _, ok := langs[q.FromLang].Types[t]; !ok {
			return fmt.Errorf("unknown type %s", t)
		}
	}

	return nil
}

// basicAuth middleware does an HTTP BasicAuth authentication for admin handlers.
func basicAuth(username, password string, c echo.Context) (bool, error) {
	app := c.Get("app").(*App)

	// Auth is disabled.
	if len(app.constants.AdminUsername) == 0 &&
		len(app.constants.AdminPassword) == 0 {
		return true, nil
	}

	if subtle.ConstantTimeCompare([]byte(username), app.constants.AdminUsername) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), app.constants.AdminPassword) == 1 {
		return true, nil
	}
	return false, nil
}
