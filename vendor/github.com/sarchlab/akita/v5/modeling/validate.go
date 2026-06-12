package modeling

import (
	"fmt"
	"reflect"
)

// ValidateSpec checks that the given value is a struct containing only
// primitive fields (bool, int*, uint*, float*, string), slices of primitives,
// and maps with string keys and primitive values. No pointers, interfaces,
// channels, or functions are allowed.
func ValidateSpec(v any) error {
	return validateValue(reflect.ValueOf(v), "spec", false)
}

// ValidateState checks that the given value is a struct containing only
// primitive fields, simple nested structs, slices of primitives or structs,
// and maps with string keys. Pointers, interfaces, channels, and functions
// are not allowed. This is slightly more permissive than ValidateSpec in that
// it allows nested structs.
func ValidateState(v any) error {
	return validateValue(reflect.ValueOf(v), "state", true)
}

func validateValue(v reflect.Value, path string, allowNestedStructs bool) error {
	if !v.IsValid() {
		return fmt.Errorf("%s: invalid value", path)
	}

	t := v.Type()

	switch t.Kind() {
	case reflect.Struct:
		return validateStruct(v, path, allowNestedStructs)
	default:
		return fmt.Errorf("%s: expected struct, got %s", path, t.Kind())
	}
}

func validateStruct(v reflect.Value, path string, allowNestedStructs bool) error {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if tag := field.Tag.Get("json"); tag == "-" {
			continue
		}

		fieldPath := fmt.Sprintf("%s.%s", path, field.Name)

		if err := validateFieldType(field.Type, fieldPath, allowNestedStructs); err != nil {
			return err
		}
	}

	return nil
}

func validateFieldType(t reflect.Type, path string, allowNestedStructs bool) error {
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return nil

	case reflect.Slice:
		return validateSliceElement(t.Elem(), path, allowNestedStructs)

	case reflect.Map:
		k := t.Key().Kind()
		if k != reflect.String &&
			k != reflect.Uint64 && k != reflect.Uint && k != reflect.Uint32 &&
			k != reflect.Int64 && k != reflect.Int && k != reflect.Int32 {
			return fmt.Errorf("%s: map key must be string or integer, got %s", path, k)
		}

		return validateMapValue(t.Elem(), path, allowNestedStructs)

	case reflect.Struct:
		if !allowNestedStructs {
			return fmt.Errorf("%s: nested structs not allowed in spec", path)
		}

		// Validate the nested struct's fields recursively.
		return validateStructType(t, path, allowNestedStructs)

	case reflect.Ptr, reflect.Interface, reflect.Chan, reflect.Func:
		return fmt.Errorf("%s: disallowed kind %s", path, t.Kind())

	default:
		return fmt.Errorf("%s: unsupported kind %s", path, t.Kind())
	}
}

func validateSliceElement(elem reflect.Type, path string, allowNestedStructs bool) error {
	switch elem.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return nil

	case reflect.Struct:
		if !allowNestedStructs {
			return fmt.Errorf("%s: slice of structs not allowed in spec", path)
		}

		return validateStructType(elem, path+"[]", allowNestedStructs)

	default:
		return fmt.Errorf("%s: slice element must be a primitive (or struct in state), got %s",
			path, elem.Kind())
	}
}

func validateMapValue(elem reflect.Type, path string, allowNestedStructs bool) error {
	switch elem.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return nil

	case reflect.Struct:
		if !allowNestedStructs {
			return fmt.Errorf("%s: map of structs not allowed in spec", path)
		}

		return validateStructType(elem, path+"[value]", allowNestedStructs)

	default:
		return fmt.Errorf("%s: map value must be a primitive (or struct in state), got %s",
			path, elem.Kind())
	}
}

func validateStructType(t reflect.Type, path string, allowNestedStructs bool) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if tag := field.Tag.Get("json"); tag == "-" {
			continue
		}

		fieldPath := fmt.Sprintf("%s.%s", path, field.Name)

		if err := validateFieldType(field.Type, fieldPath, allowNestedStructs); err != nil {
			return err
		}
	}

	return nil
}
