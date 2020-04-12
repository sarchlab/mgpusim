package bitops_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestBitops(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bitops Suite")
}
