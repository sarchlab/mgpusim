package cmdq

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_org_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/dram/internal/org Channel

func TestCmdq(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cmdq Suite")
}
