package graphnode

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func ApplyTableChange(change *pbsubstreams.TableChange, entity Entity) (err error) {
	rv := indirect(reflect.ValueOf(entity), false)
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
		return
	case reflect.Int8:
		var n int64
		n, err = strconv.ParseInt(fieldChange.NewValue, 10, 8)
		rv.SetInt(n)
		return
	case reflect.Int16:
		var n int64
		n, err = strconv.ParseInt(fieldChange.NewValue, 10, 16)
		rv.SetInt(n)
		return
	case reflect.Int32:
		var n int64
		n, err = strconv.ParseInt(fieldChange.NewValue, 10, 32)
		rv.SetInt(n)
		return
	case reflect.Int64:
		var n int64
		n, err = strconv.ParseInt(fieldChange.NewValue, 10, 64)
		rv.SetInt(n)
		return
	case reflect.Uint8:
		var n uint64
		n, err = strconv.ParseUint(fieldChange.NewValue, 10, 8)
		rv.SetUint(n)
		return
	case reflect.Uint16:
		var n uint64
		n, err = strconv.ParseUint(fieldChange.NewValue, 10, 16)
		rv.SetUint(n)
		return
	case reflect.Uint32:
		var n uint64
		n, err = strconv.ParseUint(fieldChange.NewValue, 10, 32)
		rv.SetUint(n)
		return
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
		switch rt.String() { //todo: this is very bad. Soo sorry
		case "graphnode.Int":
			var n int64
			n, err = strconv.ParseInt(fieldChange.NewValue, 10, 64)
			rv.Set(reflect.ValueOf(NewIntFromLiteral(n)))
		case "graphnode.Float":
			var n float64
			n, err = strconv.ParseFloat(fieldChange.NewValue, 64)
			rv.Set(reflect.ValueOf(NewFloatFromLiteral(n)))
		default:

			panic(fmt.Sprintf("nested structure not supported %q", rt))
		}
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

func indirect(v reflect.Value, decodingNull bool) reflect.Value {
	// Issue #24153 indicates that it is generally not a guaranteed property
	// that you may round-trip a reflect.Value by calling Value.Addr().Elem()
	// and expect the value to still be settable for values derived from
	// unexported embedded struct fields.
	//
	// The logic below effectively does this when it first addresses the value
	// (to satisfy possible pointer methods) and continues to dereference
	// subsequent pointers as necessary.
	//
	// After the first round-trip, we set v back to the original value to
	// preserve the original RW flags contained in reflect.Value.
	v0 := v
	haveAddr := false

	// If v is a named type and is addressable,
	// start with its address, so that if the type has pointer methods,
	// we find them.
	if v.Kind() != reflect.Ptr && v.Type().Name() != "" && v.CanAddr() {
		haveAddr = true
		v = v.Addr()
	}
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() && (!decodingNull || e.Elem().Kind() == reflect.Ptr) {
				haveAddr = false
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Ptr {
			break
		}

		if v.Elem().Kind() != reflect.Ptr && decodingNull && v.CanSet() {
			break
		}

		// Prevent infinite loop if v is an interface pointing to its own address:
		//     var v interface{}
		//     v = &v
		if v.Elem().Kind() == reflect.Interface && v.Elem().Elem() == v {
			v = v.Elem()
			break
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}

		if haveAddr {
			v = v0 // restore original value after round-trip Value.Addr().Elem()
			haveAddr = false
		} else {
			v = v.Elem()
		}
	}
	return v
}
