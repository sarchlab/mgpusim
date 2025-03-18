package gputensor

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTensor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tensor Suite")
}
