package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictmaker/search"
	"github.com/knadh/goyesql"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/paginator"
	flag "github.com/spf13/pflag"
)

// Lang represents a language's configuration.
type Lang struct {
	Name          string            `koanf:"name"`
	Types         map[string]string `koanf:"types"`
	TokenizerName string            `koanf:"tokenizer"`
	TokenizerType string            `koanf:"tokenizer_type"`
	Tokenizer     search.Tokenizer
}

// Languages represents a map of Langs.
type Languages map[string]Lang

type constants struct {
	Site string
}

// App contains the "global" components that are
// passed around, especially through HTTP handlers.
type App struct {
	lang       Languages
	constants  *constants
	site       *template.Template
	db         *sqlx.DB
	queries    *search.Queries
	search     *search.Search
	resultsPg  *paginator.Paginator
	glossaryPg *paginator.Paginator
	logger     *log.Logger
}

var (
	logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
	ko     = koanf.New(".")
)

func init() {
	// Commandline flags.
	f := flag.NewFlagSet("config", flag.ContinueOnError)
	f.Usage = func() {
		fmt.Println(f.FlagUsages())
		os.Exit(0)
	}
	f.StringSlice("config", []string{"config.toml"},
		"path to one or more config files (will be merged in order)")
	f.String("site", "", "path to a site theme. If left empty, only the APIs will run.")
	f.Bool("install", false, "run first time installation")
	f.Bool("version", false, "current version of the build")
	f.Parse(os.Args[1:])

	// Load config files.
	cFiles, _ := f.GetStringSlice("config")
	for _, f := range cFiles {
		log.Printf("reading config: %s", f)
		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			log.Printf("error reading config: %v", err)
		}
	}

	if err := ko.Load(posflag.Provider(f, ".", ko), nil); err != nil {
		log.Fatalf("error loading config: %v", err)
	}
}

func main() {
	// Connect to the DB.
	db, err := connectDB(ko.String("db.host"),
		ko.Int("db.port"),
		ko.String("db.user"),
		ko.String("db.password"),
		ko.String("db.db"))
	if err != nil {
		logger.Fatalf("error connecting to DB: %v", err)
	}
	defer db.Close()

	// Load SQL queries.
	qMap, err := goyesql.ParseFile("queries.sql")
	if err != nil {
		logger.Fatalf("error loading SQL queries: %v", err)
	}

	// Map queries to the query container.
	var q search.Queries
	if err := scanQueriesToStruct(&q, qMap, db.Unsafe()); err != nil {
		logger.Fatalf("no SQL queries loaded: %v", err)
	}

	// Initialize the app context that's passed around.
	app := &App{
		constants: &constants{
			Site: ko.String("site"),
		},
		lang:    make(map[string]Lang),
		search:  search.NewSearch(&q),
		queries: &q,
		db:      db,
		logger:  logger,
	}

	// Pagination.
	o := paginator.Default()
	o.DefaultPerPage = ko.Int("results.default_per_page")
	o.MaxPerPage = ko.Int("results.max_per_page")
	o.NumPageNums = ko.Int("results.num_page_nums")
	app.resultsPg = paginator.New(o)

	o.DefaultPerPage = ko.Int("glossary.default_per_page")
	o.MaxPerPage = ko.Int("glossary.max_per_page")
	o.NumPageNums = ko.Int("glossary.num_page_nums")
	app.glossaryPg = paginator.New(o)

	// Language configuration.
	if err := loadLanguages(app); err != nil {
		logger.Fatalf("error loading language conf: %v", err)
	}
	if len(app.lang) == 0 {
		logger.Fatal("0 languages in config")
	}

	// Load site theme.
	if app.constants.Site != "" {
		logger.Printf("loading site theme: %v", app.constants.Site)
		t, err := loadSiteTheme(app.constants.Site, ko.Bool("app.enable_pages"))
		if err != nil {
			logger.Fatalf("error loading site theme: %v", err)
		}
		app.site = t
	}

	// Start the HTTP server.
	r := chi.NewRouter()
	registerHandlers(r, app)
	logger.Println("listening on", ko.String("app.address"))
	logger.Fatal(http.ListenAndServe(ko.String("app.address"), r))
}
