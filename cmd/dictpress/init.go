package main

import (
	"fmt"
	"html/template"
	"os"

	"github.com/Masterminds/sprig/v3"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/dictpress/tokenizers/indicphone"
	"github.com/knadh/goyesql"
	goyesqlx "github.com/knadh/goyesql/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

func initConstants(ko *koanf.Koanf) Consts {

	c := Consts{
		Site:              ko.String("site"),
		RootURL:           ko.MustString("app.root_url"),
		AdminUsername:     ko.MustBytes("app.admin_username"),
		AdminPassword:     ko.MustBytes("app.admin_password"),
		EnableSubmissions: ko.Bool("app.enable_submissions"),
		EnableGlossary:    ko.Bool("glossary.enabled"),
		AdminAssets:       ko.Strings("app.admin_assets"),

		SiteMaxEntryRelationsPerType: ko.MustInt("site_results.max_entry_relations_per_type"),
		SiteMaxEntryContentItems:     ko.MustInt("site_results.max_entry_content_items"),
	}

	if len(c.AdminUsername) < 6 {
		lo.Fatal("admin_username should be min 6 characters")
	}

	if len(c.AdminPassword) < 8 {
		lo.Fatal("admin_password should be min 8 characters")
	}

	return c
}

// initDB initializes a database connection.
func initDB(ko *koanf.Koanf) *sqlx.DB {
	db, err := sqlx.Connect("postgres",
		fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			ko.MustString("db.host"),
			ko.MustInt("db.port"),
			ko.MustString("db.user"),
			ko.MustString("db.password"),
			ko.MustString("db.db")))
	if err != nil {
		lo.Fatalf("error initializing DB: %v", err)
	}

	return db
}

// initFS initializes the stuffbin FileSystem to provide
// access to bunded static assets to the app.
func initFS() stuffbin.FileSystem {
	path, err := os.Executable()
	if err != nil {
		lo.Fatalf("error getting executable path: %v", err)
	}

	fs, err := stuffbin.UnStuff(path)
	if err == nil {
		return fs
	}

	// Running in local mode. Load the required static assets into
	// the in-memory stuffbin.FileSystem.
	lo.Printf("unable to initialize embedded filesystem: %v", err)
	lo.Printf("using local filesystem for static assets")

	files := []string{
		"config.sample.toml",
		"queries.sql",
		"schema.sql",
		"admin",
	}

	fs, err = stuffbin.NewLocalFS("/", files...)
	if err != nil {
		lo.Fatalf("failed to load local static files: %v", err)
	}

	return fs
}

func initQueries(fs stuffbin.FileSystem, db *sqlx.DB) *data.Queries {
	// Load SQL queries.
	qB, err := fs.Read("/queries.sql")
	if err != nil {
		lo.Fatalf("error reading queries.sql: %v", err)
	}

	qMap, err := goyesql.ParseBytes(qB)
	if err != nil {
		lo.Fatalf("error loading SQL queries: %v", err)
	}

	// Map queries to the query container.
	var q data.Queries
	if err := goyesqlx.ScanToStruct(&q, qMap, db.Unsafe()); err != nil {
		lo.Fatalf("no SQL queries loaded: %v", err)
	}

	return &q
}

func initAdminTemplates(fs stuffbin.FileSystem) *template.Template {
	// Init admin templates.
	tpls, err := stuffbin.ParseTemplatesGlob(sprig.FuncMap(), fs, "/admin/*.html")
	if err != nil {
		lo.Fatalf("error parsing admin templates: %v", err)
	}

	return tpls
}

// initTokenizers initializes all bundled tokenizers.
func initTokenizers(ko *koanf.Koanf) map[string]data.Tokenizer {
	cfg := indicphone.Config{
		NumKNKeys: ko.Int("tokenizer.indicphone.kn.num_keys"),
		NumMLKeys: ko.Int("tokenizer.indicphone.ml.num_keys"),
	}
	return map[string]data.Tokenizer{
		"indicphone": indicphone.New(cfg),
	}
}

// initLangs loads language configuration into a given *App instance.
func initLangs(ko *koanf.Koanf) data.LangMap {
	var (
		tks = initTokenizers(ko)
		out = make(data.LangMap)
	)

	// Language configuration.
	for _, l := range ko.MapKeys("lang") {
		lang := data.Lang{ID: l, Types: make(map[string]string)}
		if err := ko.UnmarshalWithConf("lang."+l, &lang, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error loading languages: %v", err)
		}

		// Does the language use a bundled tokenizer?
		if lang.TokenizerType == "custom" {
			t, ok := tks[lang.TokenizerName]
			if !ok {
				lo.Fatalf("unknown custom tokenizer '%s'", lang.TokenizerName)
			}
			lang.Tokenizer = t
		}

		// Load external plugin.
		lo.Printf("language: %s", l)
		out[l] = lang
	}

	if len(out) == 0 {
		lo.Fatal("0 languages defined in config")
	}

	return out
}

// initDicts loads language->language dictionary map.
func initDicts(langs data.LangMap, ko *koanf.Koanf) data.Dicts {
	var (
		out   = make(data.Dicts, 0)
		dicts [][]string
	)

	if err := ko.Unmarshal("app.dicts", &dicts); err != nil {
		lo.Fatalf("error unmarshalling app.dict in config: %v", err)
	}

	// Language configuration.
	for _, pair := range dicts {
		if len(pair) != 2 {
			lo.Fatalf("app.dicts should have language pairs: %v", pair)
		}

		var (
			fromID = pair[0]
			toID   = pair[1]
		)
		from, ok := langs[fromID]
		if !ok {
			lo.Fatalf("unknown language '%s' defined in app.dicts config", fromID)
		}

		to, ok := langs[toID]
		if !ok {
			lo.Fatalf("unknown language '%s' defined in app.dicts config", toID)
		}

		out = append(out, [2]data.Lang{from, to})
	}

	if len(out) == 0 {
		lo.Fatal("0 dicts defined in config")
	}

	return out
}
