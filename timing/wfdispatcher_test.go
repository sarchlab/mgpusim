package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
)

//
var _ = Describe("WfDispatcher", func() {
	var (
		engine       *core.MockEngine
		cu           *ComputeUnit
		wfDispatcher *WfDispatcherImpl
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		cu = NewComputeUnit("cu", engine)
		cu.Freq = 1
		wfDispatcher = NewWfDispatcher(cu)
	})

	It("should dispatch wavefront", func() {
		wf := new(Wavefront)
		req := gcn3.NewDispatchWfReq(nil, cu, 10, nil)
		wfDispatcher.DispatchWf(wf, req)
		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})
})
