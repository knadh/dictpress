package main

import (
	"bytes"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"github.com/labstack/echo/v4"
)

const isAuthed = "is_authed"

// handleGetConfig returns the language configuration.
func handleGetConfig(c echo.Context) error {
	var (
		app = c.Get("app").(*App)
	)

	out := struct {
		RootURL   string       `json:"root_url"`
		Languages data.LangMap `json:"languages"`
		Version   string       `json:"version"`
		BuildStr  string       `json:"build"`
	}{app.constants.RootURL, app.data.Langs, versionString, buildString}

	return c.JSON(http.StatusOK, okResp{out})
}

// handleGetStats returns DB statistics.
func handleGetStats(c echo.Context) error {
	var (
		app = c.Get("app").(*App)
	)

	out, err := app.data.GetStats()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// handleInsertEntry inserts a new dictionary entry.
func handleInsertEntry(c echo.Context) error {
	app := c.Get("app").(*App)

	var e data.Entry
	if err := c.Bind(&e); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if err := validateEntry(e, app); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	id, err := app.data.InsertEntry(e)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error inserting entry: %v", err))
	}

	// Proxy to the get request to respond with the newly inserted entry.
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", id))
	return handleGetEntry(c)
}

// handleGetPendingEntries returns the pending entries for moderation.
func handleGetPendingEntries(c echo.Context) error {
	var (
		app = c.Get("app").(*App)

		pg = app.resultsPg.NewFromURL(c.Request().URL.Query())
	)

	// Search and compose results.
	out := &results{
		Entries: []data.Entry{},
	}
	res, total, err := app.data.GetPendingEntries("", nil, pg.Offset, pg.Limit)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, okResp{out})
		}

		app.lo.Printf("error querying db: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if len(res) == 0 {
		return c.JSON(http.StatusOK, okResp{out})
	}

	// Load relations into the matches.
	if err := app.data.SearchAndLoadRelations(res, data.Query{}); err != nil {
		app.lo.Printf("error querying db for defs: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	pg.SetTotal(total)

	out.Entries = res
	out.Page = pg.Page
	out.PerPage = pg.PerPage
	out.TotalPages = pg.TotalPages
	out.Total = total

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

	e.Relations = make([]data.Entry, 0)

	entries := []data.Entry{e}
	if err := app.data.SearchAndLoadRelations(entries, data.Query{}); err != nil {
		app.lo.Printf("error loading relations: %v", err)
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

	if out == nil {
		out = []data.Entry{}
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// handleUpdateEntry updates a dictionary entry.
func handleUpdateEntry(c echo.Context) error {
	var (
		app   = c.Get("app").(*App)
		id, _ = strconv.Atoi(c.Param("id"))
	)

	if id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	var e data.Entry
	if err := c.Bind(&e); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if err := app.data.UpdateEntry(id, e); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error updating entry: %v", err))
	}

	// Proxy to the get request to respond with the newly inserted entry.
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", id))
	return handleGetEntry(c)
}

// handleApproveSubmission updates a dictionary entry.
func handleApproveSubmission(c echo.Context) error {
	var (
		app   = c.Get("app").(*App)
		id, _ = strconv.Atoi(c.Param("id"))
	)

	if id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	if err := app.data.ApproveSubmission(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error approving submission: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// handleRejectSubmission updates a dictionary entry.
func handleRejectSubmission(c echo.Context) error {
	var (
		app   = c.Get("app").(*App)
		id, _ = strconv.Atoi(c.Param("id"))
	)

	if id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	if err := app.data.RejectSubmission(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error rejecting submission: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// handleDeleteEntry deletes a dictionary entry.
func handleDeleteEntry(c echo.Context) error {
	var (
		app   = c.Get("app").(*App)
		id, _ = strconv.Atoi(c.Param("id"))
	)

	if err := app.data.DeleteEntry(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting entry: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// handleAddRelation updates a relation's properties.
func handleAddRelation(c echo.Context) error {
	var (
		app       = c.Get("app").(*App)
		fromID, _ = strconv.Atoi(c.Param("fromID"))
		toID, _   = strconv.Atoi(c.Param("toID"))
	)

	if fromID < 1 || toID < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid IDs.")
	}

	var rel data.Relation
	if err := c.Bind(&rel); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if _, err := app.data.InsertRelation(fromID, toID, rel); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error inserting relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// handleUpdateRelation updates a relation's properties.
func handleUpdateRelation(c echo.Context) error {
	var (
		app      = c.Get("app").(*App)
		relID, _ = strconv.Atoi(c.Param("relID"))
	)

	if relID < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	var rel data.Relation
	if err := c.Bind(&rel); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if err := app.data.UpdateRelation(relID, rel); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error updating relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// handleReorderRelations reorders the weights of the relation IDs in the given order.
func handleReorderRelations(c echo.Context) error {
	var (
		app = c.Get("app").(*App)
	)

	req := struct {
		IDs []int `json:"ids"`
	}{}

	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if err := app.data.ReorderRelations(req.IDs); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error updating relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// handleDeleteRelation deletes a relation between two entres.
func handleDeleteRelation(c echo.Context) error {
	var (
		app       = c.Get("app").(*App)
		fromID, _ = strconv.Atoi(c.Param("fromID"))
		relID, _  = strconv.Atoi(c.Param("relID"))
	)

	if fromID < 1 || relID < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid IDs.")
	}

	if err := app.data.DeleteRelation(fromID, relID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

func handleGetComments(c echo.Context) error {
	var (
		app = c.Get("app").(*App)
	)

	out, err := app.data.GetComments()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{out})
}

func handleDeletecomments(c echo.Context) error {
	var (
		app   = c.Get("app").(*App)
		id, _ = strconv.Atoi(c.Param("commentID"))
	)

	if id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	if err := app.data.DeleteComments(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting comments: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

func validateEntry(e data.Entry, app *App) error {
	if strings.TrimSpace(e.Content) == "" {
		return errors.New("invalid `content`.")
	}

	if strings.TrimSpace(e.Initial) == "" {
		return errors.New("invalid `initial`.")
	}

	if _, ok := app.data.Langs[e.Lang]; !ok {
		return errors.New("unknown `lang`.")
	}

	return nil
}

// handleAdminPage is the root handler that renders the Javascript admin frontend.
func adminPage(tpl string) func(c echo.Context) error {
	return func(c echo.Context) error {
		app := c.Get("app").(*App)

		title := ""
		switch tpl {
		case "search":
			var (
				q  = c.Request().URL.Query().Get("query")
				id = c.Request().URL.Query().Get("id")
			)

			title = "Search"
			if q != "" {
				title = fmt.Sprintf("Search '%s'", q)
			} else if id != "" {
				title = fmt.Sprintf("Entry #%s", id)
			}

		case "pending":
			title = "Pending submissions"
		}

		b := &bytes.Buffer{}
		err := app.adminTpl.ExecuteTemplate(b, tpl, struct {
			Title string
		}{title})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				fmt.Sprintf("error compiling template: %v", err))
		}

		return c.HTMLBlob(http.StatusOK, b.Bytes())
	}
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
		c.Set(isAuthed, true)
		return true, nil
	}

	return false, nil
}
