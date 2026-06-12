package modeling

// State is a constraint for component runtime state.
//
// States must be plain structs with primitive fields and simple nested structs.
// No pointers to live objects, no ports, no functions. Cross-references between
// components should use string IDs rather than direct pointers.
//
// Go does not support a struct constraint, so this is typed as `any`.
// Use [ValidateState] at runtime to verify that a value conforms to these rules.
type State = any
