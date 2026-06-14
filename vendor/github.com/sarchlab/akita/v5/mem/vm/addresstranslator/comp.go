package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the AddressTranslator.
type Spec struct {
	Freq           timing.Freq `json:"freq"`
	Log2PageSize   uint64      `json:"log2_page_size"`
	DeviceID       uint64      `json:"device_id"`
	NumReqPerCycle int         `json:"num_req_per_cycle"`
}

// Resources holds the external wiring referenced by the AddressTranslator. The
// mappers tell the translator where to send memory and translation requests.
type Resources struct {
	MemProviderMapper         mem.AddressToPortMapper `json:"-"`
	TranslationProviderMapper mem.AddressToPortMapper `json:"-"`
}

// incomingReqState is a serializable representation of an incoming request.
type incomingReqState struct {
	ID    uint64               `json:"id"`
	Src   messaging.RemotePort `json:"src"`
	Dst   messaging.RemotePort `json:"dst"`
	RspTo uint64               `json:"rsp_to"`
	Type  string               `json:"type"`

	// Fields preserved for translated request creation.
	Address            uint64 `json:"address"`
	AccessByteSize     uint64 `json:"access_byte_size"`
	PID                vm.PID `json:"pid"`
	Data               []byte `json:"data,omitempty"`
	DirtyMask          []bool `json:"dirty_mask,omitempty"`
	CanWaitForCoalesce bool   `json:"can_wait_for_coalesce"`
}

// transactionState is a serializable representation of a runtime transaction.
type transactionState struct {
	IncomingReqs      []incomingReqState   `json:"incoming_reqs"`
	TranslationReqID  uint64               `json:"translation_req_id"`
	TranslationReqSrc messaging.RemotePort `json:"translation_req_src"`
	TranslationReqDst messaging.RemotePort `json:"translation_req_dst"`
	TranslationDone   bool                 `json:"translation_done"`
	Page              vm.Page              `json:"page"`
}

// reqToBottomState is a serializable representation of a runtime reqToBottom.
type reqToBottomState struct {
	ReqFromTopID    uint64               `json:"req_from_top_id"`
	ReqFromTopSrc   messaging.RemotePort `json:"req_from_top_src"`
	ReqFromTopDst   messaging.RemotePort `json:"req_from_top_dst"`
	ReqFromTopType  string               `json:"req_from_top_type"`
	ReqToBottomID   uint64               `json:"req_to_bottom_id"`
	ReqToBottomSrc  messaging.RemotePort `json:"req_to_bottom_src"`
	ReqToBottomDst  messaging.RemotePort `json:"req_to_bottom_dst"`
	ReqToBottomType string               `json:"req_to_bottom_type"`
}

// State contains mutable runtime data for the AddressTranslator.
type State struct {
	ControlState        memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID        uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc       messaging.RemotePort     `json:"current_cmd_src"`
	Transactions        []transactionState       `json:"transactions"`
	InflightReqToBottom []reqToBottomState       `json:"inflight_req_to_bottom"`
}

// Helper functions

func addrToPageID(addr, log2PageSize uint64) uint64 {
	return (addr >> log2PageSize) << log2PageSize
}

func msgToIncomingReqState(msg messaging.Msg) incomingReqState {
	meta := msg.Meta()
	s := incomingReqState{
		ID:    meta.ID,
		Src:   meta.Src,
		Dst:   meta.Dst,
		RspTo: meta.RspTo,
		Type:  fmt.Sprintf("%T", msg),
	}

	switch req := msg.(type) {
	case memprotocol.ReadReq:
		s.Address = req.Address
		s.AccessByteSize = req.AccessByteSize
		s.PID = req.PID
		s.CanWaitForCoalesce = req.CanWaitForCoalesce
	case memprotocol.WriteReq:
		s.Address = req.Address
		s.PID = req.PID
		s.Data = req.Data
		s.DirtyMask = req.DirtyMask
		s.CanWaitForCoalesce = req.CanWaitForCoalesce
	default:
		log.Panicf("cannot convert message of type %T", msg)
	}

	return s
}

func createTranslatedReq(
	reqState incomingReqState,
	page vm.Page,
	log2PageSize uint64,
	bottomPortRemote messaging.RemotePort,
	memProviderMapper mem.AddressToPortMapper,
) messaging.Msg {
	offset := reqState.Address % (1 << log2PageSize)
	addr := page.PAddr + offset

	switch reqState.Type {
	case "memprotocol.ReadReq":
		clone := memprotocol.ReadReq{}
		clone.ID = timing.GetIDGenerator().Generate()
		clone.Src = bottomPortRemote
		clone.Dst = memProviderMapper.Find(addr)
		clone.Address = addr
		clone.AccessByteSize = reqState.AccessByteSize
		clone.PID = 0
		clone.TrafficBytes = 12
		clone.TrafficClass = "memprotocol.ReadReq"
		clone.CanWaitForCoalesce = reqState.CanWaitForCoalesce
		return clone
	case "memprotocol.WriteReq":
		clone := memprotocol.WriteReq{}
		clone.ID = timing.GetIDGenerator().Generate()
		clone.Src = bottomPortRemote
		clone.Dst = memProviderMapper.Find(addr)
		clone.Data = reqState.Data
		clone.DirtyMask = reqState.DirtyMask
		clone.Address = addr
		clone.PID = 0
		clone.TrafficBytes = len(reqState.Data) + 12
		clone.TrafficClass = "memprotocol.WriteReq"
		clone.CanWaitForCoalesce = reqState.CanWaitForCoalesce
		return clone
	default:
		log.Panicf("cannot translate request of type %s", reqState.Type)
		return nil
	}
}

// restoreMemMsg reconstructs a concrete mem message from saved metadata. It
// preserves the original message ID so tracing lookups (keyed by the message
// ID) resolve to the same task as the original incoming message.
func restoreMemMsg(
	id uint64, src, dst messaging.RemotePort, rspTo uint64, typ string,
) messaging.Msg {
	meta := messaging.MsgMeta{ID: id, Src: src, Dst: dst, RspTo: rspTo}
	switch typ {
	case "memprotocol.WriteReq":
		return memprotocol.WriteReq{MsgMeta: meta}
	default:
		return memprotocol.ReadReq{MsgMeta: meta}
	}
}

func findTransactionByReqID(transactions []transactionState, id uint64) int {
	for i, t := range transactions {
		if t.TranslationReqID == id {
			return i
		}
	}
	return -1
}

func removeTransaction(state *State, idx int) {
	state.Transactions = append(
		state.Transactions[:idx],
		state.Transactions[idx+1:]...)
}

func isReqInBottomByID(inflight []reqToBottomState, id uint64) bool {
	for _, r := range inflight {
		if r.ReqToBottomID == id {
			return true
		}
	}
	return false
}

func findReqToBottomByID(inflight []reqToBottomState, id uint64) reqToBottomState {
	for _, r := range inflight {
		if r.ReqToBottomID == id {
			return r
		}
	}
	panic("req to bottom not found")
}

func removeReqToBottomByID(state *State, id uint64) {
	for i, r := range state.InflightReqToBottom {
		if r.ReqToBottomID == id {
			state.InflightReqToBottom = append(
				state.InflightReqToBottom[:i],
				state.InflightReqToBottom[i+1:]...)
			return
		}
	}
	panic("req to bottom not found")
}

// buildReqToBottom creates a reqToBottomState from the incoming request
// and the translated outgoing request.
func buildReqToBottom(
	reqState incomingReqState, translatedReq messaging.Msg,
) reqToBottomState {
	return reqToBottomState{
		ReqFromTopID:    reqState.ID,
		ReqFromTopSrc:   reqState.Src,
		ReqFromTopDst:   reqState.Dst,
		ReqFromTopType:  reqState.Type,
		ReqToBottomID:   translatedReq.Meta().ID,
		ReqToBottomSrc:  translatedReq.Meta().Src,
		ReqToBottomDst:  translatedReq.Meta().Dst,
		ReqToBottomType: fmt.Sprintf("%T", translatedReq),
	}
}

// Comp is the AddressTranslator component.
type Comp = modeling.Component[Spec, State, Resources]
