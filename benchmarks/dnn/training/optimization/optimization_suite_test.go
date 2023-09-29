package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_tensor_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor Tensor,Operator
//go:generate mockgen -destination "mock_layers_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/benchmarks/dnn/layers Layer

func TestOptimization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Optimization Suite")
}
