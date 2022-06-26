package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// newSubmission is an entry and relations submitted by the public for review.
// These are recorded in the entries and relations table with status=pending.
type newSubmission struct {
	EntryLang    string `form:"entry_lang"`
	EntryContent string `form:"entry_content"`
	EntryPhones  string `form:"entry_phones"`
	EntryNotes   string `form:"entry_notes"`

	RelationLang    []string `form:"relation_lang"`
	RelationContent []string `form:"relation_content"`
	RelationTypes   []string `form:"relation_type"`
}

// changeSubmission is a comment for change submitted by the public that can be
// reviewed and manually incorporated into entries.
type changeSubmission struct {
	FromGUID string `json:"from_guid"`
	ToGUID   string `json:"to_guid"`
	Comments string `json:"comments"`
}

// handleNewSubmission inserts a new dictionary entry suggestion from the public
// in the `pending` state for review.
func handleNewSubmission(c echo.Context) error {
	app := c.Get("app").(*App)

	var s newSubmission
	if err := c.Bind(&s); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	s.EntryContent = strings.TrimSpace(s.EntryContent)
	if len(s.EntryContent) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid `entry_content`.")
	}

	// Validate input.
	ln := len(s.RelationLang)
	if ln == 0 || ln != len(s.RelationContent) || ln != len(s.RelationTypes) {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid submission fields.")
	}

	// Validate language and type enums.
	if _, ok := app.data.Langs[s.EntryLang]; !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Unknown `entry_lang`.")
	}
	for i := range s.RelationLang {
		lang, ok := app.data.Langs[s.RelationLang[i]]
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "Unknown `relation_lang`.")
		}

		if _, ok := lang.Types[s.RelationTypes[i]]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "Unknown `relation_type`.")
		}

		s.RelationContent[i] = strings.TrimSpace(s.RelationContent[i])
		if len(s.RelationContent[i]) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid `entry_content`.")
		}
	}

	// Check if the main entry and the relational entries already exist.
	// If they exist, no new entries are inserted, only relations.

	// Insert the main entry.
	phones := []string{}
	for _, p := range strings.Split(s.EntryPhones, ",") {
		p = strings.TrimSpace(p)
		if len(p) > 0 {
			phones = append(phones, p)
		}
	}

	e := data.Entry{
		Lang:    s.EntryLang,
		Initial: strings.ToUpper(string(s.EntryContent[0])),
		Content: s.EntryContent,
		Phones:  pq.StringArray(phones),
		Tags:    pq.StringArray{},
		Status:  data.StatusPending,
	}

	// Save the main entry.
	fromID, err := app.data.InsertSubmissionEntry(e)
	if err != nil {
		app.lo.Printf("error inserting submission entry: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("Error saving entry.", err))
	}

	// Insert relations.
	for i := range s.RelationLang {
		phones := []string{}
		for _, p := range strings.Split(s.EntryPhones, ",") {
			p = strings.TrimSpace(p)
			if len(p) > 0 {
				phones = append(phones, p)
			}
		}

		toID, err := app.data.InsertSubmissionEntry(data.Entry{
			Lang:    s.RelationLang[i],
			Initial: strings.ToUpper(string(s.RelationContent[0][0])),
			Content: s.RelationContent[i],
			Phones:  pq.StringArray(phones),
			Tags:    pq.StringArray{},
			Status:  data.StatusPending,
		})
		if err != nil {
			app.lo.Printf("error inserting submission definition: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				fmt.Sprintf("Error saving definition.", err))
		}

		rel := data.Relation{
			Types:  pq.StringArray{s.RelationTypes[i]},
			Tags:   pq.StringArray{},
			Status: data.StatusPending,
		}
		if _, err := app.data.InsertSubmissionRelation(fromID, toID, rel); err != nil {
			app.lo.Printf("error inserting submission relation: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				fmt.Sprintf("Error saving relation.", err))
		}
	}

	return nil
}

// handleNewComments records a suggestion for change from the public in the changes table.
// These suggestions are reviewed in the admin and any change involves manually incorporating
// them to the linked entries.
func handleNewComments(c echo.Context) error {
	app := c.Get("app").(*App)

	var s changeSubmission
	if err := c.Bind(&s); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			fmt.Sprintf("error parsing request: %v", err))
	}

	if s.FromGUID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid `from_guid`.")
	}

	if len(s.Comments) > 1000 {
		return echo.NewHTTPError(http.StatusBadRequest, "Comments are too big.")
	}

	if err := app.data.InsertComments(s.FromGUID, s.ToGUID, s.Comments); err != nil {
		app.lo.Printf("error inserting change submission: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("Error saving submission.", err))
	}

	return c.JSON(http.StatusOK, okResp{true})
}
