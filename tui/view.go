package tui

import (
	"embed"

	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/dustin/go-humanize"
)

//go:embed *.txt.gotmpl
var viewTplFS embed.FS

var tpl = template.Must(template.New("main_view.txt.gotmpl").Funcs(template.FuncMap{
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
}).ParseFS(viewTplFS, "*.txt.gotmpl"))

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
