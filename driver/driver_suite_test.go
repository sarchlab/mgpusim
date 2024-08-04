package driver

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_internal_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/driver/internal MemoryAllocator
//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Port,Engine
//go:generate mockgen -destination "mock_vm_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/vm PageTable
func TestDriver(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "GCN3 GPU Driver")
}
