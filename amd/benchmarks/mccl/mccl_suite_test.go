package mccl_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLayers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MCCL Suite")
}
