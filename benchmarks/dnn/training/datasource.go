package training

import "gitlab.com/akita/dnn/tensor"

// DataSource can provide data for training and testing.
type DataSource interface {
	NextBatch(batchSize int) (data tensor.Tensor, label []int)
	Rewind()
}
