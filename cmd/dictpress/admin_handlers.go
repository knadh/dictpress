package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"github.com/labstack/echo/v4"
)

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

	if err := validateEntry(e); err != nil {
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

// handleUpdateEntry updates a dictionary entry.
func handleUpdateEntry(c echo.Context) error {
	var (
		app   = c.Get("app").(*App)
		id, _ = strconv.Atoi(c.Param("id"))
	)

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

	var rel data.Relation
	if err := c.Bind(&rel); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if err := app.data.InsertRelation(fromID, toID, rel); err != nil {
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

	var ids []int
	if err := json.NewDecoder(c.Request().Body).Decode(&ids); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if err := app.data.ReorderRelations(ids); err != nil {
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
		toID, _   = strconv.Atoi(c.Param("toID"))
	)

	if err := app.data.DeleteRelation(fromID, toID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("error deleting relation: %v", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

func validateEntry(e data.Entry) error {
	if strings.TrimSpace(e.Content) == "" {
		return errors.New("invalid `content`.")
	}
	if strings.TrimSpace(e.Initial) == "" {
		return errors.New("invalid `initial`.")
	}
	if strings.TrimSpace(e.Lang) == "" {
		return errors.New("invalid `lang`.")
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
			title = fmt.Sprintf("Search '%s'", c.Request().URL.Query().Get("query"))
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
