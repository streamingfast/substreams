package keymap

import "github.com/charmbracelet/bubbles/key"

var k = key.WithKeys("")
var TabShiftTab = key.NewBinding(key.WithHelp("tab/shift-tab", "Main nav"), k)
var PrevNextModule = key.NewBinding(key.WithHelp("u/i", "Modules nav"), k)
var PrevNextBlock = key.NewBinding(key.WithHelp("o/p", "Blocks nav"), k)
var Search = key.NewBinding(key.WithHelp("/", "Search"), k)
var PrevNextMatchedBlock = key.NewBinding(key.WithHelp("O/P", "Matched blocks nav"), k)
var ModuleSearch = key.NewBinding(key.WithHelp("m", "Module search"), k)
var RestartStream = key.NewBinding(key.WithHelp("r", "Restart stream"), k)
var UpDown = key.NewBinding(key.WithHelp("↑/↓/k/j", "Up/Down"), k)
var LeftRight = key.NewBinding(key.WithHelp("←/→/h/l", "Left/Right"), k)
var UpDownPage = key.NewBinding(key.WithHelp("pgup/pgdn", "Up/Down page"), k)
var ToggleLogs = key.NewBinding(key.WithHelp("L", "Toggle logs"), k)
var ToggleBytesFormat = key.NewBinding(key.WithHelp("F", "Bytes format display"), k)
var Help = key.NewBinding(key.WithHelp("?", "Help"), k)
var PrevNextSearchResult = key.NewBinding(key.WithHelp("n/N", "Prev/next search match"), k)
var Quit = key.NewBinding(key.WithHelp("q", "Quit"), k)
var ToggleProgressDisplayMode = key.NewBinding(key.WithHelp("m", "Toggle display mode"), k)
var GoToBlock = key.NewBinding(key.WithHelp("=", "Go to block"), k)
var ModGraphView = key.NewBinding(key.WithHelp("M", "Toggle module graph view"), k)
