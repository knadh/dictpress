// package importer imports a dictionary CSV into the database.
package importer

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictpress/internal/data"
	"github.com/lib/pq"
)

const (
	insertBatchSize = 5000
	colCount        = 11

	typeEntry = "-"
	typeDef   = "^"
)

// entry represents a single row read from the CSV. The CSV columns are:
// Array columns like tokens, tags etc. are pipe (|) separated.
// entry_type, word, initial, language, notes, tsvector_language, [tsvector_tokens], [tags], [phones], definition_type, meta
//
// entry_type = - represents a main entry and subsequent ^ represents definitions.
// definition_type (last field) should only be set in definition (^) entries.
// It represents the part of speech types defined in the config. Eg: noun, verb etc.
//
// tsvector_language = Name of the Postgres language tokenizer if it's a built in one.
// If this is set, content is automatically tokenized using this language in Postgres and [tsvector_tokens] can be left empty.
// If the language does not have a Postgres tokenizer, leave tsvector_language empty and manually set [tsvector_tokens]
type entry struct {
	// Comments show CSV column positions.
	Type           string   // 0
	Initial        string   // 1
	Content        string   // 2
	Lang           string   // 3
	Notes          string   // 4
	TSVectorLang   string   // 4
	TSVectorTokens string   // 6
	Tags           []string // 7
	Phones         []string // 8
	DefTypes       []string // 9 - Only read in definition entries (0=^)
	Meta           string   // 10

	defs []entry
}

// Importer imports CSV entries into the database.
type Importer struct {
	langs data.LangMap

	db              *sqlx.DB
	stmtInsertEntry *sqlx.Stmt
	stmtInsertRel   *sqlx.Stmt
	lo              *log.Logger
}

var (
	reSpaces, _ = regexp.Compile("\\s+")
)

// New returns a new instance of the CSV importer.
func New(langs data.LangMap, stmtInsertEntry *sqlx.Stmt, stmtInsertRel *sqlx.Stmt, db *sqlx.DB, lo *log.Logger) *Importer {
	return &Importer{
		langs:           langs,
		stmtInsertEntry: stmtInsertEntry,
		stmtInsertRel:   stmtInsertRel,
		db:              db,
		lo:              lo,
	}
}

// Import imports a CSV file into the DB.
func (im *Importer) Import(filePath string) error {
	fp, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file %s: %v", filePath, err)
	}

	var (
		// Holds all main entries.
		entries []entry
		n       = 0
		numMain = 0
		numDefs = 0
	)

	rd := csv.NewReader(fp)
	rd.FieldsPerRecord = -1
	for {
		row, err := rd.Read()
		if err != nil {
			if err == io.EOF {
				break
			}

			return fmt.Errorf("error reading CSV file %s: %v", filePath, err)
		}

		if n == 0 && row[0] != "-" {
			return fmt.Errorf("line %d: first row in the file should be of type '-'", n)
		}
		n++

		e, err := im.readEntry(row)
		if err != nil {
			return fmt.Errorf("error reading line %d: %v", n, err)
		}

		// First entry is always a main entry.
		if len(entries) == 0 {
			entries = append(entries, e)
			continue
		}

		// Add all definitions to the last main entry in the list.
		if e.Type == typeDef {
			i := len(entries) - 1
			entries[i].defs = append(entries[i].defs, e)
			numDefs++
			continue
		}

		// On hitting the batchsize, insert to DB.
		if len(entries)%insertBatchSize == 0 {
			if err := im.insertEntries(entries, numMain); err != nil {
				return fmt.Errorf("error inserting entries to DB: %v", err)
			}

			numMain += len(entries)
			entries = []entry{}

			im.lo.Printf("imported %d entries and %d definitions", numMain, numDefs)
		}

		// New main entry.
		entries = append(entries, e)
	}

	if len(entries) > 0 {
		if err := im.insertEntries(entries, numMain); err != nil {
			return fmt.Errorf("error inserting entries to DB: %v", err)
		}
	}

	im.lo.Printf("finished. imported %d entries and %d definitions", numMain+len(entries), numDefs)
	return nil
}

// initial, content, lang, notes, tsvector_language, [tokens|], [tags|], [pronunciations|]
func (im *Importer) readEntry(r []string) (entry, error) {
	typ := cleanString(r[0])
	if typ != typeEntry && typ != typeDef {
		return entry{}, fmt.Errorf("unknown type '%s' in column 0. Should be '-' (entry), or '^' for definition", typ)
	}

	e := entry{
		Type:           typ,
		Initial:        cleanString(r[1]),
		Content:        cleanString(r[2]),
		Lang:           cleanString(r[3]),
		Notes:          cleanString(r[4]),
		TSVectorLang:   cleanString(r[5]),
		TSVectorTokens: cleanString(r[6]),
		Tags:           splitString(cleanString(r[7])),
		Phones:         splitString(cleanString(r[8])),
	}

	if len(r) != colCount {
		return e, fmt.Errorf("every line should have exactly %d columns. Found %d", colCount, len(r))
	}

	lang, ok := im.langs[e.Lang]
	if !ok {
		return e, fmt.Errorf("unknown language '%s' at column 2", e.Lang)
	}

	if e.Content == "" {
		return e, fmt.Errorf("empty content (word) at column 1")
	}

	if e.Initial == "" {
		e.Initial = strings.ToUpper(string(e.Content[0]))
	}

	// If the Postgres tokenizer is not set, and there are no tokens supplied,
	// see if the language has a custom one and use it.
	if lang.Tokenizer != nil && e.TSVectorLang == "" && e.TSVectorTokens == "" {
		tks, err := lang.Tokenizer.ToTokens(e.Content, lang.ID)
		if err != nil {
			return e, fmt.Errorf("error tokenizing content (word) at column 1: %v", err)
		}

		e.TSVectorTokens = strings.Join(tks, " ")
	}

	defTypeStr := cleanString(r[9])
	if typ == typeDef {
		defTypes := splitString(defTypeStr)
		for _, t := range defTypes {
			if _, ok := lang.Types[t]; !ok {
				return e, fmt.Errorf("unknown type '%s' for language '%s'", t, e.Lang)
			}
		}
		e.DefTypes = defTypes
	} else if defTypeStr != "" {
		return e, fmt.Errorf("column 10, definition type (part of speech) should only be set of definition entries (^)")
	}

	e.Meta = strings.TrimSpace(e.Meta)
	if e.Meta == "" {
		e.Meta = "{}"
	} else if e.Meta[0:1] != "{" {
		return e, fmt.Errorf("column 11, meta JSON should begin with `{`")
	}

	return e, nil
}

func (im *Importer) insertEntries(entries []entry, lineStart int) error {
	var (
		tx   *sqlx.Tx
		stmt *sqlx.Stmt
		err  error
	)

	// Insert entries.
	entryIDs := make([]int, len(entries))
	if tx, err = im.db.Beginx(); err != nil {
		return err
	}
	stmt = tx.Stmtx(im.stmtInsertEntry)
	for i, e := range entries {
		if err := stmt.Get(&entryIDs[i],
			pq.StringArray([]string{e.Content}),
			e.Initial,
			lineStart,
			e.TSVectorTokens,
			e.TSVectorLang,
			e.Lang,
			pq.StringArray(e.Tags),
			pq.StringArray(e.Phones),
			e.Notes,
			e.Meta,
			data.StatusEnabled); err != nil {
			log.Printf("error inserting entry: %v", err)
			return err
		}
		lineStart++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Insert definition entries and collect their IDs for every main entry.
	relIDs := make([][]int, len(entries))

	if tx, err = im.db.Beginx(); err != nil {
		return err
	}
	stmt = tx.Stmtx(im.stmtInsertEntry)

	// Iterate through all main entries again, inserting their definition entries.
	for i, mainEntry := range entries {
		relIDs[i] = make([]int, len(mainEntry.defs))

		for j, e := range mainEntry.defs {
			// Insert the definition entry and record the resulting ID
			// against the parent ID.
			if err := stmt.Get(&relIDs[i][j],
				pq.StringArray([]string{e.Content}),
				e.Initial,
				i+j,
				e.TSVectorTokens,
				e.TSVectorLang,
				e.Lang,
				pq.StringArray{},
				pq.StringArray(e.Phones),
				"",
				e.Meta,
				data.StatusEnabled); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Insert relationships.
	if tx, err = im.db.Beginx(); err != nil {
		return err
	}
	stmt = tx.Stmtx(im.stmtInsertRel)
	for i, defIDs := range relIDs {
		for j, toID := range defIDs {
			d := entries[i].defs[j]
			if _, err := stmt.Exec(entryIDs[i], toID, pq.StringArray(d.DefTypes), pq.StringArray(d.Tags), d.Notes, j, data.StatusEnabled); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func cleanString(s string) string {
	return reSpaces.ReplaceAllString(strings.TrimSpace(s), " ")
}

func splitString(s string) []string {
	out := strings.Split(s, "|")
	for n, v := range out {
		out[n] = strings.TrimSpace(v)
	}

	return out
}
