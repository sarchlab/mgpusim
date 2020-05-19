package cp

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_akita_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita Engine,Port
//go:generate mockgen -destination "mock_akitaext_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/util/akitaext BufferedSender
//go:generate mockgen -destination "mock_kernels_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mgpusim/kernels GridBuilder
//go:generate mockgen -destination "mock_dispatching_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mgpusim/timing/cp/internal/dispatching Dispatcher

func TestCp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cp Suite")
}
