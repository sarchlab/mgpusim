package xor

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
)

type DataSource struct{ *training.ResidentDataSource }

func NewDataSource(to tensor.Operator) *DataSource {
	return &DataSource{
		&training.ResidentDataSource{
			Operator: to,
			State: &training.DataState{
				Data: []float64{
					0,
					0,
					0,
					1,
					1,
					0,
					1,
					1,
				},
				Labels: []int{
					0,
					1,
					1,
					0,
				},
				Shape: []int{
					2,
				},
			},
		},
	}
}
