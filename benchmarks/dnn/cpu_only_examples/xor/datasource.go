package main

import (
	"gitlab.com/akita/dnn/tensor"
)

// DataSource generate XOR data.
type DataSource struct {
	to        tensor.Operator
	allData   []float64
	allLabel  []int
	imageSize int
	currPtr   int
}

// NewDataSource creates a new XOR DataSource
func NewDataSource(to tensor.Operator) *DataSource {
	ds := &DataSource{
		to:        to,
		imageSize: 2,
	}
	ds.allData = []float64{
		0, 0,
		0, 1,
		1, 0,
		1, 1,
	}
	ds.allLabel = []int{
		0, 1, 1, 0,
	}
	return ds
}

// NextBatch returns the next batch of data.
func (ds *DataSource) NextBatch(batchSize int) (
	data tensor.Tensor,
	label []int,
) {
	start := ds.currPtr
	end := start + batchSize

	if end > len(ds.allLabel) {
		end = len(ds.allLabel)
	}

	rawData := ds.allData[start*ds.imageSize : end*ds.imageSize]
	data = ds.to.CreateWithData(rawData, []int{end - start, ds.imageSize}, "")

	label = ds.allLabel[start:end]

	ds.currPtr = end

	return data, label
}

// Rewind sets the pointer back to the beginning point.
func (ds *DataSource) Rewind() {
	ds.currPtr = 0
}
