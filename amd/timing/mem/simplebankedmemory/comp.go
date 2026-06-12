package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the simple banked memory.
//
// Address conversion is configured twice: the Addr* fields configure the
// conversion applied before storage reads/writes, and the BankAddr* fields
// configure a separate conversion used ONLY for bank selection and row-buffer
// tracking. When BankAddrConvKind is empty, bank selection falls back to the
// Addr* conversion (which is the identity when AddrConvKind is empty).
type Spec struct {
	Freq                           timing.Freq `json:"freq"`
	NumBanks                       int         `json:"num_banks"`
	BankPipelineWidth              int         `json:"bank_pipeline_width"`
	BankPipelineDepth              int         `json:"bank_pipeline_depth"`
	StageLatency                   int         `json:"stage_latency"`
	PostPipelineBufSize            int         `json:"post_pipeline_buf_size"`
	Capacity                       uint64      `json:"capacity"`
	BankSelectorKind               string      `json:"bank_selector_kind"`
	BankSelectorLog2InterleaveSize uint64      `json:"bank_selector_log2_interleave_size"`

	// RowBufferSizeLog2 is the log2 of the row buffer size in bytes, and
	// RowMissDelay is the extra delay in cycles applied on a row-buffer miss.
	// Row-buffer tracking is enabled only when both are greater than zero.
	RowBufferSizeLog2 uint64 `json:"row_buffer_size_log2"`
	RowMissDelay      int    `json:"row_miss_delay"`

	AddrConvKind            string `json:"addr_conv_kind"`
	AddrInterleavingSize    uint64 `json:"addr_interleaving_size"`
	AddrTotalNumOfElements  int    `json:"addr_total_num_of_elements"`
	AddrCurrentElementIndex int    `json:"addr_current_element_index"`
	AddrOffset              uint64 `json:"addr_offset"`

	BankAddrConvKind            string `json:"bank_addr_conv_kind"`
	BankAddrInterleavingSize    uint64 `json:"bank_addr_interleaving_size"`
	BankAddrTotalNumOfElements  int    `json:"bank_addr_total_num_of_elements"`
	BankAddrCurrentElementIndex int    `json:"bank_addr_current_element_index"`
	BankAddrOffset              uint64 `json:"bank_addr_offset"`

	StorageRef string `json:"storage_ref"`
}

// bankPipelineItemState is a serializable representation of a pipeline item.
type bankPipelineItemState struct {
	IsRead    bool                 `json:"is_read"`
	ReadMsg   memprotocol.ReadReq  `json:"read_msg"`
	WriteMsg  memprotocol.WriteReq `json:"write_msg"`
	Committed bool                 `json:"committed"`
	ReadData  []byte               `json:"read_data"`
}

// delayedItemState is an item waiting out a row-buffer-miss penalty before
// entering its bank's pipeline.
type delayedItemState struct {
	Item       bankPipelineItemState `json:"item"`
	CyclesLeft int                   `json:"cycles_left"`
}

// bankState captures one bank pipeline + buffer contents, plus the bank's
// row-buffer tracking state and row-miss delay queue.
type bankState struct {
	Pipeline        queueing.Pipeline[bankPipelineItemState] `json:"pipeline"`
	PostPipelineBuf queueing.Buffer[bankPipelineItemState]   `json:"post_pipeline_buf"`
	LastRowAddr     uint64                                   `json:"last_row_addr"`
	RowValid        bool                                     `json:"row_valid"`
	DelayQueue      []delayedItemState                       `json:"delay_queue"`
}

// State contains mutable runtime data for the simple banked memory.
// PendingReqs holds requests retrieved from the Top port that have not been
// dispatched to a bank yet; keeping them in a separate queue lets dispatch
// skip blocked banks instead of head-of-line blocking on the port buffer.
type State struct {
	ControlState  memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID  uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc messaging.RemotePort     `json:"current_cmd_src"`
	Banks         []bankState              `json:"banks"`
	PendingReqs   []bankPipelineItemState  `json:"pending_reqs"`
}

// Resources holds the shared resources referenced by the memory.
type Resources struct {
	Storage *mem.Storage
}

// Comp models a banked memory with configurable banking, row-buffer, and
// pipeline behavior. It is a modeling.Component specialized to this package's
// Spec, State, and Resources.
type Comp = modeling.Component[Spec, State, Resources]

// --- Free functions for pipeline / buffer / bank-selection / address conversion ---

func pipelineCanAccept(bank bankState, spec Spec) bool {
	if spec.BankPipelineDepth == 0 {
		return bank.PostPipelineBuf.CanPush()
	}

	return bank.Pipeline.CanAccept()
}

func pipelineAccept(
	bank *bankState,
	spec Spec,
	item bankPipelineItemState,
) {
	if spec.BankPipelineDepth == 0 {
		bank.PostPipelineBuf.PushTyped(item)
		return
	}

	bank.Pipeline.Accept(item)
}

func pipelineTick(bank *bankState) bool {
	return bank.Pipeline.Tick(&bank.PostPipelineBuf)
}

func bufferPeek(bank bankState) (bankPipelineItemState, bool) {
	if bank.PostPipelineBuf.Size() == 0 {
		return bankPipelineItemState{}, false
	}

	return bank.PostPipelineBuf.Peek(), true
}

func bufferPop(bank *bankState) {
	if bank.PostPipelineBuf.Size() == 0 {
		return
	}

	bank.PostPipelineBuf.Pop()
}

func selectBank(spec Spec, addr uint64) int {
	interleaveSize := uint64(1) << spec.BankSelectorLog2InterleaveSize
	if interleaveSize == 0 {
		panic("simplebankedmemory: invalid interleave size")
	}

	return int((addr / interleaveSize) % uint64(spec.NumBanks))
}

// storageAddress converts an external address to the internal address used
// for storage reads and writes.
func storageAddress(spec Spec, addr uint64) uint64 {
	return mem.ConvertAddress(
		spec.AddrConvKind, spec.AddrOffset,
		spec.AddrInterleavingSize, spec.AddrTotalNumOfElements,
		spec.AddrCurrentElementIndex, addr,
	)
}

// bankSelectionAddress converts an external address to the address used for
// bank selection and row-buffer tracking. The BankAddr* conversion takes
// precedence; otherwise, the storage conversion is used.
func bankSelectionAddress(spec Spec, addr uint64) uint64 {
	if spec.BankAddrConvKind != "" {
		return mem.ConvertAddress(
			spec.BankAddrConvKind, spec.BankAddrOffset,
			spec.BankAddrInterleavingSize, spec.BankAddrTotalNumOfElements,
			spec.BankAddrCurrentElementIndex, addr,
		)
	}

	return storageAddress(spec, addr)
}

// rowAddress computes the row-buffer row that a bank-local address falls in.
func rowAddress(spec Spec, addr uint64) uint64 {
	interleaveSize := uint64(1) << spec.BankSelectorLog2InterleaveSize
	bankBlockIndex := addr / interleaveSize
	bankLocalBlock := bankBlockIndex / uint64(spec.NumBanks)
	offset := addr & (interleaveSize - 1)
	bankLocalAddr := bankLocalBlock*interleaveSize + offset

	return bankLocalAddr >> spec.RowBufferSizeLog2
}

func rowBufferEnabled(spec Spec) bool {
	return spec.RowBufferSizeLog2 > 0 && spec.RowMissDelay > 0
}

func msgToItem(msg messaging.Msg) bankPipelineItemState {
	switch r := msg.(type) {
	case memprotocol.ReadReq:
		return bankPipelineItemState{
			IsRead:  true,
			ReadMsg: r,
		}
	case memprotocol.WriteReq:
		return bankPipelineItemState{
			IsRead:   false,
			WriteMsg: r,
		}
	default:
		log.Panicf("simplebankedmemory: unsupported request type %T", msg)
		return bankPipelineItemState{}
	}
}

// itemAddress returns the external address the item accesses.
func itemAddress(item bankPipelineItemState) uint64 {
	if item.IsRead {
		return item.ReadMsg.Address
	}

	return item.WriteMsg.Address
}
