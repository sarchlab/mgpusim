package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
)

var _ = Describe("FetchArbiter", func() {
	var (
		wfPools []*WavefrontPool
		arbiter *FetchArbiter
	)

	BeforeEach(func() {
		wfPools = make([]*WavefrontPool, 0, 4)
		for i := 0; i < 4; i++ {
			wfPools = append(wfPools, NewWavefrontPool(10))
		}
		arbiter = new(FetchArbiter)
		arbiter.InstBufByteSize = 256
	})

	It("should find the oldest wf to dispatch", func() {
		wfLastFetchTime := []core.VTimeInSec{
			10.2, 10.3, 9.8, 9.7, 9.4,
			9.6, 9.6, 9.5, 9.8, 10.0,
		}
		wfState := []WfState{
			WfRunning, WfRunning, WfReady, WfReady, WfRunning,
			WfRunning, WfRunning, WfRunning, WfReady, WfRunning,
		}

		for i := 0; i < len(wfState); i++ {
			wf := new(Wavefront)
			wf.LastFetchTime = wfLastFetchTime[i]
			wf.State = wfState[i]
			wfPools[i%4].AddWf(wf)

			if i == 4 {
				wf.InstBuffer = make([]byte, arbiter.InstBufByteSize)
			}
		}

		wfs := arbiter.Arbitrate(wfPools)

		Expect(len(wfs)).To(Equal(1))
		Expect(wfs[0].LastFetchTime).To(Equal(core.VTimeInSec(9.5)))
	})
})
