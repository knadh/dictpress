package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"html/template"
	"log"
	mrand "math/rand"
	"os"
	"path/filepath"
	"unicode"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/dictpress/internal/importer"
	"github.com/knadh/go-i18n"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/paginator"
	"github.com/knadh/stuffbin"
	"github.com/urfave/cli/v2"
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
	AdminAssets                  []string
	EnableSubmissions            bool
	EnableGlossary               bool
	AdminUsername, AdminPassword []byte
}

// App contains the "global" components that are
// passed around, especially through HTTP handlers.
type App struct {
	consts     Consts
	db         *sqlx.DB
	queries    *data.Queries
	data       *data.Data
	i18n       *i18n.I18n
	fs         stuffbin.FileSystem
	resultsPg  *paginator.Paginator
	glossaryPg *paginator.Paginator
	lo         *log.Logger

	adminTpl     *template.Template
	siteTpl      *template.Template
	sitePageTpls map[string]*template.Template
}

var (
	lo = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
)

// loadConfig loads the configuration files before any cli actions are run.
func loadConfig(c *cli.Context) *koanf.Koanf {
	ko := koanf.New(".")

	// Load config files in order.
	for _, f := range c.StringSlice("config") {
		lo.Printf("reading config: %s", f)
		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			log.Fatalf("error reading config: %v", err)
		}
	}

	// Load command line flags into koanf.
	if c.String("site") != "" {
		ko.Set("site", c.String("site"))
	}

	return ko
}

func runNewConfig(ctx *cli.Context) error {
	if _, err := os.Stat("config.toml"); !os.IsNotExist(err) {
		lo.Fatal("config.toml exists. Remove it to generate a new one")
	}

	// Initialize the static file system into which all
	// required static assets (.sql, .js files etc.) are loaded.
	fs := initFS()

	// Generate config file.
	b, err := fs.Read("config.sample.toml")
	if err != nil {
		lo.Fatalf("error reading sample config (is binary stuffed?): %v", err)
	}

	// Inject a random password.
	p := make([]byte, 12)
	rand.Read(p)
	pwd := []byte(fmt.Sprintf("%x", p))

	for i, c := range pwd {
		if mrand.Intn(4) == 1 {
			pwd[i] = byte(unicode.ToUpper(rune(c)))
		}
	}

	b = bytes.Replace(b, []byte("dictpress_admin_password"), pwd, -1)
	if err := os.WriteFile("config.toml", b, 0644); err != nil {
		return err
	}

	fmt.Println("config.toml generated. Edit and run --install.")
	os.Exit(0)
	return nil
}

func runInstall(c *cli.Context) error {
	installSchema(versionString, !c.Bool("yes"), initFS(), initDB(loadConfig(c)), loadConfig(c))
	return nil
}

func runUpgrade(c *cli.Context) error {
	upgrade(!c.Bool("yes"), initFS(), initDB(loadConfig(c)), loadConfig(c))
	return nil
}

func runImport(c *cli.Context) error {
	var (
		ko = loadConfig(c)
		db = initDB(ko)
		q  = initQueries(initFS(), db)
	)

	imp := importer.New(initLangs(ko), q.InsertSubmissionEntry, q.InsertSubmissionRelation, db, lo)
	lo.Printf("importing data from %s ...", c.String("file"))
	if err := imp.Import(c.String("file")); err != nil {
		lo.Fatal(err)
	}
	os.Exit(0)
	return nil
}
func runSitemap(c *cli.Context) error {
	var (
		ko      = loadConfig(c)
		queries = initQueries(initFS(), initDB(ko))
		consts  = initConstants(ko)
	)

	lo.Printf("generating sitemaps for %s -> %s", c.String("from-lang"), c.String("to-lang"))

	// Generate the sitemaps.
	err := generateSitemaps(c.String("from-lang"),
		c.String("to-lang"),
		consts.RootURL,
		c.Int("max-rows"),
		c.String("output-prefix"),
		c.String("output-dir"),
		queries.GetEntriesForSitemap)
	if err != nil {
		lo.Fatal(err)
	}

	// Generate robots.txt?
	if c.Bool("robots") {
		if err := generateRobotsTxt(c.String("url"), c.String("output-dir")); err != nil {
			lo.Fatal(err)
		}
	}

	return nil
}

func runServer(c *cli.Context) error {
	var (
		ko     = loadConfig(c)
		consts = initConstants(ko)
		fs     = initFS()
		db     = initDB(ko)
		langs  = initLangs(ko)
		dt     = data.New(initQueries(fs, db), langs, initDicts(langs, ko))
	)

	// Before the queries are prepared, see if there are pending upgrades.
	checkUpgrade(db)
	queries := initQueries(fs, db)

	// Initialize the global app context for the server.
	app := &App{
		consts:  consts,
		db:      db,
		fs:      fs,
		queries: queries,
		data:    dt,

		resultsPg: paginator.New(paginator.Opt{
			DefaultPerPage: ko.MustInt("results.default_per_page"),
			MaxPerPage:     ko.MustInt("results.max_per_page"),
			NumPageNums:    ko.MustInt("results.num_page_nums"),
			PageParam:      "page", PerPageParam: "PerPageParam",
		}),
	}

	if consts.EnableGlossary {
		app.glossaryPg = paginator.New(paginator.Opt{
			DefaultPerPage: ko.MustInt("glossary.default_per_page"),
			MaxPerPage:     ko.MustInt("glossary.max_per_page"),
			NumPageNums:    ko.MustInt("glossary.num_page_nums"),
			PageParam:      "page", PerPageParam: "PerPageParam",
		})
	}

	// Load admin HTML templates.
	app.adminTpl = initAdminTemplates(fs)

	// Initialize the echo HTTP server.
	srv := initHTTPServer(app, ko)

	// Load optional HTML website.
	if consts.Site != "" {
		lo.Printf("loading site theme: %s", consts.Site)
		theme, pages, err := loadSite(consts.Site, ko.Bool("app.enable_pages"))
		if err != nil {
			lo.Fatalf("error loading site theme: %v", err)
		}

		// Optionally load a language pack.
		langFile := filepath.Join(consts.Site, "lang.json")
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
		app.siteTpl = theme
		app.sitePageTpls = pages
		srv.Renderer = &tplRenderer{tpls: theme}
	}

	lo.Printf("starting server on %s", ko.MustString("app.address"))
	if err := srv.Start(ko.MustString("app.address")); err != nil {
		lo.Fatalf("error starting HTTP server: %v", err)
	}

	return nil
}

func main() {
	cliApp := &cli.App{
		Name:    "dictpress",
		Usage:   "Build dictionary websites. https://dict.press",
		Version: versionString,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:    "config",
				Value:   cli.NewStringSlice("config.toml"),
				Usage:   "path to one or more config files (will be merged in order)",
				EnvVars: []string{"DICTPRESS_CONFIG"},
			},
			&cli.StringFlag{
				Name:    "site",
				Usage:   "path to a site theme. If left empty, only HTTP APIs will be available",
				EnvVars: []string{"DICTPRESS_SITE"},
			},
		},
		Action: runServer,
		Commands: []*cli.Command{
			{
				Name:   "new-config",
				Usage:  "Generate a new sample config.toml file",
				Action: runNewConfig,
			},
			{
				Name:   "install",
				Usage:  "Run first time DB installation",
				Action: runInstall,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "yes",
						Usage: "assume 'yes' to prompts during installation",
					},
				},
			},
			{
				Name:   "upgrade",
				Usage:  "Upgrade database to the current version",
				Action: runUpgrade,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "yes",
						Usage: "assume 'yes' to prompts during upgrade",
					},
				},
			},
			{
				Name:   "import",
				Usage:  "Import a CSV file into the database",
				Action: runImport,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "file",
						Usage:    "CSV file to import",
						Required: true,
					},
				},
			},
			{
				Name:   "sitemap",
				Usage:  "Generate static txt sitemap files for all dictionary entries for search engines.",
				Action: runSitemap,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "from-lang",
						Usage:    "Language to translate from",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "to-lang",
						Usage:    "Language to translate to",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "url",
						Usage:    "Root URL where sitemaps will be placed (only used in robots.txt). Eg: https://site.com/sitemaps",
						Required: false,
					},
					&cli.IntFlag{
						Name:  "max-rows",
						Usage: "Maximum number of URL rows per sitemap file",
						Value: 49990,
					},
					&cli.StringFlag{
						Name:  "output-prefix",
						Usage: "Prefix for the sitemap files. Eg: sitemap generates sitemap_1.txt, sitemap_2.txt etc.",
						Value: "sitemap",
					},
					&cli.StringFlag{
						Name:  "output-dir",
						Usage: "Directory to generate the files in",
						Value: "sitemaps",
					},
					&cli.BoolFlag{
						Name:  "robots",
						Usage: "Generate robots.txt",
					},
				},
			},
		},
	}

	if err := cliApp.Run(os.Args); err != nil {
		lo.Fatal(err)
	}
}
