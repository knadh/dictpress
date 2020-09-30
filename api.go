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
	"github.com/knadh/dictmaker/search"
	"github.com/knadh/paginator"
)

const (
	entriesPerPage = 20
	// glossaryPerPage = 100
)

// Results represents a set of results.
type Results struct {
	FromLang string         `json:"-"`
	ToLang   string         `json:"-"`
	Entries  []search.Entry `json:"entries"`

	// Pagination fields.
	paginator.Set
}

// Glossary represents a set of glossary words.
type Glossary struct {
	FromLang string                `json:"-"`
	ToLang   string                `json:"-"`
	Words    []search.GlossaryWord `json:"entries"`

	// Pagination fields.
	paginator.Set
}

// pagination represents a query's pagination (limit, offset) related values.
type pagination struct {
	PerPage int `json:"per_page"`
	Page    int `json:"page"`
	Offset  int `json:"offset"`
	Limit   int `json:"limit"`
	Total   int `json:"total"`

	PageStart int   `json:"-"`
	Pages     []int `json:"-"`
	PageEnd   int   `json:"-"`
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

	sendResponse(okResp{out}, http.StatusOK, w)
}

// doSearch is a helper function that takes an HTTP query context,
// gets search params from it, performs a search and returns results.
func doSearch(r *http.Request, app *App) (search.Query, *Results, error) {
	var (
		fromLang = chi.URLParam(r, "fromLang")
		toLang   = chi.URLParam(r, "toLang")
		q        = strings.TrimSpace(chi.URLParam(r, "q"))

		qp  = r.URL.Query()
		pg  = getPagination(qp, entriesPerPage, entriesPerPage)
		out = &Results{}
	)

	if q == "" {
		q = qp.Get("q")
	}

	q, err := url.QueryUnescape(q)
	if err != nil {
		return search.Query{}, nil, fmt.Errorf("error parsing query: %v", err)
	}

	q = strings.TrimSpace(q)

	if _, ok := app.lang[fromLang]; !ok {
		return search.Query{}, nil, errors.New("unknown `from` language")
	}

	if _, ok := app.lang[toLang]; !ok {
		return search.Query{}, nil, errors.New("unknown `to` language")
	}

	// Search query.
	query := search.Query{
		FromLang:      fromLang,
		ToLang:        toLang,
		Types:         qp["type"],
		Tags:          qp["tag"],
		TokenizerName: app.lang[fromLang].TokenizerName,
		Tokenizer:     app.lang[fromLang].Tokenizer,
		Query:         q,
		Offset:        pg.Offset,
		Limit:         pg.Limit,
	}

	if err = validateSearchQuery(query, app.lang); err != nil {
		return query, out, err
	}

	// HTTP response.
	out = &Results{}
	out.Page = pg.Page
	out.PerPage = pg.PerPage
	out.Entries = search.Entries{}

	// Find entries matching the query.
	res, total, err := app.search.FindEntries(query)
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
	if err := res.LoadRelations(search.Query{
		ToLang: toLang,
		Offset: pg.Offset,
		Limit:  pg.Limit,
	}, app.queries.GetRelations); err != nil {
		app.logger.Printf("error querying db for defs: %v", err)
		return query, nil, errors.New("error querying db for definitions")
	}

	// Replace nulls with [].
	for i := range res {
		if res[i].Relations == nil {
			res[i].Relations = search.Entries{}
		}
	}

	out.Total = total
	out.Entries = res

	return query, out, nil
}

// getGlossaryWords is a helper function that takes an HTTP query context,
// gets params from it and returns a glossary of words for a language.
func getGlossaryWords(lang, initial string, pg paginator.Set, app *App) (*Glossary, error) {
	// HTTP response.
	out := &Glossary{}
	out.Page = pg.Page
	out.PerPage = pg.PerPage
	out.Words = []search.GlossaryWord{}

	// Get glossary words.
	res, total, err := app.search.GetGlossaryWords(lang, initial, pg.Offset, pg.Limit)
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

	out.Total = total
	out.Words = res

	return out, nil
}

// getPagination takes form values and extracts pagination values from it.
func getPagination(q url.Values, defaultPerPage, maxPerPage int) pagination {
	var (
		perPage, _ = strconv.Atoi(q.Get("per_page"))
		page, _    = strconv.Atoi(q.Get("page"))
	)

	if perPage < 1 || perPage > defaultPerPage {
		perPage = maxPerPage
	}

	if page < 1 {
		page = 0
	} else {
		page--
	}

	return pagination{
		Page:    page + 1,
		PerPage: perPage,
		Offset:  page * perPage,
		Limit:   perPage,
	}
}

func (p *pagination) GenerateNumbers() {
	if p.Total <= p.PerPage {
		return
	}

	var (
		// Page divisor.
		div      = p.Total / p.PerPage
		divStart = 1
		hints    = 0
	)

	if p.Total%p.PerPage == 0 {
		div = div - 1
	}

	div++

	if div > 10 {
		hints = div
		div = 10
	}

	// Generate the page numbers
	if p.Page >= 10 {
		divStart = p.PerPage - 5
		div = divStart + 15
	}

	if (div * p.PerPage) > (p.Total + p.PerPage) {
		div = (p.Total) / p.PerPage
	}

	// If the page number has exceeded the limit, fix the first to
	// print to 1.
	if p.Page >= 10 {
		p.PageStart = 1
	} else {
		p.PageStart = 1
	}

	if hints-10 > p.Page {
		p.PageEnd = hints
	}
}

// validateSearchQuery does basic validation and sanity checks
// on search.Query (useful for params coming from the outside world).
func validateSearchQuery(q search.Query, l Languages) error {
	if q.Query == "" {
		return errors.New("empty search query")
	}

	if _, ok := l[q.FromLang]; !ok {
		return fmt.Errorf("unknown language %s", q.FromLang)
	}

	if _, ok := l[q.ToLang]; !ok {
		return fmt.Errorf("unknown language %s", q.ToLang)
	}

	for _, t := range q.Types {
		if _, ok := l[q.FromLang].Types[t]; !ok {
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
