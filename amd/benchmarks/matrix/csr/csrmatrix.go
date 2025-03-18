// Package csr provides a csr matrix definition
package csr

//Matrix defines row col and value
type Matrix struct {
	RowOffsets    []uint32
	ColumnNumbers []uint32
	Values        []float32
}
