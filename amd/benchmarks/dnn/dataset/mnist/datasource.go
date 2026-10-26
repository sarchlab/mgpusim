package mnist

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
)

// A DataSource provides convenient solution to feed data to neural networks.
type DataSource struct {
	*training.ResidentDataSource
	dataSet *DataSet
}

// NewTrainingDataSource returns a DataSource object that fetches data from
// the training set.
func NewTrainingDataSource(to tensor.Operator) *DataSource {
	ds := &DataSource{
		ResidentDataSource: &training.ResidentDataSource{
			Operator: to,
			State: &training.DataState{
				Shape: []int{
					1,
					28,
					28,
				},
				Descriptor: "NCHW",
			},
		},
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
		ResidentDataSource: &training.ResidentDataSource{
			Operator: to,
			State: &training.DataState{
				Shape: []int{
					1,
					28,
					28,
				},
				Descriptor: "NCHW",
			},
		},
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
		ds.State.Data = append(ds.State.Data, normalizedD...)
		ds.State.Labels = append(ds.State.Labels, int(l))
	}
}
