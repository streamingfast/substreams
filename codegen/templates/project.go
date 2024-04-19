package templates

import (
	"strings"
	"text/template"
)

type Project interface {
	Render() (map[string][]byte, error)
}

type ProjectFunc func() (map[string][]byte, error)

func (f ProjectFunc) Render() (map[string][]byte, error) {
	return f()
}

var ProjectGeneratorFuncs = template.FuncMap{
	"add": func(left int, right int) int {
		return left + right
	},
	"sanitizeProtoFieldName": sanitizeProtoFieldName,
	"toUpper":                strings.ToUpper,
	"capitalizeFirst":        strings.Title,
}
