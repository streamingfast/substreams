package tui

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/dustin/go-humanize"
)

var viewTpl = `
{{- if not .Connected }}Connecting...{{ else -}}
Connected - Progress messages received: {{ .Updates }} ({{ .UpdatesPerSecond }}/sec)
{{ with .Request }}Backprocessing history up to requested start block {{ .StartBlockNum }}:
(hit 'm' to switch display mode){{end}}
{{ range $key, $value := .Modules }}
{{- if $.BarMode }}
  {{ pad 25 $key }}{{ printf "%d" $value.Lo | rpad 10 }}  ::  {{ range $value }}{{.Start}}-{{.End}} {{ end }}
{{- else }}
  {{ pad 25 $key }}{{ printf "%d" $value.Lo | rpad 10 }}  ::  {{ linebar $value $ }}
{{- end -}}
{{ end }}{{ end }}
{{ if .Failures }}
Failures: {{ .Failures }}.
Last failure:
  Reason: {{ .LastFailure.Reason }}
  Logs:
{{ range .LastFailure.Logs }}
    {{ . }}
{{ end }}
{{- with .LastFailure.LogsTruncated }}  <logs truncated>{{ end }}
{{ end -}}
`

var tpl = template.Must(template.New("tpl").Funcs(template.FuncMap{
	"pad": func(max int, in string) string {
		l := len(in)
		if l > max {
			return in[:max]
		}
		return in + strings.Repeat(" ", max-l)
	},
	"rpad": func(max int, in string) string {
		l := len(in)
		if l > max {
			return in[:max]
		}
		return strings.Repeat(" ", max-l) + in
	},
	"humanize": func(in uint64) string {
		return humanize.Comma(int64(in))
	},
	"linebar": func(ranges ranges, m model) string {
		return linebar(ranges, m.Modules.Lo(), uint64(m.Request.StartBlockNum), m.screenWidth)
	},
}).Parse(viewTpl))

func (m model) View() string {
	// WARN(abourget): Request.StartBlockNum cannot be relative here too.

	buf := bytes.NewBuffer(nil)
	err := tpl.Execute(buf, m)
	if err != nil {
		return fmt.Errorf("failed rendering template: %w", err).Error()
	}
	return buf.String()
}

func linebar(ranges ranges, initialBlock uint64, startBlock uint64, screenWidth int) string {
	// Make it 4 times more granular, with the Quadrants here: https://www.compart.com/en/unicode/block/U+2580
	blocksWidth := startBlock - initialBlock
	binSize := float64(blocksWidth) / float64(screenWidth)
	prevBound := initialBlock
	var s []string
	for i := 0; i < screenWidth; i++ {
		nextBound := initialBlock + uint64(binSize*float64(i+1))
		//fmt.Print("bounds", prevBound, nextBound)
		if ranges.Covered(prevBound, nextBound) {
			//fmt.Println(" covered")
			s = append(s, "▓")
		} else if ranges.PartiallyCovered(prevBound, nextBound) {
			//fmt.Println(" partial")
			s = append(s, "▒")
		} else {
			//fmt.Println("")
			s = append(s, "░")
		}
		prevBound = nextBound
	}
	return strings.Join(s, "")
}
