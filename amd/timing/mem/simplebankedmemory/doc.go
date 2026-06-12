// Package simplebankedmemory provides a configurable banked memory component.
//
// It is a variant of akita's mem/simplebankedmemory that additionally models
// per-bank row buffers (a configurable extra delay on row misses), accepts a
// separate address conversion for bank selection, and queues incoming
// requests internally so a blocked bank does not head-of-line block requests
// destined for other banks.
package simplebankedmemory
