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
{{ with .Request }}Backprocessing history up to requested target block {{ $.BackprocessingCompleteAtBlock }}:{{- end}}
(hit 'm' to switch mode)
{{ range $key, $value := .Modules }}
{{ if $.BarMode }}
  {{- pad 25 $key }}{{ printf "%d" $value.Lo | rpad 10 }}  ::  {{ barmode $value $ }}
{{- else }}
  {{- pad 25 $key }}{{ printf "%d" $value.Lo | rpad 10 }}  ::  {{ range $value }}{{.Start}}-{{.End}} {{ end -}}
{{ end }}
{{- end -}}
{{ end }}
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
	"barmode": func(in ranges, m model) string {
		return barmode(in, m.BackprocessingCompleteAtBlock, m.BarSize)
	},
}).Parse(viewTpl))

// ▓▒░

func barmode(in ranges, backprocessingCompleteAtBlock, width uint64) string {
	lo := in.Lo()
	hi := backprocessingCompleteAtBlock
	binsize := (hi - lo) / width
	var out []string
	for i := uint64(0); i < width; i++ {
		loCheck := binsize*i + lo
		hiCheck := binsize*(i+1) + lo

		if in.Covered(loCheck, hiCheck) {
			out = append(out, "▓")
		} else if in.PartiallyCovered(loCheck, hiCheck) {
			out = append(out, "▒")
		} else {
			out = append(out, "░")
		}
	}
	return strings.Join(out, "")
}

func (m model) View() string {
	buf := bytes.NewBuffer(nil)
	err := tpl.Execute(buf, m)
	if err != nil {
		return fmt.Errorf("failed rendering template: %w", err).Error()
	}
	return buf.String()
}
