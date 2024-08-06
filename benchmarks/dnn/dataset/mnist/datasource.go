package mnist

import (
	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"
)

// A DataSource provides convenient solution to feed data to neural networks.
type DataSource struct {
	to        tensor.Operator
	dataSet   *DataSet
	imageSize int
	currPtr   int
	allData   []float64
	allLabel  []int
}

// NewTrainingDataSource returns a DataSource object that fetches data from
// the training set.
func NewTrainingDataSource(to tensor.Operator) *DataSource {
	ds := &DataSource{
		to:        to,
		imageSize: 784,
	}

	ds.dataSet = new(DataSet)
	ds.dataSet.OpenTrainingFile()

	ds.loadAllData()

	return ds
}

// NewTestDataSource creates a DataSource object that fetches data from the
// test set.
func NewTestDataSource(to tensor.Operator) *DataSource {
	ds := &DataSource{
		to:        to,
		imageSize: 784,
	}

	ds.dataSet = new(DataSet)
	ds.dataSet.OpenTestFile()

	ds.loadAllData()

	return ds
}

func (ds *DataSource) loadAllData() {
	for ds.dataSet.HasNext() {
		d, l := ds.dataSet.Next()
		normalizedD := make([]float64, len(d))
		for i := 0; i < len(d); i++ {
			normalizedD[i] = float64(d[i]) / 255
		}
		ds.allData = append(ds.allData, normalizedD...)
		ds.allLabel = append(ds.allLabel, int(l))
	}
}

// NextBatch returns another batch of data.
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

	data = ds.to.CreateWithData(rawData, []int{end - start, 1, 28, 28}, "NCHW")

	label = ds.allLabel[start:end]

	ds.currPtr = end

	return data, label
}

// Rewind resets the pointer to the beginning of the dataset.
func (ds *DataSource) Rewind() {
	ds.currPtr = 0
}
