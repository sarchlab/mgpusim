package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_tensor_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/tensor Tensor,Operator
//go:generate mockgen -destination "mock_layers_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/layers Layer

func TestOptimization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Optimization Suite")
}
