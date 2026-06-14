package modeling

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// jsonMarshalerType is the reflect.Type of json.Marshaler, used to exempt types
// that customize their own JSON from the structural and data-loss checks.
var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

// jsonUnmarshalerType is the reflect.Type of json.Unmarshaler. A type that
// customizes MarshalJSON must also customize UnmarshalJSON: round-tripping is
// two independent mechanisms, and with only the marshal half a checkpoint
// saves the custom payload and then silently restores zero values (the
// default decoder only sets exported fields).
var jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

// validateForCheckpoint checks a component's Spec and State so a mis-modeled
// component fails loudly at construction rather than silently producing a wrong
// resume. It panics — like the other builder misconfiguration guards — because a
// non-serializable Spec/State is a programming error, not a runtime condition.
func validateForCheckpoint[S, T any](name string, spec S) {
	if err := ValidateSpec(spec); err != nil {
		panic(fmt.Sprintf(
			"modeling: component %q has a Spec that cannot be checkpointed: %v",
			name, err))
	}

	var zeroState T
	if err := ValidateState(zeroState); err != nil {
		panic(fmt.Sprintf(
			"modeling: component %q has a State that cannot be checkpointed: %v",
			name, err))
	}
}

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

	if t.Kind() != reflect.Struct {
		return fmt.Errorf("%s: expected struct, got %s", path, t.Kind())
	}

	return validateStructType(t, path, allowNestedStructs)
}

func validateFieldType(t reflect.Type, path string, allowNestedStructs bool) error {
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return nil

	case reflect.Slice, reflect.Array:
		return validateFieldType(t.Elem(), path+"[]", allowNestedStructs)

	case reflect.Map:
		k := t.Key().Kind()
		if k != reflect.String &&
			k != reflect.Uint64 && k != reflect.Uint && k != reflect.Uint32 &&
			k != reflect.Int64 && k != reflect.Int && k != reflect.Int32 {
			return fmt.Errorf("%s: map key must be string or integer, got %s", path, k)
		}

		return validateFieldType(t.Elem(), path+"[value]", allowNestedStructs)

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

func validateStructType(t reflect.Type, path string, allowNestedStructs bool) error {
	// A type that customizes its own JSON is trusted: it round-trips on its own
	// terms, so neither the structural rules nor the data-loss guard apply —
	// provided both halves of the round trip exist. UnmarshalJSON must be
	// checked on the pointer type: it mutates the value, so it always has a
	// pointer receiver, while MarshalJSON typically has a value receiver.
	if t.Implements(jsonMarshalerType) {
		if !reflect.PointerTo(t).Implements(jsonUnmarshalerType) {
			return fmt.Errorf(
				"%s: type %s customizes MarshalJSON but has no UnmarshalJSON, "+
					"so a checkpoint saves its custom payload and then silently "+
					"restores zero values (the default decoder only sets "+
					"exported fields); implement the pair",
				path, t)
		}

		return nil
	}

	// Data-loss guard: a struct whose state is entirely unexported, with no
	// MarshalJSON, serializes as {} and silently drops its contents across a
	// checkpoint (the lruset.Set class of bug). Catch it at validation time
	// instead of as a wrong resume.
	if serializesToEmpty(t) {
		return fmt.Errorf(
			"%s: type %s has unexported state but no MarshalJSON, so it "+
				"serializes as {} and would silently lose that state across a "+
				"checkpoint; add MarshalJSON/UnmarshalJSON or export the fields",
			path, t)
	}

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

// serializesToEmpty reports whether a struct type holds unexported fields yet
// marshals to an empty JSON object — meaning encoding/json silently drops all of
// it. Types that implement json.Marshaler are handled by the caller and never
// reach here. The partial case (some exported, some unexported fields) is
// deliberately not flagged: an unexported field there may be intentional
// rebuilt-on-load scratch, which is ambiguous, so it stays a review concern.
func serializesToEmpty(t reflect.Type) bool {
	hasUnexported := false
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).PkgPath != "" { // unexported field
			hasUnexported = true
			break
		}
	}
	if !hasUnexported {
		return false
	}

	data, err := json.Marshal(reflect.New(t).Elem().Interface())
	return err == nil && string(data) == "{}"
}
