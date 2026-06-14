// Package modeling provides a generic component framework for Akita simulations.
//
// It defines Component[S, T], a generic component parameterized by a Spec type
// (immutable configuration) and a State type (mutable runtime data). Both Spec
// and State must be plain structs with only primitive fields, slices/maps of
// primitives, or simple nested structs. They must be JSON-serializable and must
// not contain pointers, interfaces, or functions.
package modeling

// Spec is a constraint for component specifications.
//
// Specs must be plain structs with only primitive fields (bool, int, int8,
// int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64,
// string) and slices/maps of primitives. No pointers, no interfaces, no
// functions. They must be JSON-serializable.
//
// Go does not support a struct constraint, so this is typed as `any`.
// Use [ValidateSpec] at runtime to verify that a value conforms to these rules.
type Spec = any
