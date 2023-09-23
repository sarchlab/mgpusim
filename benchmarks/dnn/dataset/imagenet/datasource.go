package imagenet

import (
	"gitlab.com/akita/dnn/tensor"
)

// A DataSource provides convenient solution to feed data to neural networks.
type DataSource struct {
	to      tensor.Operator
	dataSet *DataSet
	currPtr int
}

// NewTrainingDataSource returns a DataSource object that fetches data from
// the training set.
func NewTrainingDataSource(to tensor.Operator) *DataSource {
	ds := &DataSource{
		to: to,
	}

	ds.dataSet = NewDataSet(true)

	return ds
}

// NewTestDataSource creates a DataSource object that fetches data from the
// test set.
func NewTestDataSource(to tensor.Operator) *DataSource {
	ds := &DataSource{
		to: to,
	}

	ds.dataSet = NewDataSet(false)

	return ds
}

// NextBatch returns another batch of data.
func (ds *DataSource) NextBatch(batchSize int) (
	data tensor.Tensor,
	label []int,
) {
	var count int
	var rawData []float64
	var curLabel []int

	count = 0

	for j := 0; j < batchSize; j++ {
		if ds.dataSet.HasNext() {
			d, l := ds.dataSet.Next()
			normalizedD := make([]float64, len(d))
			for i := 0; i < len(d); i++ {
				normalizedD[i] = float64(d[i]) / 255
			}
			rawData = append(rawData, normalizedD...)
			curLabel = append(curLabel, int(l))
			count++
		}
	}

	data = ds.to.CreateWithData(rawData, []int{count, 3, 224, 224}, "NCHW")

	ds.currPtr += count

	return data, curLabel
}

// Rewind resets the pointer to the beginning of the dataset.
func (ds *DataSource) Rewind() {
	ds.dataSet.Reset()
}
