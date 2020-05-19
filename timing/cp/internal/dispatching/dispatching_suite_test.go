package dispatching

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_kernels_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mgpusim/kernels GridBuilder
//go:generate mockgen -destination "mock_resource_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mgpusim/timing/cp/internal/resource CUResourcePool,CUResource
//go:generate mockgen -destination "mock_akita_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita Port
//go:generate mockgen -destination "mock_tracing_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/util/tracing NamedHookable
//go:generate mockgen -source alg.go -destination mock_alg.go -package $GOPACKAGE -mock_names=algorithm=MockAlgorithm

func TestDispatching(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dispatching Suite")
}
