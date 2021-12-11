package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/knadh/dictmaker/internal/data"
	"github.com/knadh/paginator"
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
func handleSearch(w http.ResponseWriter, r *http.Request) {
	app, _ := r.Context().Value("app").(*App)

	_, out, err := doSearch(r, app)
	if err != nil {
		var s int

		// If out is nil, it's a non 500 "soft" error.
		if out != nil {
			s = http.StatusBadRequest
		} else {
			s = http.StatusInternalServerError
		}

		sendErrorResponse(err.Error(), s, nil, w)
		return
	}

	sendResponse(out, http.StatusOK, w)
}

// handleGetEntry returns an entry by its guid.
func handleGetEntry(w http.ResponseWriter, r *http.Request) {
	var (
		app   = r.Context().Value("app").(*App)
		id, _ = strconv.Atoi(chi.URLParam(r, "id"))
	)

	e, err := app.data.GetEntry(id)
	if err != nil {
		if err == sql.ErrNoRows {
			sendErrorResponse("entry not found", http.StatusBadRequest, nil, w)
			return
		}

		sendErrorResponse(err.Error(), http.StatusInternalServerError, nil, w)
		return
	}

	e.Relations = make(data.Entries, 0)

	entries := data.Entries{e}
	if err := entries.SearchAndLoadRelations(data.Query{}, app.queries.SearchRelations); err != nil {
		app.logger.Printf("error querying db for defs: %v", err)
		sendErrorResponse(fmt.Sprintf("error loading relations: %v", err), http.StatusBadRequest, nil, w)
	}

	sendResponse(entries[0], http.StatusOK, w)
}

// handleGetParentEntries returns the parent entries of an entry by its guid.
func handleGetParentEntries(w http.ResponseWriter, r *http.Request) {
	var (
		app   = r.Context().Value("app").(*App)
		id, _ = strconv.Atoi(chi.URLParam(r, "id"))
	)

	out, err := app.data.GetParentEntries(id)
	if err != nil {
		sendErrorResponse(err.Error(), http.StatusInternalServerError, nil, w)
		return
	}

	sendResponse(out, http.StatusOK, w)
}

// doSearch is a helper function that takes an HTTP query context,
// gets search params from it, performs a search and returns results.
func doSearch(r *http.Request, app *App) (data.Query, *Results, error) {
	var (
		fromLang = chi.URLParam(r, "fromLang")
		toLang   = chi.URLParam(r, "toLang")
		q        = strings.TrimSpace(chi.URLParam(r, "q"))

		qp  = r.URL.Query()
		pg  = app.resultsPg.NewFromURL(r.URL.Query())
		out = &Results{}
	)

	if q == "" {
		q = qp.Get("q")
	}

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

// sendResponse sends a JSON envelope to the HTTP response.
func sendResponse(data interface{}, status int, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	out, err := json.Marshal(httpResp{Status: "success", Data: data})
	if err != nil {
		sendErrorResponse("Internal Server Error", http.StatusInternalServerError, nil, w)
		return
	}

	_, _ = w.Write(out)
}

// sendTpl executes a template and writes the results to the HTTP response.
func sendTpl(status int, tplName string, tpl *template.Template, data interface{}, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	_ = tpl.ExecuteTemplate(w, tplName, data)
}

// sendErrorResponse sends a JSON error envelope to the HTTP response.
func sendErrorResponse(message string, status int, data interface{}, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	resp := httpResp{Status: "error",
		Message: message,
		Data:    data}

	out, _ := json.Marshal(resp)

	_, _ = w.Write(out)
}
