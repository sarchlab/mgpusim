package dispatching

import (
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_kernels_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v5/amd/kernels GridBuilder
//go:generate mockgen -destination "mock_resource_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource CUResourcePool,CUResource
//go:generate mockgen -destination "mock_messaging_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim/messaging Port
//go:generate mockgen -source alg.go -destination mock_alg.go -package $GOPACKAGE -mock_names=algorithm=MockAlgorithm

func TestDispatching(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dispatching Suite")
}

var testID atomic.Uint64

func testNewID() uint64 { return testID.Add(1) }
