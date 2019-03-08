package driver

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestDriver(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "GCN3 GPU Driver")
}
