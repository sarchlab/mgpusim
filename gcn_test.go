package gcn3

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_kernels_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/gcn3/kernels GridBuilder
//go:generate mockgen -destination "mock_akita_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita Engine,Port

func TestEmulator(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCN3")
}
