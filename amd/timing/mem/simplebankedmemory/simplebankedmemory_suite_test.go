package simplebankedmemory

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSimplebankedmemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Simple Banked Memory Suite")
}
