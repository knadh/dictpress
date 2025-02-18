package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
)

var (
	reClean = regexp.MustCompile(`\s+`)
)

// generateSitemaps generates sitemap files from database content.
func generateSitemaps(fromLang, toLang, rootURL string, maxRows int, outputPrefix, outputDir string, getQuery *sqlx.Stmt) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %w", err)
	}

	// Fetch all DB entries.
	rows, err := getQuery.Queryx(fromLang)
	if err != nil {
		return fmt.Errorf("error querying database: %w", err)
	}
	defer rows.Close()

	var (
		urls  []string
		n     = 0
		index = 1
	)
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			return fmt.Errorf("error scanning row: %w", err)
		}

		word = strings.ToLower(strings.TrimSpace(word))
		word = reClean.ReplaceAllString(word, "+")

		// Generate URL. eg: site.com/dictionary/$fromLang/$toLang/$word
		dictURL, err := url.JoinPath(rootURL, "dictionary", fromLang, toLang, word)
		if err != nil {
			return fmt.Errorf("error joining URL paths: %w", err)
		}
		urls = append(urls, dictURL)

		// Write sitemap if we've reached the maximum URLs per file.
		if len(urls) >= maxRows {
			if err := writeSitemap(urls, index, outputPrefix, outputDir); err != nil {
				return err
			}

			urls = urls[:0]
			index++
		}
		n++
	}

	// Write remaining URLs if any.
	if len(urls) > 0 {
		if err := writeSitemap(urls, index, outputPrefix, outputDir); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	lo.Printf("generated %d URLs in %d sitemap files\n", n, index)

	return nil
}

// writeSitemap writes a slice of URLs to a sitemap file.
func writeSitemap(urls []string, index int, outputPrefix, outputDir string) error {
	filepath := filepath.Join(outputDir, fmt.Sprintf("%s%d.txt", outputPrefix, index))

	lo.Printf("writing to %s\n", filepath)
	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("error creating sitemap file: %w", err)
	}
	defer f.Close()

	for _, u := range urls {
		if _, err := f.WriteString(u + "\n"); err != nil {
			return fmt.Errorf("error writing URL to sitemap: %w", err)
		}
	}

	return nil
}

// generateRobotsTxt generates a robots.txt file with sitemap references.
func generateRobotsTxt(sitemapURL string, outputDir string) error {
	robotsPath := filepath.Join(outputDir, "robots.txt")

	lo.Printf("writing to %s\n", robotsPath)

	f, err := os.Create(robotsPath)
	if err != nil {
		return fmt.Errorf("error creating robots.txt: %w", err)
	}
	defer f.Close()

	// Write default robots.txt content.
	content := `User-agent: *
Disallow:
Allow: /
`
	if _, err := f.WriteString(strings.TrimSpace(content) + "\n\n"); err != nil {
		return fmt.Errorf("error writing robots.txt content: %w", err)
	}

	// Add sitemap references.
	files, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("error reading output directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && file.Name() != "robots.txt" && strings.Contains(file.Name(), ".txt") {
			sitemapURL, err := url.JoinPath(sitemapURL, file.Name())
			if err != nil {
				return fmt.Errorf("error joining URL paths: %w", err)
			}
			if _, err := f.WriteString(fmt.Sprintf("Sitemap: %s\n", sitemapURL)); err != nil {
				return fmt.Errorf("error writing sitemap URL: %w", err)
			}
		}
	}

	return nil
}
