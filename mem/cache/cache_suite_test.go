package cache

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_cache_test.go" -package $GOPACKAGE  -write_package_comment=false -self_package=github.com/sarchlab/mgpusim/v3/mem/cache github.com/sarchlab/mgpusim/v3/mem/cache VictimFinder,Directory

func TestCache(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cache Suite")
}
