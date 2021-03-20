package cp

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita/v2/sim Engine,Port
//go:generate mockgen -destination "mock_akitaext_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/util/v2/akitaext BufferedSender
//go:generate mockgen -destination "mock_kernels_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mgpusim/v2/kernels GridBuilder
//go:generate mockgen -destination "mock_dispatching_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mgpusim/v2/timing/cp/internal/dispatching Dispatcher

func TestCp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CP Suite")
}
