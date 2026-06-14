// Package simplebankedmemory provides a configurable banked memory component.
//
// Storage is global: a request's address indexes the backing store directly.
// When several instances are used as interleaved controllers over one shared
// storage, set the bank-selection address conversion (the Spec.BankAddrConv*
// fields) so bank selection operates on a controller-local address rather than
// the strided global address; otherwise accesses alias onto a fraction of the
// banks. The conversion affects bank selection only, never storage.
package simplebankedmemory
