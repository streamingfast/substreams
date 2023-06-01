package keymap

import "github.com/charmbracelet/bubbles/key"

var k = key.WithKeys("")
var PrevNextModule = key.NewBinding(key.WithHelp("u/i", "nav. modules"), k)
var PrevNextBlock = key.NewBinding(key.WithHelp("o/p", "nav. blocks"), k)
var Search = key.NewBinding(key.WithHelp("/", "search"), k)
var PrevNextMatchedBlock = key.NewBinding(key.WithHelp("O/P", "nav. matched blocks"), k)
var ModuleSearch = key.NewBinding(key.WithHelp("m", "module search"), k)
var RestartStream = key.NewBinding(key.WithHelp("r", "restart stream"), k)
var UpDown = key.NewBinding(key.WithHelp("↑/↓/k/j", "up/down"), k)
var LeftRight = key.NewBinding(key.WithHelp("←/→/h/l", "left/right"), k)
var UpDownPage = key.NewBinding(key.WithHelp("pgup/pgdn", "up/down page"), k)
var ToggleLogs = key.NewBinding(key.WithHelp("L", "toggle logs"), k)
var ToggleBytesFormat = key.NewBinding(key.WithHelp("F", "bytes format"), k)
var Help = key.NewBinding(key.WithHelp("?", "toggle help"), k)
var PrevNextSearchResult = key.NewBinding(key.WithHelp("n/N", "prev/next search match"), k)
var Quit = key.NewBinding(key.WithHelp("q", "quit"), k)
var ToggleProgressDisplayMode = key.NewBinding(key.WithHelp("m", "toggle display mode"), k)
var GoToBlock = key.NewBinding(key.WithHelp("=", "go to block"), k)
var ModGraphView = key.NewBinding(key.WithHelp("M", "toggle mod graph view"), k)
