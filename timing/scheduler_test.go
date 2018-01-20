package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
)

type mockWfArbitor struct {
	wfsToReturn [][]*Wavefront
}

func newMockWfArbitor() *mockWfArbitor {
	a := new(mockWfArbitor)
	a.wfsToReturn = make([][]*Wavefront, 0)
	return a
}

func (m *mockWfArbitor) Arbitrate([]*WavefrontPool) []*Wavefront {
	if len(m.wfsToReturn) == 0 {
		return nil
	}
	wfs := m.wfsToReturn[0]
	m.wfsToReturn = m.wfsToReturn[1:]
	return wfs
}

var _ = Describe("Scheduler", func() {
	var (
		toInstMemConn *core.MockConnection
		cu            *ComputeUnit
		scheduler     *Scheduler
		fetchArbitor  *mockWfArbitor
		issueArbitor  *mockWfArbitor
		instMem       *core.MockComponent
	)

	BeforeEach(func() {
		toInstMemConn = core.NewMockConnection()
		cu = NewComputeUnit("cu", nil)
		cu.InstMem = instMem
		core.PlugIn(cu, "ToInstMem", toInstMemConn)

		fetchArbitor = newMockWfArbitor()
		issueArbitor = newMockWfArbitor()
		scheduler = NewScheduler(cu, fetchArbitor, issueArbitor)
	})

	It("should fetch", func() {
		wf := new(Wavefront)
		wf.PC = 8064
		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*Wavefront{wf})

		reqToExpect := mem.NewAccessReq()
		reqToExpect.SetSrc(cu)
		reqToExpect.SetDst(instMem)
		reqToExpect.Address = 8064
		reqToExpect.ByteSize = 8
		reqToExpect.Type = mem.Read
		reqToExpect.SetSendTime(10)
		reqToExpect.Info = wf
		toInstMemConn.ExpectSend(reqToExpect, nil)

		scheduler.DoFetch(10)

		Expect(toInstMemConn.AllExpectedSent()).To(BeTrue())
		Expect(wf.State).To(Equal(WfFetching))
	})

})
