package tui

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/dustin/go-humanize"
)

func (m model) View() string {
	const width = 80

	// WARN(abourget): Request.StartBlockNum cannot be relative here too.

	buf := bytes.NewBuffer(nil)
	err := template.Must(template.New("tpl").Funcs(template.FuncMap{
		"pad": func(in string) string {
			l := len(in)
			if l > 25 {
				return in[:25]
			}
			return in + strings.Repeat(" ", 25-l)
		},
		"humanize": func(in uint64) string {
			return humanize.Comma(int64(in))
		},
		"linebar": func(ranges ranges, m model) string {
			return linebar(ranges, m.Modules.Lo(), uint64(m.Request.StartBlockNum), m.screenWidth)
		},
	}).Parse(`{{ if not .Clock -}}
{{- if not .Connected }}Connecting...{{ else -}}
Connected - Progress messages received: {{ .Updates }}
{{- if .Failures }}   Failures: {{ .Failures }}, Reason: {{ .Reason }} {{ end }}
{{ with .Request }}Backprocessing history up to requested start block {{ .StartBlockNum }}:
(hit 'm' to switch display mode){{end}}
{{ range $key, $value := .Modules }}
{{- if not $.BarMode }}
  {{ pad $key }} {{ $value.Lo }}  ::  {{ range $value }}{{.Start}}-{{.End}} {{ end -}}
{{- else }}
  {{ pad $key }} {{ $value.Lo }}  ::  {{ linebar $value $ -}}
{{- end }}
{{ end }}{{ end }}{{ end }}
{{ with .Clock -}}
-------------------- BLOCK {{ humanize .Number }} --------------------
{{ end -}}
`)).Execute(buf, m)
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
