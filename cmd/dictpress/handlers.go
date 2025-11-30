package main

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

func initHTTPServer(a *App, ko *koanf.Koanf) *echo.Echo {
	srv := echo.New()
	srv.Debug = true
	srv.HideBanner = true

	var (
		// Public handlers with no auth.
		pb = srv.Group("")

		// Admin handlers with auth.
		ad = srv.Group("", middleware.BasicAuth(a.basicAuth))
	)

	// Dictionary site HTML views.
	if a.consts.Site != "" {
		pb.GET("/", a.handleIndexPage)
		pb.GET("/dictionary/:fromLang/:toLang/:q", a.handleSearchPage)
		pb.GET("/dictionary/:fromLang/:toLang", a.handleSearchPage)
		pb.GET("/p/:page", a.handleStaticPage)

		if a.consts.EnableGlossary {
			pb.GET("/glossary/:fromLang/:toLang/:initial", a.handleGlossaryPage)
		}

		// Static files with custom bundle handling
		srv.GET("/static/*", func(c echo.Context) error {
			staticDir := filepath.Join(a.consts.Site, "static")

			switch c.Param("*") {
			case "_bundle.js":
				return handleServeBundle(c, "js", staticDir)
			case "_bundle.css":
				return handleServeBundle(c, "css", staticDir)
			default:
				// Normal static file serving
				fs := http.StripPrefix("/static", http.FileServer(http.Dir(staticDir)))
				return echo.WrapHandler(fs)(c)
			}
		})

	} else {
		// API greeting if there's no site.
		pb.GET("/", func(c echo.Context) error {
			return c.JSON(http.StatusOK, okResp{"welcome"})
		})
	}

	// Public APIs.
	pb.GET("/api/config", a.HandleGetConfig)
	pb.GET("/api/dictionary/:fromLang/:toLang/:q", a.HandleSearch)
	pb.GET("/api/dictionary/entries/:guid", a.HandleGetEntryPublic)

	// Public user submission APIs.
	if ko.Bool("app.enable_submissions") {
		pb.POST("/api/submissions", a.HandleNewSubmission)
		pb.POST("/api/submissions/comments", a.HandleNewComments)

		if a.consts.Site != "" {
			pb.GET("/submit", a.HandleSubmissionPage)
			pb.POST("/submit", a.HandleSubmissionPage)
		}
	}

	// Admin handlers and APIs.
	ad.GET("/api/entries/:fromLang/:toLang", a.HandleSearch)
	ad.GET("/api/entries/:fromLang/:toLang/:q", a.HandleSearch)
	ad.GET("/admin/static/*", echo.WrapHandler(a.fs.FileServer()))
	ad.GET("/admin", a.adminPage("index"))
	ad.GET("/admin/search", a.adminPage("search"))
	ad.GET("/admin/pending", a.adminPage("pending"))

	ad.GET("/api/stats", a.HandleGetStats)
	ad.GET("/api/entries/pending", a.HandleGetPendingEntries)
	ad.GET("/api/entries/comments", a.HandleGetComments)
	ad.DELETE("/api/entries/comments/:commentID", a.HandleDeleteComments)
	ad.DELETE("/api/entries/pending", a.HandleDeletePending)
	ad.GET("/api/entries/:id", a.HandleGetEntry)
	ad.GET("/api/entries/:id/parents", a.HandleGetParentEntries)
	ad.POST("/api/entries", a.HandleInsertEntry)
	ad.PUT("/api/entries/:id", a.HandleUpdateEntry)
	ad.DELETE("/api/entries/:id", a.HandleDeleteEntry)
	ad.DELETE("/api/entries/:fromID/relations/:relID", a.HandleDeleteRelation)
	ad.POST("/api/entries/:fromID/relations/:toID", a.HandleAddRelation)
	ad.PUT("/api/entries/:id/relations/weights", a.HandleReorderRelations)
	ad.PUT("/api/entries/:id/relations/:relID", a.HandleUpdateRelation)
	ad.PUT("/api/entries/:id/submission", a.HandleApproveSubmission)
	ad.DELETE("/api/entries/:id/submission", a.HandleRejectSubmission)

	// 404 pages.
	srv.RouteNotFound("/api/*", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusNotFound, "Unknown endpoint")
	})
	srv.RouteNotFound("/*", func(c echo.Context) error {
		return c.Render(http.StatusNotFound, "message", pageTpl{
			Title:   "404 Page not found",
			Heading: "404 Page not found",
		})
	})

	return srv
}

// HandleSearch performs a search and responds with JSON results.
func (a *App) HandleSearch(c echo.Context) error {
	isAuthed := c.Get(isAuthed) != nil

	// Prepare the query.
	query, err := a.prepareQuery(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	out, err := a.doSearch(query, isAuthed, a.pgAPI)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// HandleGetEntryPublic returns an entry by its guid.
func (a *App) HandleGetEntryPublic(c echo.Context) error {
	guid := c.Param("guid")

	e, err := a.data.GetEntry(0, guid)
	if err != nil {
		if err == sql.ErrNoRows {
			return echo.NewHTTPError(http.StatusBadRequest, "entry not found")
		}

		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	e.Relations = make([]data.Entry, 0)

	out := []data.Entry{e}
	if err := a.data.SearchAndLoadRelations(out, data.Query{}); err != nil {
		a.lo.Printf("error loading relations: %v", err)
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
func (a *App) prepareQuery(c echo.Context) (data.Query, error) {
	var (
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
	if _, ok := a.data.Langs[fromLang]; !ok {
		return data.Query{}, errors.New("unknown `from` language")
	}
	if toLang == "*" {
		toLang = ""
	} else {
		if _, ok := a.data.Langs[toLang]; !ok {
			return data.Query{}, errors.New("unknown `to` language")
		}
	}

	// Check types.
	for _, t := range q.Types {
		if _, ok := a.data.Langs[fromLang].Types[t]; !ok {
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
func (a *App) doSearch(q data.Query, isAuthed bool, pgn *paginator.Paginator) (*results, error) {
	// Pagination.
	pg := pgn.New(q.Page, q.PerPage)
	q.Offset = pg.Offset
	q.Limit = pg.Limit

	// Results container.
	out := &results{Entries: []data.Entry{}}

	// Is result caching enabled (for public, unauthenticated requests)?
	cacheKey := ""
	if a.cache != nil && !isAuthed {
		cacheKey = makeQueryCacheKey(q)
		if cached, _ := a.cache.Get(cacheKey); cached != nil {
			var out results
			if gobDecode(cached, &out) == nil {
				return &out, nil
			}
		}
	}

	// Search and compose results.
	res, total, err := a.data.Search(q)
	if err != nil {
		a.lo.Printf("error querying db: %v", err)
		return nil, errors.New("error querying db")
	}

	if len(res) == 0 {
		out.Query = q

		return out, nil
	}

	// Load relations into the matches.
	if err := a.data.SearchAndLoadRelations(res, q); err != nil {
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

	// Cache public results.
	if a.cache != nil && !isAuthed {
		if b, err := gobEncode(out); err == nil {
			a.cache.Put(cacheKey, b, nil)
		}
	}

	return out, nil
}

// getGlossaryWords is a helper function that takes an HTTP query context,
// gets params from it and returns a glossary of words for a language.
func (a *App) getGlossaryWords(lang, initial string, pg paginator.Set) (glossary, error) {
	// HTTP response.
	out := glossary{
		Words: []data.GlossaryWord{},
	}

	// Is result caching enabled?
	cacheKey := ""
	if a.cache != nil {
		cacheKey = makeGlossaryCacheKey(lang, initial, pg.Offset, pg.Limit)
		if cached, _ := a.cache.Get(cacheKey); cached != nil {
			if gobDecode(cached, &out) == nil {
				return out, nil
			}
		}
	}

	// Get glossary words.
	res, total, err := a.data.GetGlossaryWords(lang, initial, pg.Offset, pg.Limit)
	if err != nil {
		a.lo.Printf("error querying db: %v", err)
		return out, errors.New("error querying db")
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

	// Cache results.
	if a.cache != nil {
		if b, err := gobEncode(out); err == nil {
			a.cache.Put(cacheKey, b, nil)
		}
	}

	return out, nil
}

// makeQueryCacheKey creates a deterministic cache key from a Query.
// Normalizes and sorts fields in the query to generate consistent keys.
func makeQueryCacheKey(q data.Query) string {
	// Sort slices for deterministic ordering.
	types := make([]string, len(q.Types))
	copy(types, q.Types)
	sort.Strings(types)

	tags := make([]string, len(q.Tags))
	copy(tags, q.Tags)
	sort.Strings(tags)

	// Build key string with all the fields.
	key := fmt.Sprintf("s:%s:%s:%s:%s:%s:%s:%d:%d",
		q.FromLang,
		q.ToLang,
		strings.ToLower(strings.TrimSpace(q.Query)),
		strings.Join(types, ","),
		strings.Join(tags, ","),
		q.Status,
		q.Page,
		q.PerPage,
	)

	h := md5.Sum([]byte(key))
	return "s:" + hex.EncodeToString(h[:])
}

// makeGlossaryCacheKey creates a deterministic key for glossary queries.
func makeGlossaryCacheKey(lang, initial string, offset, limit int) string {
	key := fmt.Sprintf("g:%s:%s:%d:%d", lang, initial, offset, limit)
	h := md5.Sum([]byte(key))
	return "g:" + hex.EncodeToString(h[:])
}

// gobEncode encodes a given object using gob.
func gobEncode(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gobDecode decodes gob-encoded data into the provided value.
func gobDecode(data []byte, v any) error {
	return gob.NewDecoder(bytes.NewReader(data)).Decode(v)
}
