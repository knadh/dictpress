package search

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
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
	ID() string
	Name() string

	// Tokenize takes a string and tokenizes it into a list of tsvector tokens
	// that can be stored in the database for fulltext search.
	ToTokens(string) []string

	// ToTSQuery takes a search string and returns a Postgres tsquery string,
	// for example 'fat & cat`.
	ToQuery(string) string
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
}

// Search represents the dictionary search interface.
type Search struct {
	queries *Queries
	Langs   LangMap
}

// Query represents the parameters of a single search query.
type Query struct {
	Query    string
	FromLang string
	ToLang   string
	Types    []string
	Tags     []string
	Status   string
	Offset   int
	Limit    int
}

// Entry represents a dictionary entry.
type Entry struct {
	ID        int            `json:"id" db:"id"`
	GUID      string         `json:"guid" db:"guid"`
	Weight    float64        `json:"weight" db:"weight"`
	Initial   string         `json:"initial" db:"initial"`
	Lang      string         `json:"lang" db:"lang"`
	Content   string         `json:"content" db:"content"`
	Tokens    string         `json:"tokens" db:"tokens"`
	Tags      pq.StringArray `json:"tags" db:"tags"`
	Phones    pq.StringArray `json:"phones" db:"phones"`
	Notes     string         `json:"notes" db:"notes"`
	Status    string         `json:"status" db:"status"`
	Relations Entries        `json:"relations,omitempty" db:"relations"`
	Total     int            `json:"-" db:"total"`
	CreatedAt null.Time      `json:"created_at" db:"created_at"`
	UpdatedAt null.Time      `json:"updated_at" db:"updated_at"`

	// Non-public fields for scanning relationship data and populating Relation.
	FromID            int            `json:"-" db:"from_id"`
	RelationID        int            `json:"-" db:"relation_id"`
	RelationTypes     pq.StringArray `json:"-" db:"relation_types"`
	RelationTags      pq.StringArray `json:"-" db:"relation_tags"`
	RelationNotes     string         `json:"-" db:"relation_notes"`
	RelationWeight    float64        `json:"-" db:"relation_weight"`
	RelationCreatedAt null.Time      `json:"-" db:"relation_created_at"`
	RelationUpdatedAt null.Time      `json:"-" db:"relation_updated_at"`

	// RelationEntry encompasses an Entry with added fields that
	// describes its relationship to other Entries. This is only populated
	// Entries in the Relations list.
	Relation *Relation `json:"relation,omitempty"`
}

// Relation represents the relationship between two IDs.
type Relation struct {
	ID        int            `json:"id"`
	Types     pq.StringArray `json:"types"`
	Tags      pq.StringArray `json:"tags"`
	Notes     string         `json:"notes"`
	Weight    float64        `json:"weight"`
	CreatedAt null.Time      `json:"created_at"`
	UpdatedAt null.Time      `json:"updated_at"`
}

// Entries represents a slice of Entry.
type Entries []Entry

// GlossaryWord to read glosary content from db.
type GlossaryWord struct {
	ID      int    `json:"id" db:"id"`
	GUID    string `json:"guid" db:"guid"`
	Content string `json:"content" db:"content"`
	Total   int    `json:"-" db:"total"`
}

// Stats contains database statistics.
type Stats struct {
	Entries   int            `json:"entries"`
	Relations int            `json:"relations"`
	Languages map[string]int `json:"languages"`
}

// New returns an instance of the search interface.
func New(q *Queries, langs LangMap) *Search {
	return &Search{
		queries: q,
		Langs:   langs,
	}
}

// Search returns the entries filtered and paginated by a
// given Query along with the total number of matches in the
// database.
func (s *Search) Search(q Query) (Entries, int, error) {
	// Is there a Tokenizer?
	var (
		tsVectorLang  = ""
		tsVectorQuery string
		out           Entries
	)

	lang, ok := s.Langs[q.FromLang]
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
		tsVectorQuery = tk.ToQuery(q.Query)
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

	if err := s.queries.Search.Select(&out,
		q.Query,
		tsVectorLang,
		tsVectorQuery,
		q.FromLang,
		pq.StringArray(q.Tags),
		q.Status,
		q.Offset, q.Limit,
	); err != nil || len(out) == 0 {
		return nil, 0, err
	}

	return out, out[0].Total, nil
}

// GetInitials gets the list of all unique initials (first character) across
// all the words for a given language.
func (s *Search) GetInitials(lang string) ([]string, error) {
	out := make([]string, 0, 200)

	rows, err := s.queries.GetInitials.Query(lang)
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
func (s *Search) GetGlossaryWords(lang, initial string, offset, limit int) ([]GlossaryWord, int, error) {
	var out []GlossaryWord
	if err := s.queries.GetGlossaryWords.Select(&out, lang, initial, offset, limit); err != nil || len(out) == 0 {
		if len(out) == 0 {
			err = fmt.Errorf("glossary is empty")
		}

		return nil, 0, err
	}

	return out, out[0].Total, nil
}

// GetEntry returns an entry by its guid.
func (s *Search) GetEntry(guid string) (Entry, error) {
	var out Entry
	if err := s.queries.GetEntry.Get(&out, guid); err != nil {
		return out, err
	}

	return out, nil
}

// GetParentEntries returns the parent entries of an entry by its guid.
func (s *Search) GetParentEntries(guid string) (Entries, error) {
	var out Entries
	if err := s.queries.GetParentRelations.Select(&out, guid); err != nil {
		return out, err
	}

	return out, nil
}

// InsertEntry inserts a new dictionart entry.
func (s *Search) InsertEntry(e Entry) (string, error) {
	lang, ok := s.Langs[e.Lang]
	if !ok {
		return "", fmt.Errorf("unknown language %s", e.Lang)
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
			tokens = strings.Join(lang.Tokenizer.ToTokens(e.Content), " ")
		}
	}

	var guid string
	err := s.queries.InsertEntry.Get(&guid,
		e.GUID,
		e.Content,
		e.Initial,
		e.Weight,
		tokens,
		tsVectorLang,
		e.Lang,
		e.Tags,
		e.Phones,
		e.Notes,
		e.Status)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Constraint == "idx_entries_guid" {
			if e.GUID == "" {
				return "", errors.New("the entry already exists.")
			}
			return "", errors.New("the guid already exists.")
		}
	}

	return guid, err
}

// UpdateEntry updates a dictionary entry.
func (s *Search) UpdateEntry(guid string, e Entry) error {
	_, err := s.queries.UpdateEntry.Exec(guid,
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

// InsertRelation adds a relation between to entries.
func (s *Search) InsertRelation(fromGuid, toGuid string, r Relation) error {
	_, err := s.queries.InsertRelation.Exec(fromGuid,
		toGuid,
		r.Types,
		r.Tags,
		r.Notes,
		r.Weight)
	return err
}

// UpdateRelation updates a relation's properties.
func (s *Search) UpdateRelation(id int, r Relation) error {
	_, err := s.queries.UpdateRelation.Exec(id,
		r.Types,
		r.Tags,
		r.Notes,
		r.Weight)
	return err
}

// ReorderRelations updates the weights of the given relation IDs in the given order.
func (s *Search) ReorderRelations(ids []int) error {
	_, err := s.queries.ReorderRelations.Exec(pq.Array(ids))
	return err
}

// DeleteEntry deletes a dictionary entry by its guid.
func (s *Search) DeleteEntry(guid string) error {
	_, err := s.queries.DeleteEntry.Exec(guid)
	return err
}

// DeleteRelation deletes a dictionary entry by its guid.
func (s *Search) DeleteRelation(fromGuid, toGuid string) error {
	_, err := s.queries.DeleteRelation.Exec(fromGuid, toGuid)
	return err
}

// GetStats returns DB stats.
func (s *Search) GetStats() (Stats, error) {
	var (
		out Stats
		b   json.RawMessage
	)
	if err := s.queries.GetStats.Get(&b); err != nil {
		return out, err
	}

	err := json.Unmarshal(b, &out)

	return out, err
}

// SearchAndLoadRelations loads related entries into a slice of Entries.
func (e Entries) SearchAndLoadRelations(q Query, stmt *sqlx.Stmt) error {
	var (
		IDs = make([]int64, len(e))

		// Map that stores the slice indexes in e against Entry IDs
		// to attach relations back into e.
		idMap = make(map[int]int)
	)

	for i := 0; i < len(e); i++ {
		IDs[i] = int64(e[i].ID)
		e[i].Relations = make(Entries, 0)
		idMap[e[i].ID] = i
	}

	var relEntries Entries
	if err := stmt.Select(&relEntries,
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
