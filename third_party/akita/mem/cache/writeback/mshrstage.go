package writeback

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type mshrStage struct {
	cache *pipelineMW
}

func (s *mshrStage) Tick() bool {
	next := &s.cache.comp.State

	if next.HasProcessingMSHREntry {
		return s.processOneReq()
	}

	mshrBuf := &next.MSHRStageBuf

	if mshrBuf.Size() == 0 {
		return false
	}

	transIdx := mshrBuf.Pop()
	trans := &next.Transactions[transIdx]
	next.HasProcessingMSHREntry = true
	next.ProcessingMSHREntryIdx = transIdx

	// Copy data into local state fields for processing
	_ = trans

	return s.processOneReq()
}

func (s *mshrStage) Reset() {
	next := &s.cache.comp.State
	next.HasProcessingMSHREntry = false
	next.ProcessingMSHREntryIdx = 0
	next.MSHRStageBuf.Clear()
}

func (s *mshrStage) processOneReq() bool {
	if !s.cache.topPort().CanSend() {
		return false
	}

	next := &s.cache.comp.State
	processingTrans := &next.Transactions[next.ProcessingMSHREntryIdx]

	if len(processingTrans.MSHRTransactionIndices) == 0 {
		next.HasProcessingMSHREntry = false
		return true
	}

	transIdx := processingTrans.MSHRTransactionIndices[0]

	// Verify the transaction is still in the transaction list and not removed
	var trans *transactionState
	if transIdx >= 0 && transIdx < len(next.Transactions) {
		t := &next.Transactions[transIdx]
		if !t.Removed {
			trans = t
		}
	}

	transactionPresent := trans != nil && s.findTransaction(transIdx)

	spec := s.cache.comp.Spec()

	if transactionPresent {
		next.Transactions[transIdx].Removed = true

		if trans.HasRead {
			s.respondRead(trans, processingTrans.MSHRData, spec.Log2BlockSize)
		} else {
			s.respondWrite(trans)
		}

		processingTrans.MSHRTransactionIndices = processingTrans.MSHRTransactionIndices[1:]
		if len(processingTrans.MSHRTransactionIndices) == 0 {
			next.HasProcessingMSHREntry = false
		}

		return true
	}

	processingTrans.MSHRTransactionIndices = processingTrans.MSHRTransactionIndices[1:]
	if len(processingTrans.MSHRTransactionIndices) == 0 {
		next.HasProcessingMSHREntry = false
	}

	return true
}

func (s *mshrStage) respondRead(
	trans *transactionState,
	data []byte,
	log2BlockSize uint64,
) {
	_, offset := getCacheLineID(trans.ReadAddress, log2BlockSize)
	respondData := data[offset : offset+trans.ReadAccessByteSize]
	dataReady := memprotocol.DataReadyRsp{}
	dataReady.ID = timing.GetIDGenerator().Generate()
	dataReady.Src = s.cache.topPort().AsRemote()
	dataReady.Dst = trans.ReadMeta.Src
	dataReady.RspTo = trans.ReadMeta.ID
	dataReady.Data = respondData
	dataReady.TrafficBytes = len(respondData) + 4
	dataReady.TrafficClass = "memprotocol.DataReadyRsp"
	s.cache.topPort().Send(dataReady)

	tracing.TraceReqComplete(s.cache.comp, trans.ReadMeta)
}

func (s *mshrStage) respondWrite(trans *transactionState) {
	writeDoneRsp := memprotocol.WriteDoneRsp{}
	writeDoneRsp.ID = timing.GetIDGenerator().Generate()
	writeDoneRsp.Src = s.cache.topPort().AsRemote()
	writeDoneRsp.Dst = trans.WriteMeta.Src
	writeDoneRsp.RspTo = trans.WriteMeta.ID
	writeDoneRsp.TrafficBytes = 4
	writeDoneRsp.TrafficClass = "memprotocol.WriteDoneRsp"
	s.cache.topPort().Send(writeDoneRsp)

	tracing.TraceReqComplete(s.cache.comp, trans.WriteMeta)
}

func (s *mshrStage) findTransaction(transIdx int) bool {
	next := &s.cache.comp.State
	if transIdx < 0 || transIdx >= len(next.Transactions) {
		return false
	}
	return !next.Transactions[transIdx].Removed
}
