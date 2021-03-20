package emu

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_vm_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mem/v2/vm PageTable

func TestEmulator(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCN3 Emulator")
}
