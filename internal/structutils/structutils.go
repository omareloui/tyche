package structutils

import (
	"fmt"
	"reflect"
)

func StructsToMap[T any](structs []T, key, val string) map[string]string {
	m := make(map[string]string, len(structs))
	for _, stru := range structs {
		v := reflect.ValueOf(stru)
		kf := v.FieldByName(key)
		vf := v.FieldByName(val)

		if !kf.IsValid() || !vf.IsValid() || kf.Kind() != reflect.String {
			continue
		}

		var key string
		var val string

		if vf.Kind() == reflect.String {
			val = vf.String()
		} else {
			iface := vf.Interface()
			if s, ok := iface.(fmt.Stringer); ok {
				val = s.String()
			} else {
				val = fmt.Sprint(iface)
			}
		}

		if kf.Kind() == reflect.String {
			key = kf.String()
		} else {
			iface := kf.Interface()
			if s, ok := iface.(fmt.Stringer); ok {
				key = s.String()
			} else {
				key = fmt.Sprint(iface)
			}
		}

		if key != "" && val != "" {
			m[key] = val
		}
	}

	return m
}
