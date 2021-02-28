package writearound

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_cache_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/mem/cache Directory,MSHR,LowModuleFinder
//go:generate mockgen -destination "mock_akita_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita Port
//go:generate mockgen -destination "mock_util_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/util Buffer
//go:generate mockgen -destination "mock_pipelinging_test.go" -package $GOPACKAGE -write_package_comment=false "gitlab.com/akita/util/pipelining" Pipeline
func TestWriteAround(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "writearound Suite")
}
