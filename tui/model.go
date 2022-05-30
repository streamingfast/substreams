package tui

import (
	"bytes"
	"fmt"
	"html/template"

	tea "github.com/charmbracelet/bubbletea"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func newModel() model {
	return model{
		Modules: updatedRanges{},
	}
}

type model struct {
	Modules      updatedRanges
	DebugSetting bool
	EventNo      int
	Updates      int

	Failures int
	Reason   string

	progress chan *pbsubstreams.ModuleProgress
}
type ranges []blockRange

func (r ranges) Len() int           { return len(r) }
func (r ranges) Less(i, j int) bool { return r[i].Start < r[j].Start }
func (r ranges) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }

func (r ranges) LoHi() (uint64, uint64) {
	if len(r) == 0 {
		return 0, 0
	}
	return r[0].Start, r[len(r)-1].End
}
func (r ranges) Lo() uint64 { a, _ := r.LoHi(); return a }
func (r ranges) Hi() uint64 { _, b := r.LoHi(); return b }

// Covered assumes block ranges have reduced overlaps/junctions.
func (r ranges) Covered(lo, hi uint64) bool {
	for _, blockRange := range r {
		if lo >= blockRange.Start && hi <= blockRange.End {
			return true
		}
	}
	return false
}

type blockRange struct {
	Start uint64
	End   uint64
}

type updatedRanges map[string]ranges

// LoHi returns the lowest and highest of all modules. The global span,
// used to determine the width and the divider of each printable cell.
func (u updatedRanges) LoHi() (lo uint64, hi uint64) {
	var loset bool
	for _, v := range u {
		tlo, thi := v.LoHi()
		if thi > hi {
			hi = thi
		}
		if !loset {
			lo = tlo
			loset = true
		} else if tlo < lo {
			lo = tlo
		}
	}
	return
}

func (u updatedRanges) Lo() uint64 { a, _ := u.LoHi(); return a }
func (u updatedRanges) Hi() uint64 { _, b := u.LoHi(); return b }

type newRange map[string]blockRange

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.EventNo += 1

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlBackslash:
			fmt.Println("Quitting...")
			return m, tea.Quit
		}
		switch msg.String() {
		case "m":
			m.DebugSetting = !m.DebugSetting
		case "f":
			m.Failures += 1
		case "q":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		// m.progress.Width = msg.Width - padding*2 - 4
		// if m.progress.Width > maxWidth {
		// 	m.progress.Width = maxWidth
		// }
	case *pbsubstreams.ModuleProgress:
		m.Updates += 1
		switch progMsg := msg.Type.(type) {
		case *pbsubstreams.ModuleProgress_ProcessedRanges:
			newModules := updatedRanges{}
			for k, v := range m.Modules {
				newModules[k] = v
			}

			for _, v := range progMsg.ProcessedRanges.ProcessedRanges {
				newModules[msg.Name] = mergeRangeLists(newModules[msg.Name], blockRange{
					Start: v.StartBlock,
					End:   v.EndBlock,
				})
			}

			m.Modules = newModules
		case *pbsubstreams.ModuleProgress_InitialState_:
		case *pbsubstreams.ModuleProgress_ProcessedBytes_:
		case *pbsubstreams.ModuleProgress_Failed_:
			m.Failures += 1
			if progMsg.Failed.Reason != "" {
				m.Reason = fmt.Sprintf("Reason: %s, logs: %s, truncated: %v", progMsg.Failed.Reason, progMsg.Failed.Logs, progMsg.Failed.LogsTruncated)
			}
			return m, nil
		}
	default:
	}

	return m, nil
}

func (m model) View() string {
	const width = 80

	buf := bytes.NewBuffer(nil)
	err := template.Must(template.New("tpl").Parse(`
DebugSetting: [{{ with .DebugSetting }}X{{ else }} {{ end }}]
Event no: {{ .EventNo }} {{- if .Failures }}   Failures: {{ .Failures }}, Reason: {{ .Reason }} {{ end }}
Updates: {{ .Updates }}
{{ range $key, $value := .Modules }}
  {{ $key }}       {{ $value.Lo }}, {{ $value.Hi }} - {{ range $value }}{{.Start}}-{{.End}} {{ end -}}
{{ end }}
`)).Execute(buf, m)
	if err != nil {
		return fmt.Errorf("failed rendering template: %w", err).Error()
	}
	return buf.String()
}
