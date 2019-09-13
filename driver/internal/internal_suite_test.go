package internal

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_mmu_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mem/vm/mmu MMU
func TestInternal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Internal Suite")
}
