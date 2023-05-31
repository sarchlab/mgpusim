package dispatching

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_kernels_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/kernels GridBuilder
//go:generate mockgen -destination "mock_resource_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/timing/cp/internal/resource CUResourcePool,CUResource
//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/sim Port
//go:generate mockgen -destination "mock_tracing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/tracing NamedHookable
//go:generate mockgen -source alg.go -destination mock_alg.go -package $GOPACKAGE -mock_names=algorithm=MockAlgorithm

func TestDispatching(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dispatching Suite")
}
