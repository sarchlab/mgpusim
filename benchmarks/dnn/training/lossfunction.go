package training

import (
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor"
)

// LossFunction defines how loss is calculated.
type LossFunction interface {
	Loss(
		output tensor.Tensor,
		label []int,
	) (
		loss float64,
		derivative tensor.Tensor,
	)
}
