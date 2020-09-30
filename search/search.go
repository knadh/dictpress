package search

import (
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

// Tokenizer represents a function that takes a string
// and returns a list of Postgres tsvector tokens.
type Tokenizer interface {
	ID() string
	Name() string

	// Tokenize takes a search string and returns a Postgres tsquery string,
	// for example 'fat & cat`.
	Tokenize(string) string
}

// Queries contains prepared DB queries.
type Queries struct {
	Search           *sqlx.Stmt `query:"search"`
	GetRelations     *sqlx.Stmt `query:"get-relations"`
	GetInitials      *sqlx.Stmt `query:"get-initials"`
	GetGlossaryWords *sqlx.Stmt `query:"get-glossary-words"`
}

// Search represents the dictionary search interface.
type Search struct {
	queries *Queries
}

// Query represents the parameters of a single search query.
type Query struct {
	Query         string
	FromLang      string
	ToLang        string
	Types         []string
	Tags          []string
	TokenizerName string
	Tokenizer     Tokenizer
	Offset        int
	Limit         int
}

// Entry represents a dictionary entry.
type Entry struct {
	ID        int            `json:"id" db:"id"`
	GUID      string         `json:"guid" db:"guid"`
	Content   string         `json:"content" db:"content"`
	Lang      string         `json:"lang" db:"lang"`
	Types     pq.StringArray `json:"types" db:"types"`
	Tags      pq.StringArray `json:"tags" db:"tags"`
	Phones    pq.StringArray `json:"phones" db:"phones"`
	Notes     string         `json:"notes" db:"notes"`
	CreatedAt null.Time      `json:"created_at" db:"created_at"`
	UpdatedAt null.Time      `json:"updated_at" db:"updated_at"`
	Relations Entries        `json:"relations,omitempty" db:"relations"`
	Total     int            `json:"-" db:"total"`

	// RelationEntry encompasses an Entry with added fields that
	// describes its relationship to other Entry'ies.
	RelationTypes pq.StringArray `json:"relation_types,omitempty" db:"relation_types"`
	RelationTags  pq.StringArray `json:"relation_tags,omitempty" db:"relation_tags"`
	RelationNotes string         `json:"relation_notes,omitempty" db:"relation_notes"`
	FromID        int            `json:"-" db:"from_id"`
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

// NewSearch returns an instance of the search interface.
func NewSearch(q *Queries) *Search {
	return &Search{
		queries: q,
	}
}

// FindEntries returns the entries filtered and paginated by a
// given Query along with the total number of matches in the
// database.
func (s *Search) FindEntries(q Query) (Entries, int, error) {
	// Is there a Tokenizer?
	var (
		tsVectorLang = ""
		tokens       string
	)

	if q.Tokenizer == nil {
		// No external tokenizer.
		tsVectorLang = q.TokenizerName
	} else {
		// If there's an external tokenizer loaded, run it to get the tokens
		// and pass it to the DB directly instructing the DB not to tokenize internally.
		tokens = q.Tokenizer.Tokenize(q.Query)
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
	var out Entries

	if err := s.queries.Search.Select(&out,
		q.Query,
		tsVectorLang,
		tokens,
		q.FromLang,
		pq.StringArray(q.Types),
		pq.StringArray(q.Tags),
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

	defer func() { _ = rows.Close() }()

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

// LoadRelations loads related entries into a slice of Entries.
func (e Entries) LoadRelations(q Query, stmt *sqlx.Stmt) error {
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

	var rel Entries
	if err := stmt.Select(&rel,
		q.ToLang,
		pq.StringArray(q.Types),
		pq.StringArray(q.Tags),
		pq.Int64Array(IDs)); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}

		return err
	}

	for _, r := range rel {
		idx := idMap[r.FromID]
		e[idx].Relations = append(e[idx].Relations, r)
	}

	return nil
}
