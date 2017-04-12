package cu_test

import . "github.com/onsi/ginkgo"
import "gitlab.com/yaotsu/gcn3/timing/cu"

var _ = Describe("Scheduler", func() {
	var (
		scheduler *cu.Scheduler
	)

	BeforeEach(func() {
		scheduler = cu.NewScheduler("scheduler")
	})

	Context("when processing MapWGReq", func() {
		It("should map wg if resource available", func() {
		})
	})
})
