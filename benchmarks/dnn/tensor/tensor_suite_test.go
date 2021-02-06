package tensor

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTensor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tensor Suite")
}
