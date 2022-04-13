package graphnode

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func ApplyTableChange(change *pbsubstreams.TableChange, entity Entity) (err error) {
	rv := reflect.ValueOf(entity)
	rt := rv.Type()
	fieldChanges := map[string]*pbsubstreams.Field{}
	for _, field := range change.Fields {
		fieldChanges[field.Name] = field
	}

	l := rv.NumField()
	for i := 0; i < l; i++ {
		structField := rt.Field(i)
		fieldTag := parseFieldTag(structField.Tag)
		v := rv.Field(i)

		if field, found := fieldChanges[fieldTag.dbFieldName]; found {
			if err = applyTableChange(v, field, fieldTag); err != nil {
				return
			}
		}
	}
	return nil
}

func applyTableChange(rv reflect.Value, fieldChange *pbsubstreams.Field, fieldTags *FieldTags) (err error) {
	if fieldTags == nil {
		fieldTags = &FieldTags{}
	}

	rt := rv.Type()

	switch rv.Kind() {
	case reflect.String:
		rv.SetString(fieldChange.NewValue)
	case reflect.Int8:
		var n int64
		n, err = strconv.ParseInt(fieldChange.NewValue, 10, 8)
		rv.SetInt(n)
	case reflect.Int16:
		var n int64
		n, err = strconv.ParseInt(fieldChange.NewValue, 10, 16)
		rv.SetInt(n)
	case reflect.Int32:
		var n int64
		n, err = strconv.ParseInt(fieldChange.NewValue, 10, 32)
		rv.SetInt(n)
	case reflect.Int64:
		var n int64
		n, err = strconv.ParseInt(fieldChange.NewValue, 10, 64)
		rv.SetInt(n)
	case reflect.Uint8:
		var n uint64
		n, err = strconv.ParseUint(fieldChange.NewValue, 10, 8)
		rv.SetUint(n)
	case reflect.Uint16:
		var n uint64
		n, err = strconv.ParseUint(fieldChange.NewValue, 10, 16)
		rv.SetUint(n)
	case reflect.Uint32:
		var n uint64
		n, err = strconv.ParseUint(fieldChange.NewValue, 10, 32)
		rv.SetUint(n)
	case reflect.Uint64:
		var n uint64
		n, err = strconv.ParseUint(fieldChange.NewValue, 10, 64)
		rv.SetUint(n)
	case reflect.Float32:
		var n float64
		n, err = strconv.ParseFloat(fieldChange.NewValue, 32)
		rv.SetFloat(n)
		return
	case reflect.Float64:
		var n float64
		n, err = strconv.ParseFloat(fieldChange.NewValue, 64)
		rv.SetFloat(n)
		return
	case reflect.Bool:
		var b bool
		b, err = strconv.ParseBool(fieldChange.NewValue)
		rv.SetBool(b)
		return
	}
	switch rt.Kind() {
	case reflect.Array:
		//{" + strings.Join(*b, ",") + "} //todo: handle array from json string
		return
	case reflect.Slice:
		//{" + strings.Join(*b, ",") + "} //todo: handle array from json string

	case reflect.Struct:
		panic("nested structure not supported")
		//if err = applyTableChangeStruct(rt, rv, changes); err != nil {
		//	return
		//}

	default:
		return fmt.Errorf("decode: unsupported type %q", rt)
	}

	return
}

type FieldTags struct {
	dbFieldName  string
	dbOptional   bool
	cvsFieldName string
}

//`db:"derived_usd,nullable" csv:"derived_usd"`
func parseFieldTag(tag reflect.StructTag) *FieldTags {
	tags := &FieldTags{}

	tagStr := tag.Get("db")
	parts := strings.Split(tagStr, ",")
	tags.dbFieldName = parts[0]
	tags.dbOptional = len(parts) == 2 && parts[1] == "nullable"

	tagStr = tag.Get("cvs")
	tags.cvsFieldName = tagStr

	return tags
}
