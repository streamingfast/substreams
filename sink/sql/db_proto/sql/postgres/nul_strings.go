package postgres

import (
	"strings"

	"go.uber.org/zap"
)

// jsonNul is how protojson writes a NUL character inside a JSON string.
const jsonNul = `\u0000`

// removeNulCharacters takes the NUL characters out of the strings in a row: PostgreSQL
// rejects them in TEXT, VARCHAR and JSONB values, so a row carrying one cannot be stored
// as it is. Each column it changes is logged the first time.
func (d *Database) removeNulCharacters(table string, values []any) {
	layout := d.rowLayouts[table]

	for position, value := range values {
		isJSON := false
		if position < len(layout) && layout[position].column != nil {
			isJSON = layout[position].column.Nested != nil
		}

		cleaned, changed := withoutNul(value, isJSON)
		if !changed {
			continue
		}
		values[position] = cleaned

		column := ""
		if position < len(layout) {
			column = layout[position].name
		}
		key := table + "." + column
		if _, logged := d.nulColumnsLogged[key]; !logged {
			d.nulColumnsLogged[key] = struct{}{}
			d.logger.Warn("removed NUL characters from a string, PostgreSQL cannot store them; later occurrences in this column are not logged",
				zap.String("table", table), zap.String("column", column))
		}
	}
}

// withoutNul returns value with the NUL characters taken out of its strings. A JSON value
// holds them escaped, as \u0000.
func withoutNul(value any, isJSON bool) (any, bool) {
	switch v := value.(type) {
	case string:
		if isJSON {
			return jsonWithoutNul(v)
		}
		return stringWithoutNul(v)

	case []any:
		changed := false
		for i, element := range v {
			if s, ok := element.(string); ok {
				if cleaned, elementChanged := stringWithoutNul(s); elementChanged {
					v[i] = cleaned
					changed = true
				}
			}
		}
		return v, changed

	case postgresTextArray:
		changed := false
		for i, element := range v {
			if cleaned, elementChanged := jsonWithoutNul(element); elementChanged {
				v[i] = cleaned
				changed = true
			}
		}
		return v, changed
	}

	return value, false
}

func stringWithoutNul(s string) (string, bool) {
	if strings.IndexByte(s, 0) < 0 {
		return s, false
	}

	return strings.ReplaceAll(s, "\x00", ""), true
}

// jsonWithoutNul removes the \u0000 escapes from a JSON document. It reads escapes pair by
// pair so an escaped backslash followed by u0000, which is text, is kept.
func jsonWithoutNul(s string) (string, bool) {
	if !strings.Contains(s, jsonNul) {
		return s, false
	}

	var b strings.Builder
	b.Grow(len(s))
	changed := false
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if strings.HasPrefix(s[i:], jsonNul) {
			i += len(jsonNul) - 1
			changed = true
			continue
		}
		b.WriteByte(s[i])
		if i+1 < len(s) {
			i++
			b.WriteByte(s[i])
		}
	}

	return b.String(), changed
}
