package messaging

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/sim Port,Engine,Buffer
//go:generate mockgen -destination "mock_pipelining_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/pipelining Pipeline

func TestNOC(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "NOC")
}
