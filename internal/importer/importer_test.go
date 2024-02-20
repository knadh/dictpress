package importer

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/goyesql"
	goyesqlx "github.com/knadh/goyesql/sqlx"
	"github.com/knadh/koanf/v2"
	"gotest.tools/v3/assert"
)

const (
	host          = "localhost"
	port          = 5432
	user          = "dictpress"
	pwd           = "dictpress"
	dbName        = "dictpress"
	tokenizerType = "postgres"
	relationTable = "relations"
	settingsTable = "settings"
	entriesTable  = "entries"
	commentsTable = "comments"
)

var (
	db    *sqlx.DB
	q     data.Queries
	out   data.Dicts
	langs data.LangMap
)

func initDB(t *testing.T) *sqlx.DB {
	// assumes  postgres is up
	dbInstance, err := sqlx.Connect("postgres",
		fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, pwd, dbName))
	if err != nil {
		t.Fatal(err)
	}
	return dbInstance
}

func prepareQueries(t *testing.T, db *sqlx.DB) {
	// Load SQL queries.
	f, err := os.OpenFile("../../queries.sql", os.O_RDONLY, 0400)
	if err != nil {
		t.Fatal(err.Error())
	}
	qB, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err.Error())
	}

	qMap, err := goyesql.ParseBytes(qB)
	if err != nil {
		t.Fatalf("error loading SQL queries: %v", err)
	}

	// Map queries to the query container.
	if err := goyesqlx.ScanToStruct(&q, qMap, db.Unsafe()); err != nil {
		t.Fatalf("no SQL queries loaded: %v", err)
	}
}

// TODO reuse the existing methods in init.go
func intializeLangDict(t *testing.T, langs data.LangMap, ko *koanf.Koanf) {
	var (
		out   = make(data.Dicts, 0)
		dicts [][]string
	)
	if err := ko.Unmarshal("mock/case_1", &dicts); err != nil {
		t.Fatalf("error unmarshalling app.dict in config: %v", err)
	}
	for _, pair := range dicts {
		if len(pair) != 2 {
			t.Fatalf("app.dicts should have language pairs: %v", pair)
		}

		var (
			fromID = pair[0]
			toID   = pair[1]
		)
		from, ok := langs[fromID]
		if !ok {
			t.Fatalf("unknown language '%s' defined in app.dicts config", fromID)
		}

		to, ok := langs[toID]
		if !ok {
			t.Fatalf("unknown language '%s' defined in app.dicts config", toID)
		}

		out = append(out, [2]data.Lang{from, to})
	}
}

func populateLangs() {
	// TODO read from case_1.toml
	langs = make(data.LangMap)
	engLangTypeMap := make(map[string]string)
	italianLangTypeMap := make(map[string]string)
	engLangTypeMap["noun"] = "Noun"
	italianLangTypeMap["sost"] = "Sostantivo"
	langs["italian"] = data.Lang{
		ID:            "italian",
		Name:          "Italian",
		Types:         italianLangTypeMap,
		TokenizerName: "",
		TokenizerType: tokenizerType,
	}
	langs["english"] = data.Lang{
		ID:            "english",
		Name:          "English",
		Types:         engLangTypeMap,
		TokenizerName: "",
		TokenizerType: tokenizerType,
	}
}

func prepareTest(t *testing.T) {
	k := koanf.New(".")
	db = initDB(t)
	prepareQueries(t, db)
	populateLangs()
	intializeLangDict(t, langs, k)
}

func cleanUp(t *testing.T) {
	// Truncate tables
	_, err := db.DB.Exec(fmt.Sprintf("truncate %s,%s,%s,%s",
		relationTable, settingsTable, entriesTable, commentsTable))
	if err != nil {
		t.Fatal(err)
	}
}

func verify(t *testing.T, filePath string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	expectedLen := len(records)
	var count []int
	err = db.Select(&count, fmt.Sprintf("SELECT count(*) FROM %s", entriesTable))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, count[0], expectedLen)
	return true
}

func TestImporter_Import(t *testing.T) {
	type fields struct {
		langs           data.LangMap
		db              *sqlx.DB
		stmtInsertEntry *sqlx.Stmt
		stmtInsertRel   *sqlx.Stmt
		lo              *log.Logger
	}
	prepareTest(t)
	defer db.Close()
	type args struct {
		filePath string
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErr    bool
		verifyFunc func(string) bool
	}{
		{
			"case_1:Importer_Import success",
			fields{
				langs:           langs,
				db:              db,
				stmtInsertEntry: q.InsertEntry,
				stmtInsertRel:   q.InsertRelation,
				lo:              log.Default(),
			},
			args{
				filePath: "mock/case_1.csv",
			},
			false,
			func(filePath string) bool {
				return verify(t, filePath)
			},
		},
		{
			"case_2:Importer_Import failure",
			fields{
				langs:           langs,
				db:              db,
				stmtInsertEntry: q.InsertEntry,
				stmtInsertRel:   q.InsertRelation,
				lo:              log.Default(),
			},
			args{
				filePath: "mock/case_2.csv",
			},
			true,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer cleanUp(t)
			im := &Importer{
				langs:           tt.fields.langs,
				db:              tt.fields.db,
				stmtInsertEntry: tt.fields.stmtInsertEntry,
				stmtInsertRel:   tt.fields.stmtInsertRel,
				lo:              tt.fields.lo,
			}
			if err := im.Import(tt.args.filePath); (err != nil) != tt.wantErr {
				t.Errorf("Import() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.verifyFunc != nil {
				assert.Assert(t, tt.verifyFunc(tt.args.filePath))
			}
		})
	}
}
