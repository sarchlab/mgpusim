package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v3/kernels"
	"github.com/sarchlab/mgpusim/v3/timing/wavefront"
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
		wfLastFetchTime := []sim.VTimeInSec{
			10.2, 10.3, 9.8, 9.7, 9.4,
			9.6, 9.5, 9.6, 9.8, 10.0,
		}
		wfState := []wavefront.WfState{
			wavefront.WfRunning, wavefront.WfRunning, wavefront.WfReady, wavefront.WfReady, wavefront.WfRunning,
			wavefront.WfRunning, wavefront.WfRunning, wavefront.WfRunning, wavefront.WfReady, wavefront.WfRunning,
		}

		for i := 0; i < len(wfState); i++ {
			wf := new(wavefront.Wavefront)
			wf.Wavefront = new(kernels.Wavefront)
			wf.LastFetchTime = wfLastFetchTime[i]
			wf.State = wfState[i]
			wfPools[i%4].AddWf(wf)

			if i == 4 {
				wf.InstBuffer = make([]byte, arbiter.InstBufByteSize)
			}
		}

		wfs := arbiter.Arbitrate(wfPools)

		Expect(len(wfs)).To(Equal(1))
		Expect(wfs[0].LastFetchTime).To(Equal(sim.VTimeInSec(9.5)))
	})
})
