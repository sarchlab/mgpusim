package training

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
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
