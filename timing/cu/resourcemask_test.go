package cu_test

import (
	. "github.com/onsi/ginkgo"
	"gitlab.com/yaotsu/gcn3/timing/cu"
)

var _ = Describe("Resource Mask", func() {

	var mask *cu.ResourceMask

	BeforeEach(func() {
		mask = cu.NewResourceMask(128)
	})

	It("should get the first region that has the required status", func() {

	})
})
