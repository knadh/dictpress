package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/dictpress/internal/importer"
	"github.com/knadh/go-i18n"
	"github.com/knadh/goyesql"
	goyesqlx "github.com/knadh/goyesql/sqlx"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/paginator"
	"github.com/knadh/stuffbin"
	flag "github.com/spf13/pflag"
)

var (
	buildString   = "unknown"
	versionString = "unknown"
)

// Lang represents a language's configuration.
type Lang struct {
	Name          string            `koanf:"name" json:"name"`
	Types         map[string]string `koanf:"types" json:"types"`
	TokenizerName string            `koanf:"tokenizer" json:"tokenizer"`
	TokenizerType string            `koanf:"tokenizer_type" json:"tokenizer_type"`
	Tokenizer     data.Tokenizer    `koanf:"-" json:"-"`
}

type Consts struct {
	Site                         string
	RootURL                      string
	EnableSubmissions            bool
	EnableGlossary               bool
	AdminUsername, AdminPassword []byte
}

// App contains the "global" components that are
// passed around, especially through HTTP handlers.
type App struct {
	consts     Consts
	adminTpl   *template.Template
	siteTpl    *template.Template
	db         *sqlx.DB
	queries    *data.Queries
	data       *data.Data
	i18n       *i18n.I18n
	fs         stuffbin.FileSystem
	resultsPg  *paginator.Paginator
	glossaryPg *paginator.Paginator
	lo         *log.Logger
}

var (
	lo = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
	ko = koanf.New(".")
)

func init() {
	// Commandline flags.
	f := flag.NewFlagSet("config", flag.ContinueOnError)

	f.Usage = func() {
		fmt.Println(f.FlagUsages())
		fmt.Printf("dictpress (%s). Build dictionary websites. https://dict.press", versionString)
		os.Exit(0)
	}

	f.Bool("new-config", false, "generate a new sample config.toml file.")
	f.StringSlice("config", []string{"config.toml"},
		"path to one or more config files (will be merged in order)")
	f.String("site", "", "path to a site theme. If left empty, only HTTP APIs will be available.")
	f.Bool("install", false, "run first time DB installation")
	f.String("import", "", "import a CSV file into the database. eg: --import=data.csv")
	f.Bool("yes", false, "assume 'yes' to prompts, eg: during --install")
	f.Bool("version", false, "current version of the build")

	if err := f.Parse(os.Args[1:]); err != nil {
		lo.Fatalf("error parsing flags: %v", err)
	}

	if ok, _ := f.GetBool("version"); ok {
		fmt.Println(buildString)
		os.Exit(0)
	}

	// Generate new config file.
	if ok, _ := f.GetBool("new-config"); ok {
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
		lo.Printf("reading config: %s", f)

		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			fmt.Printf("error reading config: %v", err)
			os.Exit(1)
		}
	}

	if err := ko.Load(posflag.Provider(f, ".", ko), nil); err != nil {
		lo.Fatalf("error loading config: %v", err)
	}
}

func main() {
	// Connect to the DB.
	db := initDB(ko.MustString("db.host"),
		ko.MustInt("db.port"),
		ko.MustString("db.user"),
		ko.MustString("db.password"),
		ko.MustString("db.db"),
	)
	defer db.Close()

	// Initialize the app context that's passed around.
	app := &App{
		consts: initConstants(ko),
		db:     db,
		fs:     initFS(),
		lo:     lo,
	}

	// Install schema.
	if ko.Bool("install") {
		installSchema(app, !ko.Bool("yes"))
		return
	}

	// Load SQL queries.
	qB, err := app.fs.Read("/queries.sql")
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

	// Load language config.
	var (
		langs = initLangs(ko)
		dicts = initDicts(langs, ko)
	)
	// Run the CSV importer.
	if fPath := ko.String("import"); fPath != "" {
		imp := importer.New(langs, q.InsertSubmissionEntry, q.InsertSubmissionRelation, db, lo)
		lo.Printf("importing data from %s ...", fPath)
		if err := imp.Import(fPath); err != nil {
			lo.Fatal(err)
		}
		os.Exit(0)
	}

	app.data = data.New(&q, langs, dicts)
	app.queries = &q

	// Result paginators.
	app.resultsPg = paginator.New(paginator.Opt{
		DefaultPerPage: ko.MustInt("results.default_per_page"),
		MaxPerPage:     ko.MustInt("results.max_per_page"),
		NumPageNums:    ko.MustInt("results.num_page_nums"),
		PageParam:      "page", PerPageParam: "PerPageParam",
	})

	if app.consts.EnableGlossary {
		app.glossaryPg = paginator.New(paginator.Opt{
			DefaultPerPage: ko.MustInt("glossary.default_per_page"),
			MaxPerPage:     ko.MustInt("glossary.max_per_page"),
			NumPageNums:    ko.MustInt("glossary.num_page_nums"),
			PageParam:      "page", PerPageParam: "PerPageParam",
		})
	}

	// Load admin HTML templates.
	app.adminTpl = initAdminTemplates(app)

	// Initialize the echo HTTP server.
	srv := initHTTPServer(app, ko)

	// Load optional HTML website.
	if app.consts.Site != "" {
		lo.Printf("loading site theme: %s", app.consts.Site)
		t, err := loadSite(app.consts.Site, ko.Bool("app.enable_pages"))
		if err != nil {
			lo.Fatalf("error loading site theme: %v", err)
		}

		// Optionally load a language pack.
		langFile := filepath.Join(app.consts.Site, "lang.json")
		if _, err := os.Stat(langFile); !errors.Is(err, os.ErrNotExist) {
			i, err := i18n.NewFromFile(langFile)
			if err != nil {
				lo.Fatalf("error loading i18n lang.json file: %v", err)
			}
			app.i18n = i
		} else {
			app.i18n, _ = i18n.New([]byte(`{"_.code": "", "_.name": ""}`))
		}

		// Attach HTML template renderer.
		app.siteTpl = t
		srv.Renderer = &tplRenderer{tpls: t}
	}

	lo.Printf("starting server on %s", ko.MustString("app.address"))
	if err := srv.Start(ko.MustString("app.address")); err != nil {
		lo.Fatalf("error starting HTTP server: %v", err)
	}
}
