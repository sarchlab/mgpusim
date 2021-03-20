package writearound

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_cache_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mem/v2/cache Directory,MSHR
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mem/v2/mem LowModuleFinder
//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita/v2/sim Port
//go:generate mockgen -destination "mock_util_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/util/v2/buffering Buffer
//go:generate mockgen -destination "mock_pipelinging_test.go" -package $GOPACKAGE -write_package_comment=false "gitlab.com/akita/util/v2/pipelining" Pipeline
func TestWriteAround(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "writearound Suite")
}
