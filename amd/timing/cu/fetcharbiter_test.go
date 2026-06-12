package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
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
		wfLastFetchTime := []timing.VTimeInPicoSec{
			10200, 10300, 9800, 9700, 9400,
			9600, 9500, 9600, 9800, 10000,
		}
		wfState := []wavefront.WfState{
			wavefront.WfRunning, wavefront.WfRunning, wavefront.WfReady,
			wavefront.WfReady, wavefront.WfRunning,
			wavefront.WfRunning, wavefront.WfRunning, wavefront.WfRunning,
			wavefront.WfReady, wavefront.WfRunning,
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
		Expect(wfs[0].LastFetchTime).To(Equal(timing.VTimeInPicoSec(9500)))
	})
})
