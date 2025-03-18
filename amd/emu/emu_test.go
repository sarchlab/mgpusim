package emu

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_vm_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/vm PageTable

func TestEmulator(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCN3 Emulator")
}
