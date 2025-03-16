package training

import "github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/tensor"

// DataSource can provide data for training and testing.
type DataSource interface {
	NextBatch(batchSize int) (data tensor.Tensor, label []int)
	Rewind()
}
