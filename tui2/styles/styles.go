package styles

import (
	"github.com/charmbracelet/lipgloss"
)

type blockSelectStyle struct {
	Box                  lipgloss.Style
	SelectedBlock        lipgloss.Style
	CurrentBlock         lipgloss.Style
	SearchUnmatchedBlock lipgloss.Style
	SearchMatchedBlock   lipgloss.Style
}

type navigatorStyle struct {
	SelectedModule                lipgloss.Style
	HighlightedModule             lipgloss.Style
	HighlightedUnselectableModule lipgloss.Style
	SelectableModule              lipgloss.Style
	UnselectableModule            lipgloss.Style
	Preview                       lipgloss.Style
}

type outputStyle struct {
	LogLabel  lipgloss.Style
	LogLine   lipgloss.Style
	ErrorLine lipgloss.Style
}

type modSelectStyle struct {
	Box               lipgloss.Style
	SelectedModule    lipgloss.Style
	HighlightedModule lipgloss.Style
	UnselectedModule  lipgloss.Style
}

//highlightColor := lipgloss.Color("210")
//highlightColorDim := lipgloss.Color("174")
//selectorColor := lipgloss.Color("167")
//hashColor := lipgloss.Color("185")

var (
	// Some colors
	purple    = lipgloss.Color("99")
	gray      = lipgloss.Color("245")
	lightGray = lipgloss.Color("241")

	// Some styles
	ActiveBorderColor = lipgloss.AdaptiveColor{Dark: "62", Light: "81"}

	InactiveBorderColor = lipgloss.AdaptiveColor{Dark: "241", Light: "250"}

	StreamRunningColor = lipgloss.AdaptiveColor{Dark: "3", Light: "3"}
	StreamStoppedColor = lipgloss.AdaptiveColor{Dark: "2", Light: "10"}
	StreamErrorColor   = lipgloss.AdaptiveColor{Dark: "9", Light: "9"}

	ServerName = lipgloss.NewStyle().
			Height(1).
			MarginLeft(1).
			MarginBottom(1).
			Padding(0, 1).
			Background(lipgloss.AdaptiveColor{Dark: "57", Light: "105"}).
			Foreground(lipgloss.AdaptiveColor{Dark: "229", Light: "246"}).
			Bold(true)

	TopLevelNormalTab = lipgloss.NewStyle().
				MarginRight(2)

	TopLevelActiveTab = TopLevelNormalTab.
				Foreground(lipgloss.AdaptiveColor{Dark: "36", Light: "50"})

	TopLevelActiveTabDot = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Dark: "36", Light: "50"})

	MenuItem = lipgloss.NewStyle().
			PaddingLeft(1).
			Border(lipgloss.Border{
			Left: " ",
		}, false, false, false, true).
		Height(3)

	MenuLastUpdate = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Dark: "241", Light: "250"}).
			Align(lipgloss.Right)

	Footer = lipgloss.NewStyle().
		MarginTop(1).
		Padding(0, 1).
		Height(1)

	HelpKey = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "241", Light: "250"})

	HelpValue = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Dark: "239", Light: "252"})

	HelpDivider = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Dark: "237", Light: "254"}).
			SetString(" • ")

	URLStyle = lipgloss.NewStyle().
			MarginLeft(1).
			Foreground(lipgloss.AdaptiveColor{Dark: "168", Light: "182"})

	Error = lipgloss.NewStyle().
		MarginTop(2)

	ErrorTitle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Dark: "230", Light: "220"}).
			Background(lipgloss.AdaptiveColor{Dark: "204", Light: "211"}).
			Bold(true).
			Padding(0, 1)

	ErrorBody = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Dark: "252", Light: "242"}).
			MarginLeft(2)

	Spinner = lipgloss.NewStyle().
		MarginTop(1).
		MarginLeft(2).
		Foreground(lipgloss.AdaptiveColor{Dark: "205", Light: "213"})

	CodeNoContent = lipgloss.NewStyle().
			SetString("No Content.").
			MarginTop(1).
			MarginLeft(2).
			Foreground(lipgloss.AdaptiveColor{Dark: "242", Light: "252"})

	StatusBar = lipgloss.NewStyle().
			Height(1)

	StatusBarKey = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.AdaptiveColor{Dark: "206", Light: "229"}).
			Foreground(lipgloss.AdaptiveColor{Dark: "228", Light: "166"})

	StatusBarValue = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(2).
			Background(lipgloss.AdaptiveColor{Dark: "235", Light: "253"}).
			Foreground(lipgloss.AdaptiveColor{Dark: "243", Light: "248"})

	StatusBarInfo = lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.AdaptiveColor{Dark: "212", Light: "219"}).
			Foreground(lipgloss.AdaptiveColor{Dark: "230", Light: "220"})

	StatusBarBranch = lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.AdaptiveColor{Dark: "62", Light: "81"}).
			Foreground(lipgloss.AdaptiveColor{Dark: "230", Light: "220"})

	StatusBarHelp = lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.AdaptiveColor{Dark: "237", Light: "254"}).
			Foreground(lipgloss.AdaptiveColor{Dark: "243", Light: "248"})

	Tabs = lipgloss.NewStyle().
		Height(1)

	TabLabel = lipgloss.NewStyle().
			Margin(0, 1)

	tabBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┴",
		BottomRight: "┴",
	}
	activeTabBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      " ",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┘",
		BottomRight: "└",
	}

	Logo = lipgloss.NewStyle().
		Border(lipgloss.Border{Bottom: "─", Left: "", BottomLeft: "─", BottomRight: "─", TopLeft: ""}).
		Padding(0, 1).
		Margin(0).
		Foreground(lipgloss.AdaptiveColor{Dark: "1", Light: "9"}).Bold(true)

	TabInactive = lipgloss.NewStyle().
			Border(tabBorder, true)

	TabActive = lipgloss.NewStyle().
			Border(activeTabBorder, true).
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Dark: "36", Light: "50"})

	TabSeparator = lipgloss.NewStyle().
			SetString("│").
			Padding(0, 1).
			Foreground(lipgloss.AdaptiveColor{Dark: "238", Light: "251"})

	RequestCell    = lipgloss.NewStyle().Padding(0, 1)
	RequestOddRow  = RequestCell.Foreground(gray)
	RequestEvenRow = RequestCell.Foreground(lightGray)
	RequestRight   = RequestCell.Align(lipgloss.Right)

	ModalBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Dark: "205", Light: "213"})

	BlockSelect = blockSelectStyle{
		Box:                  lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true),
		SelectedBlock:        lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "12", Light: "14"}).Bold(true),
		CurrentBlock:         lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "12", Light: "14"}).Bold(true),
		SearchUnmatchedBlock: lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Dark: "235", Light: "253"}),
		SearchMatchedBlock:   lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Dark: "235", Light: "253"}).Foreground(lipgloss.AdaptiveColor{Dark: "9", Light: "1"}).Bold(true),
	}

	Navigator = navigatorStyle{
		SelectedModule:                lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "12", Light: "14"}).Bold(true),
		HighlightedModule:             lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "10", Light: "2"}),
		HighlightedUnselectableModule: lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "1", Light: "9"}).Faint(true),
		SelectableModule:              lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "8", Light: "7"}).Faint(false),
		UnselectableModule:            lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "1", Light: "9"}).Faint(true),
		Preview:                       lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "70", Light: "84"}).Bold(false).Faint(true),
	}

	Output = outputStyle{
		LogLabel:  lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "243", Light: "248"}),
		LogLine:   lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "252", Light: "242"}),
		ErrorLine: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "1", Light: "9"}),
	}

	ModSelect = modSelectStyle{
		Box:               lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true),
		SelectedModule:    lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "12", Light: "14"}).Bold(true),
		HighlightedModule: lipgloss.NewStyle().Margin(0, 2).Foreground(lipgloss.AdaptiveColor{Dark: "21", Light: "33"}).Bold(true),
	}
)
