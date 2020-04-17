// Package csr provides a csr matrix definition
package csr

type Matrix struct {
	RowOffsets    []uint32
	ColumnNumbers []uint32
	Values        []float32
}
