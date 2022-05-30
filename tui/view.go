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
	}).Parse(`{{ if not .Clock -}}
{{- if not .Connected }}Connecting...{{ else -}}
Connected - Progress messages received: {{ .Updates }}
{{- if .Failures }}   Failures: {{ .Failures }}, Reason: {{ .Reason }} {{ end }}
Backprocessing history up to request start block:
{{ range $key, $value := .Modules }}
  {{ pad $key }} {{ $value.Lo }}, {{ $value.Hi }} - {{ range $value }}{{.Start}}-{{.End}} {{ end -}}

{{ end }}{{ end }}{{ end }}{{- with .Clock -}}
-------------------- BLOCK {{ humanize .Number }} --------------------
{{ end -}}
`)).Execute(buf, m)
	if err != nil {
		return fmt.Errorf("failed rendering template: %w", err).Error()
	}
	return buf.String()
}
