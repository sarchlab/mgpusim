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

// waveWithCode returns a wavefront whose kernel occupies size bytes from
// address 0x1000 and records that extent.
func waveWithCode(t *testing.T, size uint64) *Wavefront {
	t.Helper()
	group := checkpointGroup(checkpointCU(t))
	group.raw().Packet.Packet.KernelObject = 0x1000
	code := &group.raw().CodeObject
	code.Present, code.HasSymbol, code.HasMeta = true, true, true
	code.Symbol.Size = size

	return group.waves()[0]
}

func TestFetchAheadKeepsSeveralLinesInFlight(t *testing.T) {
	wave := waveWithCode(t, 0x1000)
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

// Lines fetched ahead may never execute, so a wavefront that cannot tell
// where its code ends requests one line at a time.
func TestFetchAheadNeedsTheCodeExtent(t *testing.T) {
	wave := checkpointGroup(checkpointCU(t)).waves()[0]
	state := wave.execution()
	state.State = wavefront.WfReady
	state.InstBufferStartPC = 0x1000
	ahead := FetchArbiter{InstBufByteSize: 256, FetchAheadLines: 3}

	if !ahead.canFetchFromWF(wave) {
		t.Fatal("refused the first line")
	}
	state.NextFetchPC = 0x1040
	if ahead.canFetchFromWF(wave) {
		t.Fatal("fetched ahead without knowing where the code ends")
	}

	state.InstBuffer = make([]byte, 64)
	if !ahead.canFetchFromWF(wave) {
		t.Fatal("refused the next line after the first arrived")
	}
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

// A wavefront that cannot tell where its code ends must not read farther past
// its instructions than it did with the default buffer, however large the
// fetch-ahead buffer is.
func TestUnknownCodeExtentKeepsTheDefaultBuffer(t *testing.T) {
	wave := checkpointGroup(checkpointCU(t)).waves()[0]
	wave.execution().State = wavefront.WfReady
	wave.execution().InstBufferStartPC = 0x1000

	ahead := FetchArbiter{InstBufByteSize: 640, FetchAheadLines: 10}
	if got := fillInstBuffer(ahead, wave); got != DefaultInstBufByteSize {
		t.Fatalf("buffered %d bytes without a code end, want %d", got, DefaultInstBufByteSize)
	}
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

// With a recorded code end, fetch-ahead fills its whole buffer.
func TestFetchAheadFillsItsBuffer(t *testing.T) {
	wave := waveWithCode(t, 0x1000)
	wave.execution().State = wavefront.WfReady
	wave.execution().InstBufferStartPC = 0x1000

	ahead := FetchArbiter{InstBufByteSize: 640, FetchAheadLines: 10}
	if got := fillInstBuffer(ahead, wave); got != 640 {
		t.Fatalf("buffered %d bytes, want 640", got)
	}
}

func TestFetchAheadStopsAtTheEndOfTheCode(t *testing.T) {
	wave := waveWithCode(t, 0x80)
	state := wave.execution()
	state.State = wavefront.WfReady
	state.InstBufferStartPC = 0x1000
	ahead := FetchArbiter{InstBufByteSize: 256, FetchAheadLines: 4}

	state.NextFetchPC = 0x1040
	if !ahead.canFetchFromWF(wave) {
		t.Fatal("refused the last line of the code")
	}
	state.NextFetchPC = 0x1080
	if ahead.canFetchFromWF(wave) {
		t.Fatal("requested a line after the end of the code")
	}
}

func TestFetchAheadStopsAtTheInstructionBufferSize(t *testing.T) {
	wave := waveWithCode(t, 0x1000)
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
