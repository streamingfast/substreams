package styles

import (
	"github.com/charmbracelet/lipgloss"
)

type Styles struct {
	ActiveBorderColor   lipgloss.AdaptiveColor
	InactiveBorderColor lipgloss.AdaptiveColor

	StreamRunningColor lipgloss.AdaptiveColor
	StreamStoppedColor lipgloss.AdaptiveColor
	StreamErrorColor   lipgloss.AdaptiveColor

	Header               lipgloss.Style
	ServerName           lipgloss.Style
	TopLevelNormalTab    lipgloss.Style
	TopLevelActiveTab    lipgloss.Style
	TopLevelActiveTabDot lipgloss.Style

	MenuItem       lipgloss.Style
	MenuLastUpdate lipgloss.Style

	Footer      lipgloss.Style
	Branch      lipgloss.Style
	HelpKey     lipgloss.Style
	HelpValue   lipgloss.Style
	HelpDivider lipgloss.Style
	URLStyle    lipgloss.Style

	Error      lipgloss.Style
	ErrorTitle lipgloss.Style
	ErrorBody  lipgloss.Style

	Spinner lipgloss.Style

	CodeNoContent lipgloss.Style

	StatusBar       lipgloss.Style
	StatusBarKey    lipgloss.Style
	StatusBarValue  lipgloss.Style
	StatusBarInfo   lipgloss.Style
	StatusBarBranch lipgloss.Style
	StatusBarHelp   lipgloss.Style

	Tabs         lipgloss.Style
	TabInactive  lipgloss.Style
	TabActive    lipgloss.Style
	TabSeparator lipgloss.Style

	BlockSelect BlockSelectStyle
	Navigator   NavigatorStyle
	Output      OutputStyle
	ModSelect   ModSelectStyle
}

type BlockSelectStyle struct {
	Box                  lipgloss.Style
	SelectedBlock        lipgloss.Style
	CurrentBlock         lipgloss.Style
	SearchUnmatchedBlock lipgloss.Style
	SearchMatchedBlock   lipgloss.Style
}

type NavigatorStyle struct {
	SelectedModule                lipgloss.Style
	HighlightedModule             lipgloss.Style
	HighlightedUnselectableModule lipgloss.Style
	SelectableModule              lipgloss.Style
	UnselectableModule            lipgloss.Style
	Preview                       lipgloss.Style
}

type OutputStyle struct {
	LogLabel  lipgloss.Style
	LogLine   lipgloss.Style
	ErrorLine lipgloss.Style
}

type ModSelectStyle struct {
	Box               lipgloss.Style
	SelectedModule    lipgloss.Style
	HighlightedModule lipgloss.Style
	UnselectedModule  lipgloss.Style
}

// DefaultStyles returns default styles for the UI.
func DefaultStyles() *Styles {
	//highlightColor := lipgloss.Color("210")
	//highlightColorDim := lipgloss.Color("174")
	//selectorColor := lipgloss.Color("167")
	//hashColor := lipgloss.Color("185")

	s := new(Styles)

	s.ActiveBorderColor = lipgloss.AdaptiveColor{Dark: "62", Light: "81"}
	s.InactiveBorderColor = lipgloss.AdaptiveColor{Dark: "241", Light: "250"}

	s.StreamRunningColor = lipgloss.AdaptiveColor{Dark: "3", Light: "3"}
	s.StreamStoppedColor = lipgloss.AdaptiveColor{Dark: "2", Light: "10"}
	s.StreamErrorColor = lipgloss.AdaptiveColor{Dark: "9", Light: "9"}

	s.Header = lipgloss.NewStyle().
		Margin(1, 2).
		Bold(true)

	s.Tabs = lipgloss.NewStyle().
		Margin(1, 2)

	s.ServerName = lipgloss.NewStyle().
		Height(1).
		MarginLeft(1).
		MarginBottom(1).
		Padding(0, 1).
		Background(lipgloss.AdaptiveColor{Dark: "57", Light: "105"}).
		Foreground(lipgloss.AdaptiveColor{Dark: "229", Light: "246"}).
		Bold(true)

	s.TopLevelNormalTab = lipgloss.NewStyle().
		MarginRight(2)

	s.TopLevelActiveTab = s.TopLevelNormalTab.Copy().
		Foreground(lipgloss.AdaptiveColor{Dark: "36", Light: "50"})

	s.TopLevelActiveTabDot = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "36", Light: "50"})

	s.MenuItem = lipgloss.NewStyle().
		PaddingLeft(1).
		Border(lipgloss.Border{
			Left: " ",
		}, false, false, false, true).
		Height(3)

	s.MenuLastUpdate = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "241", Light: "250"}).
		Align(lipgloss.Right)

	s.Footer = lipgloss.NewStyle().
		MarginTop(1).
		Padding(0, 1).
		Height(1)

	s.Branch = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "203", Light: "210"}).
		Background(lipgloss.AdaptiveColor{Dark: "236", Light: "253"}).
		Padding(0, 1)

	s.HelpKey = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "241", Light: "250"})

	s.HelpValue = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "239", Light: "252"})

	s.HelpDivider = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "237", Light: "254"}).
		SetString(" • ")

	s.URLStyle = lipgloss.NewStyle().
		MarginLeft(1).
		Foreground(lipgloss.AdaptiveColor{Dark: "168", Light: "182"})

	s.Error = lipgloss.NewStyle().
		MarginTop(2)

	s.ErrorTitle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "230", Light: "220"}).
		Background(lipgloss.AdaptiveColor{Dark: "204", Light: "211"}).
		Bold(true).
		Padding(0, 1)

	s.ErrorBody = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "252", Light: "242"}).
		MarginLeft(2)

	s.Spinner = lipgloss.NewStyle().
		MarginTop(1).
		MarginLeft(2).
		Foreground(lipgloss.AdaptiveColor{Dark: "205", Light: "213"})

	s.CodeNoContent = lipgloss.NewStyle().
		SetString("No Content.").
		MarginTop(1).
		MarginLeft(2).
		Foreground(lipgloss.AdaptiveColor{Dark: "242", Light: "252"})

	s.StatusBar = lipgloss.NewStyle().
		Height(1)

	s.StatusBarKey = lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Background(lipgloss.AdaptiveColor{Dark: "206", Light: "229"}).
		Foreground(lipgloss.AdaptiveColor{Dark: "228", Light: "166"})

	s.StatusBarValue = lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(2).
		Background(lipgloss.AdaptiveColor{Dark: "235", Light: "253"}).
		Foreground(lipgloss.AdaptiveColor{Dark: "243", Light: "248"})

	s.StatusBarInfo = lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.AdaptiveColor{Dark: "212", Light: "219"}).
		Foreground(lipgloss.AdaptiveColor{Dark: "230", Light: "220"})

	s.StatusBarBranch = lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.AdaptiveColor{Dark: "62", Light: "81"}).
		Foreground(lipgloss.AdaptiveColor{Dark: "230", Light: "220"})

	s.StatusBarHelp = lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.AdaptiveColor{Dark: "237", Light: "254"}).
		Foreground(lipgloss.AdaptiveColor{Dark: "243", Light: "248"})

	s.Tabs = lipgloss.NewStyle().
		Height(1)

	s.TabInactive = lipgloss.NewStyle()

	s.TabActive = lipgloss.NewStyle().
		Underline(true).
		Foreground(lipgloss.AdaptiveColor{Dark: "36", Light: "50"})

	s.TabSeparator = lipgloss.NewStyle().
		SetString("│").
		Padding(0, 1).
		Foreground(lipgloss.AdaptiveColor{Dark: "238", Light: "251"})

	s.BlockSelect = BlockSelectStyle{
		Box:                  lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true),
		SelectedBlock:        lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "12", Light: "14"}).Bold(true),
		CurrentBlock:         lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "12", Light: "14"}).Bold(true),
		SearchUnmatchedBlock: lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Dark: "235", Light: "253"}),
		SearchMatchedBlock:   lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Dark: "235", Light: "253"}).Foreground(lipgloss.AdaptiveColor{Dark: "9", Light: "1"}).Bold(true),
	}

	s.Navigator = NavigatorStyle{
		SelectedModule:                lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "12", Light: "14"}).Bold(true),
		HighlightedModule:             lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "10", Light: "2"}),
		HighlightedUnselectableModule: lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "1", Light: "9"}).Faint(true),
		SelectableModule:              lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "8", Light: "7"}).Faint(false),
		UnselectableModule:            lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "1", Light: "9"}).Faint(true),
		Preview:                       lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "70", Light: "84"}).Bold(false).Faint(true),
	}

	s.Output = OutputStyle{
		LogLabel:  lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "243", Light: "248"}),
		LogLine:   lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "252", Light: "242"}),
		ErrorLine: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "1", Light: "9"}),
	}

	s.ModSelect = ModSelectStyle{
		Box:               lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true),
		SelectedModule:    lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "12", Light: "14"}).Bold(true),
		HighlightedModule: lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "21", Light: "33"}).Bold(true),
	}

	return s
}
