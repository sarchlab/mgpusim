package driver

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_internal_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mgpusim/v2/driver/internal MemoryAllocator
//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita/v2/sim Port,Engine
//go:generate mockgen -destination "mock_vm_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mem/v2/vm PageTable
func TestDriver(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "GCN3 GPU Driver")
}
