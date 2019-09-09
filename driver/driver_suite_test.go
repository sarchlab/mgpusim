package driver

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_mmu_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mem/vm/mmu MMU
//go:generate mockgen -destination "mock_akita_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita Port,Engine

func TestDriver(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "GCN3 GPU Driver")
}
