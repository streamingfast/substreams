package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EnhancedInput is a custom input component with history and tab completion
type EnhancedInput struct {
	textInput     textinput.Model
	history       *InputHistory
	prompt        string
	description   string
	validation    *regexp.Regexp
	validationErr string
	submitted     bool
	cancelled     bool
}

// NewEnhancedInput creates a new enhanced input component
func NewEnhancedInput(prompt, description, placeholder, defaultValue string, validationRegex, validationError string) *EnhancedInput {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(defaultValue)
	ti.Focus()

	var validationRE *regexp.Regexp
	if validationRegex != "" {
		var err error
		validationRE, err = regexp.Compile(validationRegex)
		if err != nil {
			// Log error but continue without validation
			validationRE = nil
		}
	}

	return &EnhancedInput{
		textInput:     ti,
		history:       globalInputHistory,
		prompt:        prompt,
		description:   description,
		validation:    validationRE,
		validationErr: validationError,
	}
}

// Init implements tea.Model
func (e *EnhancedInput) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model
func (e *EnhancedInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			e.cancelled = true
			return e, tea.Quit

		case "enter":
			value := strings.TrimRight(e.textInput.Value(), " ")

			// Validate if validation is set
			if e.validation != nil && !e.validation.MatchString(value) {
				// Show validation error (could use a status message)
				return e, nil
			}

			e.submitted = true
			e.history.Add(value)
			e.history.Reset()
			return e, tea.Quit

		case "up":
			if value, changed := e.history.NavigateUp(e.textInput.Value()); changed {
				e.textInput.SetValue(value)
				// Move cursor to end
				e.textInput.CursorEnd()
			}
			return e, nil

		case "down":
			if value, changed := e.history.NavigateDown(e.textInput.Value()); changed {
				e.textInput.SetValue(value)
				e.textInput.CursorEnd()
			}
			return e, nil

			// Note: Tab completion removed - use FilePicker for file paths instead
		}
	}

	e.textInput, cmd = e.textInput.Update(msg)
	return e, cmd
}

// View implements tea.Model
func (e *EnhancedInput) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	view := ""
	if e.prompt != "" {
		view += titleStyle.Render(e.prompt) + "\n"
	}
	if e.description != "" {
		view += descStyle.Render(e.description) + "\n"
	}
	view += e.textInput.View() + "\n"

	return view
}

// Run executes the enhanced input and returns the value and any error
func (e *EnhancedInput) Run() (string, error) {
	p := tea.NewProgram(e)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	finalInput := finalModel.(*EnhancedInput)
	if finalInput.cancelled {
		return "", fmt.Errorf("input cancelled")
	}

	return finalInput.textInput.Value(), nil
}
