package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/knadh/dictmaker/internal/data"
)

// handleGetConfig returns the language configuration.
func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	var (
		app = r.Context().Value("app").(*App)
	)

	out := struct {
		RootURL   string       `json:"root_url"`
		Languages data.LangMap `json:"languages"`
		Version   string       `json:"version"`
	}{app.constants.RootURL, app.data.Langs, buildString}

	sendResponse(out, http.StatusOK, w)
}

// handleGetStats returns DB statistics.
func handleGetStats(w http.ResponseWriter, r *http.Request) {
	var (
		app = r.Context().Value("app").(*App)
	)

	out, err := app.data.GetStats()
	if err != nil {
		sendErrorResponse(err.Error(), http.StatusInternalServerError, nil, w)
		return
	}

	sendResponse(out, http.StatusOK, w)
}

func handleAdminEntryPage(w http.ResponseWriter, r *http.Request) {
	var (
		app = r.Context().Value("app").(*App)
	)

	sendTpl(http.StatusOK, "entry", app.adminTpl, nil, w)
}

// handleInsertEntry inserts a new dictionary entry.
func handleInsertEntry(w http.ResponseWriter, r *http.Request) {
	var (
		app = r.Context().Value("app").(*App)
	)

	var e data.Entry
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		sendErrorResponse(fmt.Sprintf("error parsing request: %v", err), http.StatusBadRequest, nil, w)
		return
	}

	if err := validateEntry(e); err != nil {
		sendErrorResponse(err.Error(), http.StatusBadRequest, nil, w)
		return
	}

	guid, err := app.data.InsertEntry(e)
	if err != nil {
		sendErrorResponse(fmt.Sprintf("error inserting entry: %v", err), http.StatusInternalServerError, nil, w)
		return
	}

	// Proxy to the get request to respond with the newly inserted entry.
	ctx := chi.RouteContext(r.Context())
	ctx.URLParams.Keys = append(ctx.URLParams.Keys, "guid")
	ctx.URLParams.Values = append(ctx.URLParams.Values, guid)

	handleGetEntry(w, r)
}

// handleUpdateEntry updates a dictionary entry.
func handleUpdateEntry(w http.ResponseWriter, r *http.Request) {
	var (
		app  = r.Context().Value("app").(*App)
		guid = chi.URLParam(r, "guid")
	)

	var e data.Entry
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		sendErrorResponse(fmt.Sprintf("error parsing request: %v", err), http.StatusBadRequest, nil, w)
		return
	}

	if err := app.data.UpdateEntry(guid, e); err != nil {
		sendErrorResponse(fmt.Sprintf("error updating entry: %v", err), http.StatusInternalServerError, nil, w)
		return
	}

	sendResponse(app.data.Langs, http.StatusOK, w)
}

// handleDeleteEntry deletes a dictionary entry.
func handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	var (
		app  = r.Context().Value("app").(*App)
		guid = chi.URLParam(r, "guid")
	)

	if err := app.data.DeleteEntry(guid); err != nil {
		sendErrorResponse(fmt.Sprintf("error deleting entry: %v", err), http.StatusInternalServerError, nil, w)
		return
	}

	sendResponse(true, http.StatusOK, w)
}

// handleAddRelation updates a relation's properties.
func handleAddRelation(w http.ResponseWriter, r *http.Request) {
	var (
		app      = r.Context().Value("app").(*App)
		fromGuid = chi.URLParam(r, "fromGuid")
		toGuid   = chi.URLParam(r, "toGuid")
	)

	var rel data.Relation
	if err := json.NewDecoder(r.Body).Decode(&rel); err != nil {
		sendErrorResponse(fmt.Sprintf("error parsing request: %v", err), http.StatusBadRequest, nil, w)
		return
	}

	if err := app.data.InsertRelation(fromGuid, toGuid, rel); err != nil {
		sendErrorResponse(fmt.Sprintf("error updating relation: %v", err), http.StatusInternalServerError, nil, w)
		return
	}

	sendResponse(app.data.Langs, http.StatusOK, w)
}

// handleUpdateRelation updates a relation's properties.
func handleUpdateRelation(w http.ResponseWriter, r *http.Request) {
	var (
		app      = r.Context().Value("app").(*App)
		relID, _ = strconv.Atoi(chi.URLParam(r, "relID"))
	)

	var rel data.Relation
	if err := json.NewDecoder(r.Body).Decode(&rel); err != nil {
		sendErrorResponse(fmt.Sprintf("error parsing request: %v", err), http.StatusBadRequest, nil, w)
		return
	}

	if err := app.data.UpdateRelation(relID, rel); err != nil {
		sendErrorResponse(fmt.Sprintf("error updating relation: %v", err), http.StatusInternalServerError, nil, w)
		return
	}

	sendResponse(app.data.Langs, http.StatusOK, w)
}

// handleReorderRelations reorders the weights of the relation IDs in the given order.
func handleReorderRelations(w http.ResponseWriter, r *http.Request) {
	var (
		app = r.Context().Value("app").(*App)
	)

	// ids := struct {
	// 	IDs []int `json:ids`
	// }{}
	var ids []int
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		sendErrorResponse(fmt.Sprintf("error parsing request: %v", err), http.StatusBadRequest, nil, w)
		return
	}

	if err := app.data.ReorderRelations(ids); err != nil {
		sendErrorResponse(fmt.Sprintf("error reordering relations: %v", err), http.StatusInternalServerError, nil, w)
		return
	}

	sendResponse(true, http.StatusOK, w)
}

// handleDeleteRelation deletes a relation between two entres.
func handleDeleteRelation(w http.ResponseWriter, r *http.Request) {
	var (
		app      = r.Context().Value("app").(*App)
		fromGuid = chi.URLParam(r, "fromGuid")
		toGuid   = chi.URLParam(r, "toGuid")
	)

	if err := app.data.DeleteRelation(fromGuid, toGuid); err != nil {
		sendErrorResponse(fmt.Sprintf("error deleting entry: %v", err), http.StatusInternalServerError, nil, w)
		return
	}

	sendResponse(true, http.StatusOK, w)
}

func adminPage(tpl string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			app = r.Context().Value("app").(*App)
		)

		title := ""
		switch tpl {
		case "search":
			title = fmt.Sprintf("Search '%s'", r.URL.Query().Get("query"))
		}

		sendTpl(http.StatusOK, tpl, app.adminTpl, struct {
			Title string
		}{title}, w)
	})
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
