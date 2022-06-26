package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const (
	StatusPending  = "pending"
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
)

// Lang represents a language's configuration.
type Lang struct {
	Name          string            `json:"name"`
	Types         map[string]string `json:"types"`
	TokenizerName string            `json:"tokenizer"`
	TokenizerType string            `json:"tokenizer_type"`
	Tokenizer     Tokenizer         `json:"-"`
}

// LangMap represents a map of language controllers indexed by the language key.
type LangMap map[string]Lang

// Tokenizer represents a function that takes a string
// and returns a list of Postgres tsvector tokens.
type Tokenizer interface {
	// Tokenize takes a string and tokenizes it into a list of tsvector tokens
	// that can be stored in the database for fulltext search.
	ToTokens(s string, lang string) ([]string, error)

	// ToTSQuery takes a search string and returns a Postgres tsquery string,
	// for example 'fat & cat`.
	ToQuery(s string, lang string) (string, error)
}

// Token represents a Postgres tsvector token.
type Token struct {
	Token  string
	Weight int
}

// Queries contains prepared DB queries.
type Queries struct {
	Search             *sqlx.Stmt `query:"search"`
	SearchRelations    *sqlx.Stmt `query:"search-relations"`
	GetEntry           *sqlx.Stmt `query:"get-entry"`
	GetParentRelations *sqlx.Stmt `query:"get-parent-relations"`
	GetInitials        *sqlx.Stmt `query:"get-initials"`
	GetGlossaryWords   *sqlx.Stmt `query:"get-glossary-words"`
	InsertEntry        *sqlx.Stmt `query:"insert-entry"`
	UpdateEntry        *sqlx.Stmt `query:"update-entry"`
	InsertRelation     *sqlx.Stmt `query:"insert-relation"`
	UpdateRelation     *sqlx.Stmt `query:"update-relation"`
	ReorderRelations   *sqlx.Stmt `query:"reorder-relations"`
	DeleteEntry        *sqlx.Stmt `query:"delete-entry"`
	DeleteRelation     *sqlx.Stmt `query:"delete-relation"`
	GetStats           *sqlx.Stmt `query:"get-stats"`

	GetPendingEntries        *sqlx.Stmt `query:"get-pending-entries"`
	InsertSubmissionEntry    *sqlx.Stmt `query:"insert-submission-entry"`
	InsertSubmissionRelation *sqlx.Stmt `query:"insert-submission-relation"`
	InsertComments           *sqlx.Stmt `query:"insert-comments"`
	GetComments              *sqlx.Stmt `query:"get-comments"`
	DeleteComments           *sqlx.Stmt `query:"delete-comments"`
	ApproveSubmission        *sqlx.Stmt `query:"approve-submission"`
	RejectSubmission         *sqlx.Stmt `query:"reject-submission"`
}

// Data represents the dictionary search interface.
type Data struct {
	queries *Queries
	Langs   LangMap
}

// Query represents the parameters of a single search query.
type Query struct {
	Query    string   `json:"query"`
	FromLang string   `json:"from_lang"`
	ToLang   string   `json:"to_lang"`
	Types    []string `json:"types"`
	Tags     []string `json:"tags"`
	Status   string   `json:"status"`
	Offset   int      `json:"offset"`
	Limit    int      `json:"limit"`
}

// New returns an instance of the search interface.
func New(q *Queries, langs LangMap) *Data {
	return &Data{
		queries: q,
		Langs:   langs,
	}
}

// Search returns the entries filtered and paginated by a
// given Query along with the total number of matches in the
// database.
func (d *Data) Search(q Query) ([]Entry, int, error) {
	// Is there a Tokenizer?
	var (
		tsVectorLang  = ""
		tsVectorQuery string
		out           []Entry
	)

	lang, ok := d.Langs[q.FromLang]
	if !ok {
		return out, 0, fmt.Errorf("unknown language %s", q.FromLang)
	}

	var (
		tkName = lang.TokenizerName
		tk     = lang.Tokenizer
	)

	if tk == nil {
		// No external tokenizer. Use the Postgres tokenizer name.
		tsVectorLang = tkName
	} else {
		// If there's an external tokenizer loaded, run it to get the tokens
		// and pass it to the DB directly instructing the DB not to tokenize internally.
		var err error
		tsVectorQuery, err = tk.ToQuery(q.Query, q.FromLang)
		if err != nil {
			return nil, 0, err
		}
	}

	// Filters ($1 to $3)
	// $1 - raw search query to use in union if tokens don't yield results
	// $2 - builtin PG fulltext dictionary language name (english|german...). Empty in case of an external tokenizer.
	// $3 - externally computed tokens if $2 = empty
	// $4 - lang (optional)
	// $5 - []types (optional)
	// $6 - []tags (optional)
	// $7 - offset
	// $8 - limit

	if err := d.queries.Search.Select(&out,
		q.Query,
		tsVectorLang,
		tsVectorQuery,
		q.FromLang,
		pq.StringArray(q.Tags),
		q.Status,
		q.Offset, q.Limit,
	); err != nil {
		if err == sql.ErrNoRows {
			return []Entry{}, 0, nil
		}

		return nil, 0, err
	}

	if len(out) == 0 {
		return []Entry{}, 0, nil
	}

	// Replace nulls with [].
	for i := range out {
		if out[i].Relations == nil {
			out[i].Relations = []Entry{}
		}
	}

	return out, out[0].Total, nil
}

// GetPendingEntries fetches entries based on the given condition.
func (d *Data) GetPendingEntries(lang string, tags pq.StringArray, offset, limit int) ([]Entry, int, error) {
	var out []Entry

	if err := d.queries.GetPendingEntries.Select(&out, lang, tags, offset, limit); err != nil || len(out) == 0 {
		return nil, 0, err
	}

	// Replace nulls with [].
	for i := range out {
		if out[i].Relations == nil {
			out[i].Relations = []Entry{}
		}
	}

	return out, out[0].Total, nil
}

// GetInitials gets the list of all unique initials (first character) across
// all the words for a given language.
func (d *Data) GetInitials(lang string) ([]string, error) {
	out := make([]string, 0, 200)

	rows, err := d.queries.GetInitials.Query(lang)
	if err != nil {
		return out, err
	}

	if rows.Err() != nil {
		return out, rows.Err()
	}

	defer rows.Close()

	var i string

	for rows.Next() {
		if err := rows.Scan(&i); err != nil {
			return out, err
		}

		out = append(out, i)
	}

	return out, nil
}

// GetGlossaryWords gets words ordered by weight for a language
// to build a glossary.
func (d *Data) GetGlossaryWords(lang, initial string, offset, limit int) ([]GlossaryWord, int, error) {
	var out []GlossaryWord
	if err := d.queries.GetGlossaryWords.Select(&out, lang, initial, offset, limit); err != nil || len(out) == 0 {
		if len(out) == 0 {
			return nil, 0, nil
		}

		return nil, 0, err
	}

	return out, out[0].Total, nil
}

// GetEntry returns an entry by its id.
func (d *Data) GetEntry(id int) (Entry, error) {
	var out Entry
	if err := d.queries.GetEntry.Get(&out, id); err != nil {
		return out, err
	}

	return out, nil
}

// GetParentEntries returns the parent entries of an entry by its id.
func (d *Data) GetParentEntries(id int) ([]Entry, error) {
	var out []Entry
	if err := d.queries.GetParentRelations.Select(&out, id); err != nil {
		return out, err
	}

	return out, nil
}

// InsertEntry inserts a new non-unique (content+lang) dictionary entry and returns its id.
func (d *Data) InsertEntry(e Entry) (int, error) {
	id, err := d.insertEntry(e, d.queries.InsertEntry)
	return id, err
}

// InsertSubmissionEntry checks if a given content+lang exists and returns the existing ID.
// If it doesn't exist, a new entry is inserted and its ID is returned. This is used for
// accepting public submissions which are conntected to existing entries (if they exist).
func (d *Data) InsertSubmissionEntry(e Entry) (int, error) {
	id, err := d.insertEntry(e, d.queries.InsertSubmissionEntry)
	return id, err
}

// UpdateEntry updates a dictionary entry.
func (d *Data) UpdateEntry(id int, e Entry) error {
	if e.Status == "" {
		e.Status = StatusEnabled
	}

	_, err := d.queries.UpdateEntry.Exec(id,
		e.Content,
		e.Initial,
		e.Weight,
		e.Tokens,
		e.Lang,
		e.Tags,
		e.Phones,
		e.Notes,
		e.Status)
	return err
}

// InsertRelation adds a non-unique relation between to entries.
func (d *Data) InsertRelation(fromID, toID int, r Relation) (int, error) {
	id, err := d.insertRelation(fromID, toID, r, d.queries.InsertRelation)
	return id, err
}

// InsertRelation adds a relation between to entries only if a from_id+to_id+types
// relation doesn't already exist.
func (d *Data) InsertSubmissionRelation(fromID, toID int, r Relation) (int, error) {
	id, err := d.insertRelation(fromID, toID, r, d.queries.InsertSubmissionRelation)
	return id, err
}

// UpdateRelation updates a relation's properties.
func (d *Data) UpdateRelation(id int, r Relation) error {
	_, err := d.queries.UpdateRelation.Exec(id,
		r.Types,
		r.Tags,
		r.Notes,
		r.Weight)
	return err
}

// ReorderRelations updates the weights of the given relation IDs in the given order.
func (d *Data) ReorderRelations(ids []int) error {
	_, err := d.queries.ReorderRelations.Exec(pq.Array(ids))
	return err
}

// DeleteEntry deletes a dictionary entry by its id.
func (d *Data) DeleteEntry(id int) error {
	_, err := d.queries.DeleteEntry.Exec(id)
	return err
}

// DeleteRelation deletes a dictionary entry by its id.
func (s *Data) DeleteRelation(fromID, toID int) error {
	_, err := s.queries.DeleteRelation.Exec(fromID, toID)
	return err
}

// InsertComments inserts a change suggestion from the public.
func (d *Data) InsertComments(fromGUID, toGUID, comments string) error {
	_, err := d.queries.InsertComments.Exec(fromGUID, toGUID, comments)
	return err
}

// GetComments retrieves change submissions.
func (d *Data) GetComments() ([]Comments, error) {
	var out []Comments

	if err := d.queries.GetComments.Select(&out); err != nil {
		return nil, err
	}

	return out, nil
}

// DeleteComments deletes a change suggestion from the public.
func (d *Data) DeleteComments(id int) error {
	_, err := d.queries.DeleteComments.Exec(id)
	return err
}

// GetStats returns DB stats.
func (d *Data) GetStats() (Stats, error) {
	var (
		out Stats
		b   json.RawMessage
	)
	if err := d.queries.GetStats.Get(&b); err != nil {
		return out, err
	}

	err := json.Unmarshal(b, &out)

	return out, err
}

// ApproveSubmission approves a pending submission (entry, relations, related entries).
func (d *Data) ApproveSubmission(id int) error {
	_, err := d.queries.ApproveSubmission.Exec(id)
	return err
}

// RejectSubmission rejects a pending submission and deletes related pending entries.
func (d *Data) RejectSubmission(id int) error {
	_, err := d.queries.RejectSubmission.Exec(id)
	return err
}

func (d *Data) insertEntry(e Entry, stmt *sqlx.Stmt) (int, error) {
	lang, ok := d.Langs[e.Lang]
	if !ok {
		return 0, fmt.Errorf("unknown language %s", e.Lang)
	}

	// No tokens. Automatically generate.
	var (
		tsVectorLang = ""
		tokens       = e.Tokens
	)
	if len(e.Tokens) == 0 {
		if lang.Tokenizer == nil {
			// No external tokenizer. Use the Postgres tokenizer name.
			tsVectorLang = lang.TokenizerName
		} else {
			// If there's an external tokenizer loaded, run it to get the tokens
			// and pass it to the DB directly instructing the DB not to tokenize internally.
			t, err := lang.Tokenizer.ToTokens(e.Content, e.Lang)
			if err != nil {
				return 0, nil
			}
			tokens = strings.Join(t, " ")
		}
	}

	if e.Status == "" {
		e.Status = StatusEnabled
	}

	var id int
	err := stmt.Get(&id, e.Content, e.Initial, e.Weight, tokens, tsVectorLang, e.Lang, e.Tags, e.Phones, e.Notes, e.Status)
	return id, err
}

func (d *Data) insertRelation(fromID, toID int, r Relation, stmt *sqlx.Stmt) (int, error) {
	if r.Status == "" {
		r.Status = StatusEnabled
	}

	var id int
	err := stmt.Get(&id, fromID, toID, r.Types, r.Tags, r.Notes, r.Weight, r.Status)
	return id, err
}

// SearchAndLoadRelations loads related entries into the given Entries.
func (d *Data) SearchAndLoadRelations(e []Entry, q Query) error {
	var (
		IDs = make([]int64, len(e))

		// Map that stores the slice indexes in e against Entry IDs
		// to attach relations back into e.
		idMap = make(map[int]int)
	)

	for i := 0; i < len(e); i++ {
		IDs[i] = int64(e[i].ID)
		e[i].Relations = make([]Entry, 0)
		idMap[e[i].ID] = i
	}

	var relEntries []Entry
	if err := d.queries.SearchRelations.Select(&relEntries,
		q.ToLang,
		pq.StringArray(q.Types),
		pq.StringArray(q.Tags),
		pq.Int64Array(IDs),
		q.Status); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}

		return err
	}

	for _, r := range relEntries {
		// Copy top-level relation fields to the Relation sub-struct.
		r.Relation = &Relation{
			ID:        r.RelationID,
			Types:     r.RelationTypes,
			Tags:      r.RelationTags,
			Notes:     r.RelationNotes,
			Weight:    r.RelationWeight,
			Status:    r.Status,
			CreatedAt: r.RelationCreatedAt,
			UpdatedAt: r.RelationUpdatedAt,
		}

		idx := idMap[r.FromID]
		e[idx].Relations = append(e[idx].Relations, r)
	}

	return nil
}

// TokensToTSVector takes a list of tokens, de-duplicates them, and returns a
// Postgres tsvector string.
func TokensToTSVector(tokens []Token) []string {
	var (
		keys = make(map[string]bool)
		out  = []string{}
	)
	for _, t := range tokens {
		if _, ok := keys[t.Token]; !ok {
			keys[t.Token] = true
			out = append(out, fmt.Sprintf("%s:%d", t.Token, t.Weight))
		}
	}
	return out
}
