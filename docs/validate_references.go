package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type BrokenLink struct {
	SourceFile string
	LinkText   string
	TargetPath string
	LineNumber int
}

func main() {
	docsRoot := "."

	brokenLinks := []BrokenLink{}

	// Walk through all markdown files
	err := filepath.WalkDir(docsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-markdown files
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
			return nil
		}

		// Find all markdown links
		broken := validateMarkdownLinks(path, string(content))
		brokenLinks = append(brokenLinks, broken...)

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// Report results
	if len(brokenLinks) == 0 {
		fmt.Println("✓ All internal Markdown links are valid!")
	} else {
		fmt.Printf("\n❌ Found %d broken links:\n\n", len(brokenLinks))

		// Group by source file
		linksByFile := make(map[string][]BrokenLink)
		for _, link := range brokenLinks {
			linksByFile[link.SourceFile] = append(linksByFile[link.SourceFile], link)
		}

		for sourceFile, links := range linksByFile {
			fmt.Printf("File: %s\n", sourceFile)
			for _, link := range links {
				if link.LineNumber > 0 {
					fmt.Printf("  Line %d: [%s](%s)\n", link.LineNumber, link.LinkText, link.TargetPath)
				} else {
					fmt.Printf("  [%s](%s)\n", link.LinkText, link.TargetPath)
				}
				fmt.Printf("    → Target not found: %s\n", link.TargetPath)
			}
			fmt.Println()
		}

		os.Exit(1)
	}
}

func validateMarkdownLinks(sourceFile string, content string) []BrokenLink {
	var broken []BrokenLink

	// Regular expressions for markdown links
	// Matches: [text](path) and [text](path "title")
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	// Split content into lines for line number tracking
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		matches := linkRegex.FindAllStringSubmatch(line, -1)

		for _, match := range matches {
			if len(match) < 3 {
				continue
			}

			linkText := match[1]
			linkPath := match[2]

			// Remove any title/tooltip after the path (e.g., "path "tooltip")
			if spaceIdx := strings.Index(linkPath, " "); spaceIdx != -1 {
				linkPath = linkPath[:spaceIdx]
			}

			// Skip empty links
			if linkPath == "" {
				continue
			}

			// Skip URL-like links (absolute URLs and protocols)
			if strings.HasPrefix(linkPath, "http://") ||
			   strings.HasPrefix(linkPath, "https://") ||
			   strings.HasPrefix(linkPath, "ftp://") ||
			   strings.HasPrefix(linkPath, "//") ||
			   strings.HasPrefix(linkPath, "mailto:") {
				continue
			}

			// Skip anchor-only links
			if strings.HasPrefix(linkPath, "#") {
				continue
			}

			// Remove anchor/fragment for file existence check
			pathWithoutAnchor := linkPath
			if hashIdx := strings.Index(linkPath, "#"); hashIdx != -1 {
				pathWithoutAnchor = linkPath[:hashIdx]
			}

			// Skip if empty after removing anchor
			if pathWithoutAnchor == "" {
				continue
			}

			// Resolve relative path
			sourceDir := filepath.Dir(sourceFile)
			targetPath := filepath.Join(sourceDir, pathWithoutAnchor)
			targetPath = filepath.Clean(targetPath)

			// Check if target exists
			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				broken = append(broken, BrokenLink{
					SourceFile: sourceFile,
					LinkText:   linkText,
					TargetPath: linkPath,
					LineNumber: lineNum + 1,
				})
			}
		}
	}

	return broken
}
