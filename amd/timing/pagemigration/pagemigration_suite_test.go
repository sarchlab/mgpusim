package pagemigration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPageMigration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Page Migration Suite")
}
