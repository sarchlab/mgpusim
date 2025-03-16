package cp

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Engine,Port
//go:generate mockgen -destination "mock_kernels_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v4/amd/kernels GridBuilder
//go:generate mockgen -destination "mock_dispatching_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v4/amd/timing/cp/internal/dispatching Dispatcher

func TestCp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CP Suite")
}
