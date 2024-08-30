package codegen

import (
	"fmt"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

func ToMarkdown(input string) string {
	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}

	renderer, err := glamour.NewTermRenderer(glamour.WithWordWrap(0), glamour.WithStandardStyle(style))
	if err != nil {
		panic(fmt.Errorf("failed rendering markdown %q: %w", input, err))
	}

	out, err := renderer.Render(input)
	if err != nil {
		panic(fmt.Errorf("failed rendering markdown %q: %w", input, err))
	}
	return out
}
