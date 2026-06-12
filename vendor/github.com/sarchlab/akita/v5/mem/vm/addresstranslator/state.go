package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// incomingReqState is a serializable representation of an incoming request.
type incomingReqState struct {
	ID         uint64         `json:"id"`
	Src        sim.RemotePort `json:"src"`
	Dst        sim.RemotePort `json:"dst"`
	RspTo      uint64         `json:"rsp_to"`
	SendTaskID uint64         `json:"send_task_id"`
	RecvTaskID uint64         `json:"recv_task_id"`
	Type       string         `json:"type"`

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
	IncomingReqs              []incomingReqState `json:"incoming_reqs"`
	TranslationReqID          uint64             `json:"translation_req_id"`
	TranslationReqSendTaskID  uint64             `json:"translation_req_send_task_id"`
	TranslationReqSrc         sim.RemotePort     `json:"translation_req_src"`
	TranslationReqDst         sim.RemotePort     `json:"translation_req_dst"`
	TranslationDone           bool               `json:"translation_done"`
	Page                      vm.Page            `json:"page"`
}

// reqToBottomState is a serializable representation of a runtime reqToBottom.
type reqToBottomState struct {
	ReqFromTopID          uint64         `json:"req_from_top_id"`
	ReqFromTopSrc         sim.RemotePort `json:"req_from_top_src"`
	ReqFromTopDst         sim.RemotePort `json:"req_from_top_dst"`
	ReqFromTopSendTaskID  uint64         `json:"req_from_top_send_task_id"`
	ReqFromTopRecvTaskID  uint64         `json:"req_from_top_recv_task_id"`
	ReqFromTopType        string         `json:"req_from_top_type"`
	ReqToBottomID         uint64         `json:"req_to_bottom_id"`
	ReqToBottomSendTaskID uint64         `json:"req_to_bottom_send_task_id"`
	ReqToBottomSrc        sim.RemotePort `json:"req_to_bottom_src"`
	ReqToBottomDst        sim.RemotePort `json:"req_to_bottom_dst"`
	ReqToBottomType       string         `json:"req_to_bottom_type"`
}

// State contains mutable runtime data for the AddressTranslator.
type State struct {
	IsFlushing          bool               `json:"is_flushing"`
	Transactions        []transactionState `json:"transactions"`
	InflightReqToBottom []reqToBottomState `json:"inflight_req_to_bottom"`
}

// Helper functions

func addrToPageID(addr, log2PageSize uint64) uint64 {
	return (addr >> log2PageSize) << log2PageSize
}

func msgToIncomingReqState(msg sim.Msg) incomingReqState {
	meta := msg.Meta()
	s := incomingReqState{
		ID:         meta.ID,
		Src:        meta.Src,
		Dst:        meta.Dst,
		RspTo:      meta.RspTo,
		SendTaskID: meta.SendTaskID,
		RecvTaskID: meta.RecvTaskID,
		Type:       fmt.Sprintf("%T", msg),
	}

	switch req := msg.(type) {
	case *mem.ReadReq:
		s.Address = req.Address
		s.AccessByteSize = req.AccessByteSize
		s.PID = req.PID
		s.CanWaitForCoalesce = req.CanWaitForCoalesce
	case *mem.WriteReq:
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
	bottomPortRemote sim.RemotePort,
	spec Spec,
) sim.Msg {
	offset := reqState.Address % (1 << log2PageSize)
	addr := page.PAddr + offset

	switch reqState.Type {
	case "*mem.ReadReq":
		clone := &mem.ReadReq{}
		clone.ID = sim.GetIDGenerator().Generate()
		clone.Src = bottomPortRemote
		clone.Dst = findMemoryPort(spec, addr)
		clone.Address = addr
		clone.AccessByteSize = reqState.AccessByteSize
		clone.PID = 0
		clone.TrafficBytes = 12
		clone.TrafficClass = "mem.ReadReq"
		clone.CanWaitForCoalesce = reqState.CanWaitForCoalesce
		return clone
	case "*mem.WriteReq":
		clone := &mem.WriteReq{}
		clone.ID = sim.GetIDGenerator().Generate()
		clone.Src = bottomPortRemote
		clone.Dst = findMemoryPort(spec, addr)
		clone.Data = reqState.Data
		clone.DirtyMask = reqState.DirtyMask
		clone.Address = addr
		clone.PID = 0
		clone.TrafficBytes = len(reqState.Data) + 12
		clone.TrafficClass = "mem.WriteReq"
		clone.CanWaitForCoalesce = reqState.CanWaitForCoalesce
		return clone
	default:
		log.Panicf("cannot translate request of type %s", reqState.Type)
		return nil
	}
}

// restoreMemMsg reconstructs a concrete mem message from saved metadata.
func restoreMemMsg(
	id uint64, src, dst sim.RemotePort, rspTo uint64,
	sendTaskID, recvTaskID uint64, typ string,
) sim.Msg {
	switch typ {
	case "*mem.WriteReq":
		m := &mem.WriteReq{}
		m.ID = id
		m.Src = src
		m.Dst = dst
		m.RspTo = rspTo
		m.SendTaskID = sendTaskID
		m.RecvTaskID = recvTaskID
		return m
	default:
		m := &mem.ReadReq{}
		m.ID = id
		m.Src = src
		m.Dst = dst
		m.RspTo = rspTo
		m.SendTaskID = sendTaskID
		m.RecvTaskID = recvTaskID
		return m
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
	reqState incomingReqState, translatedReq sim.Msg,
) reqToBottomState {
	return reqToBottomState{
		ReqFromTopID:          reqState.ID,
		ReqFromTopSrc:         reqState.Src,
		ReqFromTopDst:         reqState.Dst,
		ReqFromTopSendTaskID:  reqState.SendTaskID,
		ReqFromTopRecvTaskID:  reqState.RecvTaskID,
		ReqFromTopType:        reqState.Type,
		ReqToBottomID:         translatedReq.Meta().ID,
		ReqToBottomSendTaskID: translatedReq.Meta().SendTaskID,
		ReqToBottomSrc:        translatedReq.Meta().Src,
		ReqToBottomDst:        translatedReq.Meta().Dst,
		ReqToBottomType:       fmt.Sprintf("%T", translatedReq),
	}
}
