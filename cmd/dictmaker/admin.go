package main

import (
	"net/http"
)

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	var (
		app = r.Context().Value("app").(*App)
	)

	sendResponse(app.lang, http.StatusOK, w)
}
