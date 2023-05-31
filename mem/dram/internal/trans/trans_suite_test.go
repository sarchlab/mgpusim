package trans

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -source commandcreator.go -destination mock_commandcreator_test.go -self_package github.com/sarchlab/mgpusim/v3/mem/dram/internal/trans -package $GOPACKAGE
//go:generate mockgen -destination "mock_cmdq_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/dram/internal/cmdq CommandQueue
//go:generate mockgen -destination "mock_addressmapping_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/dram/internal/addressmapping Mapper

func TestTrans(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Trans Suite")
}
