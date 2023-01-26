package main

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/cli"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/codegen"
)

var initCmd = &cobra.Command{
	Use:   "init [<path>]",
	Short: "Initialize a new, working Substreams project from scratch.",
	Long: cli.Dedent(`
		Initialize a new, working Substreams project from scratch. The path parameter is optional,
		with your current working directory being the default value.
	`),
	RunE:         runSubstreamsInitE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func init() {
	alphaCmd.AddCommand(initCmd)
}

type (
	errMsg error
)
type ChoiceModel struct {
	questionContext string
	choices         []string
	cursor          int
	selected        string
}
type InputModel struct {
	textInput textinput.Model
	err       error
}

func newChainSelection() ChoiceModel {
	return ChoiceModel{
		questionContext: "chain",
		choices:         []string{"Ethereum", "other"},
	}
}
func projectNameSelection() InputModel {
	ti := textinput.New()
	ti.Placeholder = "Project name"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	return InputModel{
		textInput: ti,
		err:       nil,
	}
}

func (m ChoiceModel) Init() tea.Cmd {
	return nil
}
func (m InputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ChoiceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:

		switch msg.String() {

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			ok := m.selected == m.choices[m.cursor]
			if ok {
				return m, tea.Quit
			} else {
				m.selected = m.choices[m.cursor]
			}
		}
	}
	return m, nil
}
func (m InputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter, tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}

	case errMsg:
		m.err = msg
		return m, nil
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m ChoiceModel) View() string {
	output := fmt.Sprintf("What %s would you like your generated substream to be\n\n", m.questionContext)

	for i, choice := range m.choices {

		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		checked := " "
		if m.selected == m.choices[i] {
			checked = "x"
		}
		output += fmt.Sprintf("%s [%s] %s\n", cursor, checked, choice)
	}

	output += "\nPress enter again to continue.\n"

	return output
}
func (m InputModel) View() string {
	return fmt.Sprintf(
		"What would you like your project to be named?\n\n%s\n\n%s",
		m.textInput.View(),
		"(press 'enter' to continue)",
	) + "\n"
}

func runSubstreamsInitE(cmd *cobra.Command, args []string) error {
	srcDir, err := filepath.Abs("/tmp/test")
	if err != nil {
		return fmt.Errorf("getting absolute path of working directory: %w", err)
	}
	// if len(args) == 1 {
	// 	srcDir, err = filepath.Abs(args[0])
	// 	if err != nil {
	// 		return fmt.Errorf("getting absolute path of given directory: %w", err)
	// 	}
	// }

	// // Bubble Tea model to select project name
	// projectNameModel, err := tea.NewProgram(projectNameSelection()).Run()
	// if err != nil {
	// 	return fmt.Errorf("creating name selector: %w", err)
	// }
	// projectNameExposed := projectNameModel.(InputModel)
	// nameSelected := projectNameExposed.textInput.Value()

	// // Bubble Tea model to select chain for template
	// chainModel, err := tea.NewProgram(newChainSelection()).Run()
	// if err != nil {
	// 	return fmt.Errorf("creating chain selector: %w", err)
	// }
	// chainModelExposed := chainModel.(ChoiceModel)
	// chainSelected := chainModelExposed.selected

	// if chainSelected != "Ethereum" {
	// 	fmt.Println("We haven't added any templates for your selected chain quite yet...")
	// 	fmt.Println("Come join us in discord at https://discord.gg/u8amUbGBgF and suggest templates/chains you want to see!")
	// 	return nil
	// }

	gen := codegen.NewProjectGenerator(srcDir, "testting")
	err = gen.GenerateProjectTest()
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	return nil
}
