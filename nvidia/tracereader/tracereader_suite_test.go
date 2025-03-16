package tracereader_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTracereader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tracereader Suite")
}
