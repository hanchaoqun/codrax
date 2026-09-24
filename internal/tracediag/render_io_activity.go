package tracediag

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Only the closed IOActivity DTO family reaches this renderer. Follow its
// reviewed JSON presence rules: required zero counts/coordinates survive,
// optional nil measures stay absent, and no rate/size/scope is recomputed.
func renderIOActivityDetail(v reflect.Value, path string, emit func(string), depth int, policy *detailRenderPolicy) {
	t := v.Type()
	var tokens []string
	for i := 0; i < v.NumField(); i++ {
		field, value := t.Field(i), v.Field(i)
		if field.PkgPath != "" || jsonExcluded(field) {
			continue
		}
		if strings.Contains(field.Tag.Get("json"), ",omitempty") && isZeroValue(value) {
			continue
		}
		if value.Kind() == reflect.Pointer {
			if value.IsNil() || !isScalarKind(value.Type().Elem().Kind()) {
				continue
			}
			value = value.Elem()
		}
		if !isScalarKind(value.Kind()) {
			continue
		}
		tag := jsonTagName(field)
		token := formatScalarForTag(value, tag)
		if value.Kind() == reflect.Float64 {
			token = strconv.FormatFloat(value.Float(), 'f', -1, 64)
		}
		tokens = append(tokens, tag+"="+token)
	}
	if len(tokens) != 0 {
		emit(fmt.Sprintf("- %s: %s", path, strings.Join(tokens, " ")))
	}
	for _, i := range orderedDetailFieldIndexes(v, policy) {
		field, value := t.Field(i), v.Field(i)
		if field.PkgPath != "" || jsonExcluded(field) {
			continue
		}
		if value.Kind() == reflect.Pointer && (value.IsNil() || isScalarKind(value.Type().Elem().Kind())) {
			continue
		}
		switch value.Kind() {
		case reflect.Struct, reflect.Pointer, reflect.Slice, reflect.Array:
			walkDetailWithPolicy(value, path+"."+jsonTagName(field), emit, depth+1, policy)
		}
	}
}
