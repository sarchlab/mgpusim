package driver

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_internal_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v5/amd/driver/internal MemoryAllocator
func TestDriver(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "GCN3 GPU Driver")
}
