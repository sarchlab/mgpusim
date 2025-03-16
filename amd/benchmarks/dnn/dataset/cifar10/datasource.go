package cifar10

import (
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/tensor"
	//"fmt"
)

// A DataSource provides convenient solution to feed data to neural networks.
type DataSource struct {
	to            tensor.Operator
	dataSet       *DataSet
	pixelPerImage int
	currPtr       int
	allData       []float64
	allLabel      []int
}

// NewTrainingDataSource returns a DataSource object that fetches data from
// the training set.
func NewTrainingDataSource(to tensor.Operator) *DataSource {
	ds := &DataSource{
		to:            to,
		pixelPerImage: 1024,
	}

	ds.dataSet = new(DataSet)
	ds.dataSet.initTrain()
	ds.dataSet.openTrainingFile()
	ds.loadAllData()

	return ds
}

// NewTestDataSource creates a DataSource object that fetches data from the
// test set.
func NewTestDataSource(to tensor.Operator) *DataSource {
	ds := &DataSource{
		to:            to,
		pixelPerImage: 1024,
	}

	ds.dataSet = new(DataSet)
	ds.dataSet.initTest()
	ds.dataSet.openTestFile()
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
		if ds.dataSet.isFirstChannel() {
			// only first channel got label for the whole images
			ds.allLabel = append(ds.allLabel, int(l))
		}
	}
}

// NextBatch returns another batch of data.
func (ds *DataSource) NextBatch(batchSize int) (
	data tensor.Tensor,
	label []int,
) {
	channelPerImage := 3
	start := ds.currPtr
	end := start + batchSize

	if end > len(ds.allLabel) {
		end = len(ds.allLabel)
	}

	valuePerImage := ds.pixelPerImage * channelPerImage
	rawData := ds.allData[start*valuePerImage : end*valuePerImage]

	data = ds.to.CreateWithData(rawData, []int{end - start, 3, 32, 32}, "NCHW")

	label = ds.allLabel[start:end]

	ds.currPtr = end

	return data, label
}

// Rewind resets the pointer to the beginning of the dataset.
func (ds *DataSource) Rewind() {
	ds.currPtr = 0
}

// to test with lenet, add test function to change the dimension of a picture, currently 3*32*32, to 1*28*28
