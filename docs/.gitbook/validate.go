package main

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/logging"
	"go.uber.org/zap"
)

var zlog, _ = logging.PackageLogger("github.com/streamingfast/substreams/docs/.gitbook", "gitbook.validate")

func main() {
	logging.InstantiateLoggers(logging.WithDefaultSpec(".*=error"))

	gitbookConfig, err := findGitbookConfig(".")
	cli.NoError(err, "Unable to find gitbook root")

	docsRoot := "."
	if gitbookConfig != "" {
		docsRoot, err = readGitbookConfigRoot(gitbookConfig)
		cli.NoError(err, "Unable to read gitbook config root")
	}

	fmt.Println("Validating out Gitbook documentation")
	fmt.Println("Documentation root:", docsRoot)

	fmt.Println("")
	fmt.Println("Checking internal Markdown links...")
	validReferences := validateLocalReferences(docsRoot)

	fmt.Println("")
	fmt.Println("Checking all files linked to SUMMARY.md...")
	validSummary := validateAllFilesAreInSummary(docsRoot)

	fmt.Println("")
	fmt.Println("Checking for duplicate file references in SUMMARY.md...")
	validUniqueSummary := validateSummaryNoDuplicates(docsRoot)

	fmt.Println("")
	if validReferences && validSummary && validUniqueSummary {
		fmt.Println("🎉 All documentation checks passed!")
		os.Exit(0)
	} else {
		fmt.Println("❌ Documentation checks failed!")
		os.Exit(1)
	}
}

func validateLocalReferences(docsRoot string) bool {
	sourceFiles, err := extractAllSourceFiles(docsRoot)
	cli.NoError(err, "Unable to extract all source files")

	brokenLinks := []MarkdownLink{}
	for sourceFile := range sourceFiles {
		zlog.Info("checking file", zap.String("path", sourceFile))
		content, err := os.ReadFile(sourceFile)
		cli.NoError(err, "unable to read source file", zap.String("path", sourceFile))

		// Find all markdown links
		zlog.Info("validating links", zap.String("path", sourceFile))
		broken := validateMarkdownLinks(sourceFile, string(content))
		brokenLinks = append(brokenLinks, broken...)
	}

	// Report results
	if len(brokenLinks) == 0 {
		fmt.Println("✓ All internal Markdown links are valid!")
	} else {
		fmt.Printf("\n❌ Found %d broken links:\n\n", len(brokenLinks))

		// Group by source file
		linksByFile := make(map[string][]MarkdownLink)
		for _, link := range brokenLinks {
			linksByFile[link.SourceFile] = append(linksByFile[link.SourceFile], link)
		}

		for sourceFile, links := range linksByFile {
			fmt.Printf("File: %s\n", sourceFile)
			for _, link := range links {
				if link.LineNumber > 0 {
					fmt.Printf("  Line %d: [%s](%s)\n", link.LineNumber, link.LinkText, link.LinkPath)
				} else {
					fmt.Printf("  [%s](%s)\n", link.LinkText, link.LinkPath)
				}
				fmt.Printf("    → Target not found: %s\n", link.LinkPath)
			}
			fmt.Println()
		}

		return false
	}

	return true
}

func validateSummaryNoDuplicates(docsRoot string) bool {
	cwd, err := os.Getwd()
	cli.NoError(err, "Unable to get current working directory")

	summaryPath := filepath.Join(docsRoot, "SUMMARY.md")
	summaryContent, err := os.ReadFile(summaryPath)
	cli.NoError(err, "Unable to read SUMMARY.md", zap.String("path", summaryPath))

	summaryLinks := extractMarkdownLinks(summaryPath, string(summaryContent))

	// Track which files appear and where
	fileOccurrences := make(map[string][]MarkdownLink)
	for _, link := range summaryLinks {
		if link.TargetFilePath != nil {
			fileOccurrences[*link.TargetFilePath] = append(fileOccurrences[*link.TargetFilePath], link)
		}
	}

	// Find duplicates
	duplicates := []string{}
	for filePath, occurrences := range fileOccurrences {
		if len(occurrences) > 1 {
			duplicates = append(duplicates, filePath)
		}
	}

	if len(duplicates) == 0 {
		fmt.Println("✓ No duplicate file references in SUMMARY.md!")
		return true
	}

	fmt.Printf("\n❌ Found %d files referenced multiple times in SUMMARY.md:\n\n", len(duplicates))
	for _, filePath := range duplicates {
		relPath, err := filepath.Rel(cwd, filePath)
		if err != nil {
			relPath = filePath
		}
		fmt.Printf("  • %s (referenced %d times):\n", relPath, len(fileOccurrences[filePath]))
		for _, link := range fileOccurrences[filePath] {
			relSummaryPath, err := filepath.Rel(cwd, summaryPath)
			if err != nil {
				relSummaryPath = summaryPath
			}
			fmt.Printf("    - %s:%d - [%s](%s)\n", relSummaryPath, link.LineNumber, link.LinkText, link.LinkPath)
		}
		fmt.Println()
	}

	return false
}

func validateAllFilesAreInSummary(docsRoot string) bool {
	sourceFiles, err := extractAllSourceFiles(docsRoot)
	cli.NoError(err, "Unable to extract all source files")

	summaryPath := filepath.Join(docsRoot, "SUMMARY.md")
	summaryContent, err := os.ReadFile(summaryPath)
	cli.NoError(err, "Unable to read SUMMARY.md", zap.String("path", summaryPath))

	summaryLinks := extractMarkdownLinks(summaryPath, string(summaryContent))

	linkedFiles := make(map[string]bool)
	for _, link := range summaryLinks {
		if link.TargetFilePath != nil {
			linkedFiles[*link.TargetFilePath] = true
		}
	}

	unlinkedFiles := []string{}
	for sourceFile := range sourceFiles {
		// Skip SUMMARY.md itself
		if sourceFile == summaryPath {
			continue
		}

		if _, found := linkedFiles[sourceFile]; !found {
			unlinkedFiles = append(unlinkedFiles, sourceFile)
		}
	}

	if len(unlinkedFiles) == 0 {
		fmt.Println("✓ All documentation files are linked in SUMMARY.md!")
	} else {
		fmt.Printf("\n❌ Found %d unlinked documentation files:\n\n", len(unlinkedFiles))
		for _, file := range unlinkedFiles {
			fmt.Printf("  • %s\n", file)
		}

		return false
	}

	return true
}

func extractAllSourceFiles(docsRoot string) (map[string]bool, error) {
	sourceFiles := map[string]bool{}
	err := filepath.WalkDir(docsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-markdown files
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		sourceFiles[path] = true
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking docs root: %w", err)
	}

	return sourceFiles, nil
}

// Matches: [text](path) and [text](path "title")
var linkRegex = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

func extractMarkdownLinks(sourceFile string, content string) (out []MarkdownLink) {
	lineNum := 0
	for line := range strings.SplitSeq(content, "\n") {
		lineNum++
		matches := linkRegex.FindAllStringSubmatch(line, -1)

		for _, match := range matches {
			zlog.Debug("found link", zap.String("source_file", sourceFile), zap.String("line", line), zap.Any("match", match))
			if len(match) < 3 {
				continue
			}

			linkText := match[1]
			linkPath := match[2]
			linkTitle := ""
			if len(match) >= 4 {
				linkTitle = match[3]
			}

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
				parsedURL, err := url.Parse(linkPath)
				if err != nil {
					zlog.Warn("unable to parse url link", zap.String("link", linkPath), zap.Error(err))
					continue
				}

				out = append(out, MarkdownLink{
					LinkText:  linkText,
					LinkPath:  linkPath,
					LinkTitle: linkTitle,

					TargetHTTPPath: parsedURL,

					LineNumber: lineNum + 1,
					SourceFile: sourceFile,
				})
				continue
			}

			// Skip anchor-only links
			if strings.HasPrefix(linkPath, "#") {
				out = append(out, MarkdownLink{
					LinkText:  linkText,
					LinkPath:  linkPath,
					LinkTitle: linkTitle,

					TargetFilePath: &sourceFile,

					LineNumber: lineNum + 1,
					SourceFile: sourceFile,
				})
				continue
			}

			// Remove anchor/fragment for file existence check
			pathWithoutAnchor := linkPath
			if hashIdx := strings.Index(linkPath, "#"); hashIdx != -1 {
				pathWithoutAnchor = linkPath[:hashIdx]
			}

			// Resolve relative path
			sourceDir := filepath.Dir(sourceFile)
			targetPath := filepath.Join(sourceDir, pathWithoutAnchor)
			targetPath = filepath.Clean(targetPath)

			out = append(out, MarkdownLink{
				LinkText:  linkText,
				LinkPath:  linkPath,
				LinkTitle: linkTitle,

				TargetFilePath: &targetPath,

				SourceFile: sourceFile,
				LineNumber: lineNum + 1,
			})
		}
	}

	return
}

func validateMarkdownLinks(sourceFile string, content string) (broken []MarkdownLink) {
	for _, link := range extractMarkdownLinks(sourceFile, content) {
		if link.TargetFilePath != nil {
			if _, err := os.Stat(*link.TargetFilePath); os.IsNotExist(err) {
				broken = append(broken, link)
			}
		}
	}

	return broken
}

func readGitbookConfigRoot(configPath string) (string, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	rootRegex := regexp.MustCompile(`(?m)^\s*root\s*:\s*(\S+)\s*$`)
	matches := rootRegex.FindSubmatch(content)
	if len(matches) < 2 {
		return "", nil // No root defined
	}

	// Make value retrieve relative to config file location
	rootDir := filepath.Dir(configPath)
	rootValue := string(matches[1])
	absRoot := filepath.Join(rootDir, rootValue)
	sanitizedRoot := filepath.Clean(absRoot)

	return sanitizedRoot, nil
}

func findGitbookConfig(startDir string) (string, error) {
	var configPath string
	err := walkParents(startDir, func(path string) error {
		candidate := filepath.Join(path, ".gitbook.yaml")
		if _, err := os.Stat(candidate); err == nil {
			configPath = candidate
			return fs.ErrExist // Use fs.ErrExist to break the walk early
		}
		return nil
	})

	if err != nil && err != fs.ErrExist {
		return "", err
	}

	return configPath, nil
}

func walkParents(path string, fn func(string) error) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	for {
		err := fn(absPath)
		if err != nil {
			return err
		}

		parent := filepath.Dir(absPath)
		if parent == absPath {
			break
		}
		absPath = parent
	}

	return nil
}

// MarkdownLink represents a link found in a Markdown file, normally extracted
// as [text](path) or [text](path "title") format.
type MarkdownLink struct {
	// LinkText is the actually extracted <text> part of the Markdown link.
	LinkText string
	// LinkPath is the actually extracted <path> part of the Markdown link,
	// potentially including anchors/fragments.
	LinkPath string
	// LinkTitle is the optional title/tooltip part of the Markdown link.
	LinkTitle string

	// TargetFilePath is the resolved file path the link points to, if path
	// was determined to be a file path, it is resolved against the source file.
	TargetFilePath *string
	// TargetHTTPPath is the resolved URL the link points to, if path was
	// determined to be a URL.
	TargetHTTPPath *url.URL

	SourceFile string
	LineNumber int
}

func (m *MarkdownLink) IsAnchorOnly() bool {
	return strings.HasPrefix(m.LinkPath, "#")
}

func (m MarkdownLink) String() string {
	return fmt.Sprintf("[%s](%s) in %s:%d", m.LinkText, m.LinkPath, m.SourceFile, m.LineNumber)
}
