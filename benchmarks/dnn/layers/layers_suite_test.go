package layers_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestLayers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Layers Suite")
}
