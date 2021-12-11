package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictmaker/internal/data"
	"github.com/knadh/goyesql"
	goyesqlx "github.com/knadh/goyesql/sqlx"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/paginator"
	"github.com/knadh/stuffbin"
	flag "github.com/spf13/pflag"
)

var (
	buildString = "unknown"
)

// Lang represents a language's configuration.
type Lang struct {
	Name          string            `koanf:"name" json:"name"`
	Types         map[string]string `koanf:"types" json:"types"`
	TokenizerName string            `koanf:"tokenizer" json:"tokenizer"`
	TokenizerType string            `koanf:"tokenizer_type" json:"tokenizer_type"`
	Tokenizer     data.Tokenizer    `koanf:"-" json:"-"`
}

type constants struct {
	Site    string
	RootURL string
}

// App contains the "global" components that are
// passed around, especially through HTTP handlers.
type App struct {
	constants  constants
	siteTpl    *template.Template
	adminTpl   *template.Template
	db         *sqlx.DB
	queries    *data.Queries
	data       *data.Data
	fs         stuffbin.FileSystem
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

	f.Bool("new", false, "generate a new sample config.toml file.")
	f.StringSlice("config", []string{"config.toml"},
		"path to one or more config files (will be merged in order)")
	f.String("site", "", "path to a site theme. If left empty, only HTTP APIs will be available.")
	f.Bool("install", false, "run first time DB installation")
	f.Bool("yes", false, "assume 'yes' to prompts, eg: during --install")
	f.Bool("version", false, "current version of the build")

	if err := f.Parse(os.Args[1:]); err != nil {
		logger.Fatalf("error parsing flags: %v", err)
	}

	if ok, _ := f.GetBool("version"); ok {
		fmt.Println(buildString)
		os.Exit(0)
	}

	// Generate new config file.
	if ok, _ := f.GetBool("new"); ok {
		if err := generateNewFiles(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("config.toml generated. Edit and run --install.")
		os.Exit(0)
	}

	// Load config files.
	cFiles, _ := f.GetStringSlice("config")
	for _, f := range cFiles {
		logger.Printf("reading config: %s", f)

		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			fmt.Printf("error reading config: %v", err)
			os.Exit(1)
		}
	}

	if err := ko.Load(posflag.Provider(f, ".", ko), nil); err != nil {
		logger.Fatalf("error loading config: %v", err)
	}
}

func main() {
	// Connect to the DB.
	db := initDB(ko.String("db.host"),
		ko.Int("db.port"),
		ko.String("db.user"),
		ko.String("db.password"),
		ko.String("db.db"),
	)
	defer db.Close()

	// Initialize the app context that's passed around.
	app := &App{
		constants: constants{
			Site:    ko.String("site"),
			RootURL: ko.String("app.root_url"),
		},
		db:     db,
		fs:     initFS(),
		logger: logger,
	}

	// Install schema.
	if ko.Bool("install") {
		installSchema(app, !ko.Bool("yes"))
		return
	}

	// Load SQL queries.
	qB, err := app.fs.Read("/queries.sql")
	if err != nil {
		logger.Fatalf("error reading queries.sql: %v", err)
	}

	qMap, err := goyesql.ParseBytes(qB)
	if err != nil {
		logger.Fatalf("error loading SQL queries: %v", err)
	}

	// Map queries to the query container.
	var q data.Queries

	if err := goyesqlx.ScanToStruct(&q, qMap, db.Unsafe()); err != nil {
		logger.Fatalf("no SQL queries loaded: %v", err)
	}

	// Language configuration.
	langs := initLangs(ko)
	if len(langs) == 0 {
		logger.Fatal("0 languages in config")
	}

	app.data = data.New(&q, langs)
	app.queries = &q

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

	// Load admin theme.
	app.adminTpl = initAdminTemplates("admin")

	// Load site theme.
	if app.constants.Site != "" {
		logger.Printf("loading site theme: %v", app.constants.Site)

		t, err := loadSiteTheme(app.constants.Site, ko.Bool("app.enable_pages"))
		if err != nil {
			logger.Fatalf("error loading site theme: %v", err)
		}

		app.siteTpl = t
	}

	// Start the HTTP server.
	r := chi.NewRouter()
	initHandlers(r, app)
	logger.Println("listening on", ko.String("app.address"))
	logger.Fatal(http.ListenAndServe(ko.String("app.address"), r))
}
