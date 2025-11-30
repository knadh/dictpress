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

// HandleGetConfig returns the language configuration.
func (a *App) HandleGetConfig(c echo.Context) error {
	out := struct {
		RootURL   string       `json:"root_url"`
		Languages data.LangMap `json:"languages"`
		Version   string       `json:"version"`
		BuildStr  string       `json:"build"`
	}{a.consts.RootURL, a.data.Langs, versionString, buildString}

	return c.JSON(http.StatusOK, okResp{out})
}

// HandleGetStats returns DB statistics.
func (a *App) HandleGetStats(c echo.Context) error {
	out, err := a.data.GetStats()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// HandleInsertEntry inserts a new dictionary entry.
func (a *App) HandleInsertEntry(c echo.Context) error {
	var e data.Entry
	if err := c.Bind(&e); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	e, err := a.validateEntry(e)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if len(e.Meta) == 0 {
		e.Meta = map[string]interface{}{}
	}

	id, err := a.data.InsertEntry(e)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error inserting entry: %v", err))
	}

	// Proxy to the get request to respond with the newly inserted entry.
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", id))
	return a.HandleGetEntry(c)
}

// HandleGetPendingEntries returns the pending entries for moderation.
func (a *App) HandleGetPendingEntries(c echo.Context) error {
	pg := a.pgSite.NewFromURL(c.Request().URL.Query())

	// Search and compose results.
	out := &results{
		Entries: []data.Entry{},
	}
	res, total, err := a.data.GetPendingEntries("", nil, pg.Offset, pg.Limit)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, okResp{out})
		}

		a.lo.Printf("error querying db: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if len(res) == 0 {
		return c.JSON(http.StatusOK, okResp{out})
	}

	// Load relations into the matches.
	if err := a.data.SearchAndLoadRelations(res, data.Query{}); err != nil {
		a.lo.Printf("error querying db for defs: %v", err)
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

// HandleGetEntry returns an entry by its guid.
func (a *App) HandleGetEntry(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	e, err := a.data.GetEntry(id, "")
	if err != nil {
		if err == sql.ErrNoRows {
			return echo.NewHTTPError(http.StatusBadRequest, "entry not found")
		}

		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	e.Relations = make([]data.Entry, 0)

	entries := []data.Entry{e}
	if err := a.data.SearchAndLoadRelations(entries, data.Query{}); err != nil {
		a.lo.Printf("error loading relations: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error loading relations")
	}

	return c.JSON(http.StatusOK, okResp{entries[0]})
}

// HandleGetParentEntries returns the parent entries of an entry by its guid.
func (a *App) HandleGetParentEntries(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	out, err := a.data.GetParentEntries(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if out == nil {
		out = []data.Entry{}
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// HandleUpdateEntry updates a dictionary entry.
func (a *App) HandleUpdateEntry(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	if id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	var e data.Entry
	if err := c.Bind(&e); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	e, err := a.validateEntry(e)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := a.data.UpdateEntry(id, e); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error updating entry: %v", err))
	}

	// Proxy to the get request to respond with the newly inserted entry.
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", id))
	return a.HandleGetEntry(c)
}

// HandleApproveSubmission updates a dictionary entry.
func (a *App) HandleApproveSubmission(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	if id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	if err := a.data.ApproveSubmission(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error approving submission: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// HandleRejectSubmission updates a dictionary entry.
func (a *App) HandleRejectSubmission(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	if id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	if err := a.data.RejectSubmission(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error rejecting submission: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// HandleDeleteEntry deletes a dictionary entry.
func (a *App) HandleDeleteEntry(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	if err := a.data.DeleteEntry(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting entry: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// HandleAddRelation updates a relation's properties.
func (a *App) HandleAddRelation(c echo.Context) error {
	fromID, _ := strconv.Atoi(c.Param("fromID"))
	toID, _ := strconv.Atoi(c.Param("toID"))

	if fromID < 1 || toID < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid IDs.")
	}

	var rel data.Relation
	if err := c.Bind(&rel); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if _, err := a.data.InsertRelation(fromID, toID, rel); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error inserting relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// HandleUpdateRelation updates a relation's properties.
func (a *App) HandleUpdateRelation(c echo.Context) error {
	relID, _ := strconv.Atoi(c.Param("relID"))

	if relID < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	var rel data.Relation
	if err := c.Bind(&rel); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if err := a.data.UpdateRelation(relID, rel); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error updating relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// HandleReorderRelations reorders the weights of the relation IDs in the given order.
func (a *App) HandleReorderRelations(c echo.Context) error {
	req := struct {
		IDs []int `json:"ids"`
	}{}

	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if err := a.data.ReorderRelations(req.IDs); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error updating relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// HandleDeleteRelation deletes a relation between two entres.
func (a *App) HandleDeleteRelation(c echo.Context) error {
	fromID, _ := strconv.Atoi(c.Param("fromID"))
	relID, _ := strconv.Atoi(c.Param("relID"))

	if fromID < 1 || relID < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid IDs.")
	}

	if err := a.data.DeleteRelation(fromID, relID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

func (a *App) HandleGetComments(c echo.Context) error {
	out, err := a.data.GetComments()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting relation: %v", err))
	}

	if out == nil {
		return c.JSON(http.StatusOK, okResp{[]interface{}{}})
	}

	return c.JSON(http.StatusOK, okResp{out})
}

func (a *App) HandleDeleteComments(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("commentID"))

	if id < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `id`.")
	}

	if err := a.data.DeleteComments(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting comments: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

func (a *App) HandleDeletePending(c echo.Context) error {
	if err := a.data.DeleteAllPending(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting pending entries: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

func (a *App) validateEntry(e data.Entry) (data.Entry, error) {
	for i, v := range e.Content {
		e.Content[i] = strings.TrimSpace(v)
	}

	if len(e.Content) == 0 || strings.TrimSpace(e.Content[0]) == "" {
		return data.Entry{}, errors.New("invalid `content`")
	}

	if strings.TrimSpace(e.Initial) == "" {
		return data.Entry{}, errors.New("invalid `initial`")
	}

	if _, ok := a.data.Langs[e.Lang]; !ok {
		return data.Entry{}, errors.New("unknown `lang`")
	}

	return e, nil
}

// adminPage is the root handler that renders the Javascript admin frontend.
func (a *App) adminPage(tpl string) func(c echo.Context) error {
	return func(c echo.Context) error {
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
		err := a.adminTpl.ExecuteTemplate(b, tpl, struct {
			Title    string
			AssetVer string
			Consts   Consts
		}{title, assetVer, a.consts})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				fmt.Sprintf("error compiling template: %v", err))
		}

		return c.HTMLBlob(http.StatusOK, b.Bytes())
	}
}

// basicAuth middleware does an HTTP BasicAuth authentication for admin handlers.
func (a *App) basicAuth(username, password string, c echo.Context) (bool, error) {
	if subtle.ConstantTimeCompare([]byte(username), a.consts.AdminUsername) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), a.consts.AdminPassword) == 1 {
		c.Set(isAuthed, true)
		return true, nil
	}

	return false, nil
}
