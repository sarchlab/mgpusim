package cu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/timing"
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
			wfPools = append(wfPools, NewWavefrontPool(newTestComputeUnit("CU", nil), 0, 10))
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
			wf := newTestWave()

			wf.execution().LastFetchTime = wfLastFetchTime[i]
			wf.execution().State = wfState[i]
			testAddWave(wfPools[i%4], wf)

			if i == 4 {
				wf.execution().InstBuffer = make([]byte, arbiter.InstBufByteSize)
			}
		}

		wfs := arbiter.Arbitrate(wfPools)

		Expect(len(wfs)).To(Equal(1))
		Expect(wfs[0].execution().LastFetchTime).To(Equal(timing.VTimeInPicoSec(9500)))
	})
})

func TestFetchStartsAtLinkedKernelEntry(t *testing.T) {
	group := checkpointGroup(checkpointCU(t))
	wave := group.waves()[0]
	group.raw().Packet.Packet.KernelObject = 0x10000
	code := &group.raw().CodeObject
	code.Present, code.HasSymbol, code.HasMeta = true, true, true
	code.Meta.KernelCodeEntryByteOffset = 0x1800
	code.Symbol.Size = 64

	state := wave.execution()
	state.State = wavefront.WfReady
	state.InstBufferStartPC = 0x11800
	arbiter := FetchArbiter{InstBufByteSize: 256}
	if !arbiter.canFetchFromWF(wave) {
		t.Fatal("refused the first instruction at the linked kernel entry")
	}
	state.InstBuffer = make([]byte, 64)
	if arbiter.canFetchFromWF(wave) {
		t.Fatal("allowed a fetch beyond the linked kernel instructions")
	}
}

func TestFetchAheadKeepsSeveralLinesInFlight(t *testing.T) {
	wave := checkpointGroup(checkpointCU(t)).waves()[0]
	state := wave.execution()
	state.State = wavefront.WfReady
	state.InstBufferStartPC = 0x1000

	one := FetchArbiter{InstBufByteSize: 256}
	ahead := FetchArbiter{InstBufByteSize: 256, FetchAheadLines: 3}

	state.NextFetchPC = 0x1040
	if one.canFetchFromWF(wave) {
		t.Fatal("fetched a second line while one was in flight")
	}
	if !ahead.canFetchFromWF(wave) {
		t.Fatal("refused a second line in flight")
	}

	state.NextFetchPC = 0x10c0
	if ahead.canFetchFromWF(wave) {
		t.Fatal("exceeded the number of lines in flight")
	}

	// One line arrived: two are in flight again.
	state.InstBuffer = make([]byte, 64)
	if !ahead.canFetchFromWF(wave) {
		t.Fatal("refused to refill after a line arrived")
	}
}

func TestFetchAheadStopsAtTheInstructionBufferSize(t *testing.T) {
	wave := checkpointGroup(checkpointCU(t)).waves()[0]
	state := wave.execution()
	state.State = wavefront.WfReady
	state.InstBufferStartPC = 0x1000
	arbiter := FetchArbiter{InstBufByteSize: 256, FetchAheadLines: 4}

	// Two lines buffered and two in flight fill the buffer.
	state.InstBuffer = make([]byte, 128)
	state.NextFetchPC = 0x1100
	if arbiter.canFetchFromWF(wave) {
		t.Fatal("requested more lines than the instruction buffer holds")
	}

	state.NextFetchPC = 0x10c0
	if !arbiter.canFetchFromWF(wave) {
		t.Fatal("refused a line the instruction buffer has room for")
	}
}

func TestRedirectFetchStartsANewStream(t *testing.T) {
	wave := checkpointGroup(checkpointCU(t)).waves()[0]
	state := wave.execution()
	state.InstBuffer = make([]byte, 128)
	state.InstBufferStartPC = 0x1000
	state.NextFetchPC = 0x1100
	wave.SetPC(0x2044)

	redirectFetch(wave)

	if len(state.InstBuffer) != 0 || state.InstBufferStartPC != 0x2040 {
		t.Fatalf("buffer holds %d bytes from 0x%x", len(state.InstBuffer), state.InstBufferStartPC)
	}
	if state.FetchEpoch != 1 {
		t.Fatalf("fetch epoch %d, want 1", state.FetchEpoch)
	}
	if next := nextFetchPC(state); next != 0x2040 {
		t.Fatalf("next fetch at 0x%x, want the line of the new PC", next)
	}
}
