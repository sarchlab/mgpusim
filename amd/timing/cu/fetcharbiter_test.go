package cu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
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

// recordCodeEnd makes the wavefront's kernel a code object whose symbol covers
// exactly size bytes of instructions from address base.
func recordCodeEnd(wave *Wavefront, base, size uint64) {
	raw := wave.group().raw()
	raw.Packet.Present = true
	raw.Packet.Packet.KernelObject = base
	raw.CodeObject.Present, raw.CodeObject.HasSymbol, raw.CodeObject.HasMeta = true, true, true
	raw.CodeObject.Version = insts.CodeObjectV5
	raw.CodeObject.Symbol.Size = size
}

// waveWithCode returns a wavefront whose kernel occupies size bytes from
// address 0x1000 and records that extent.
func waveWithCode(t *testing.T, size uint64) *Wavefront {
	t.Helper()
	wave := checkpointGroup(checkpointCU(t)).waves()[0]
	recordCodeEnd(wave, 0x1000, size)

	return wave
}

// fillInstBuffer returns each admitted request at once, as when the
// wavefront's PC does not move, and reports the bytes buffered when the
// arbiter stops admitting.
func fillInstBuffer(arbiter FetchArbiter, wave *Wavefront) int {
	state := wave.execution()
	for arbiter.canFetchFromWF(wave) && len(state.InstBuffer) < 1<<20 {
		state.NextFetchPC = nextFetchPC(state) + InstFetchByteSize
		state.InstBuffer = append(state.InstBuffer, make([]byte, InstFetchByteSize)...)
	}

	return len(state.InstBuffer)
}

// Fetching one line at a time fills the buffer the spec configures.
func TestOneLineFetchFillsTheConfiguredBuffer(t *testing.T) {
	wave := checkpointGroup(checkpointCU(t)).waves()[0]
	wave.execution().State = wavefront.WfReady
	wave.execution().InstBufferStartPC = 0x1000

	one := FetchArbiter{InstBufByteSize: 640}
	if got := fillInstBuffer(one, wave); got != 640 {
		t.Fatalf("buffered %d bytes, want the configured 640", got)
	}
}

// Fetching whole lines, after a taken branch a wavefront waits for the
// outstanding fetch, and uses its line if it continues the buffer.
func TestLineFetchRedirectWaitsForTheOutstandingFetch(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	arbiter := FetchArbiter{InstBufByteSize: 640}

	wave := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wave)
	state := wave.execution()
	state.State = wavefront.WfReady
	state.InstBufferStartPC = 0x1000
	state.InstBuffer = make([]byte, 64)

	// The fetch of the next line is outstanding when a branch jumps to it.
	req := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{Address: 0x1040,
		AccessByteSize: 64}}
	cu.comp.State.InstFetch = append(cu.comp.State.InstFetch, testFetchState(cu, &InstFetchReqInfo{
		Wavefront: wave, Req: req, Address: 0x1040, Epoch: state.FetchEpoch, FetchTaskID: testIDs.NewID()}))
	state.IsFetching = true
	state.NextFetchPC = 0x1080
	wave.SetPC(0x1040)
	cu.redirectFetch(wave)

	if arbiter.canFetchFromWF(wave) {
		t.Fatal("requested the target while the obsolete fetch was outstanding")
	}

	cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(),
		RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})
	if len(state.InstBuffer) != 64 || state.InstBufferStartPC != 0x1040 {
		t.Fatalf("the returned line was not used: %d bytes from 0x%x",
			len(state.InstBuffer), state.InstBufferStartPC)
	}
	if state.IsFetching {
		t.Fatal("still fetching after the only fetch returned")
	}
	if !arbiter.canFetchFromWF(wave) || nextFetchPC(state) != 0x1080 {
		t.Fatalf("next fetch at 0x%x, want 0x1080", nextFetchPC(state))
	}
}

func TestRedirectFetchStartsANewStream(t *testing.T) {
	wave := checkpointGroup(checkpointCU(t)).waves()[0]
	state := wave.execution()
	state.InstBuffer = make([]byte, 128)
	state.InstBufferStartPC = 0x1000
	state.NextFetchPC = 0x1100
	wave.SetPC(0x2044)

	wave.cu.redirectFetch(wave)

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
