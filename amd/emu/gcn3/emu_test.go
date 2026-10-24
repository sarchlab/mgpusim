package gcn3

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEmulator(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCN3 Emulator")
}
