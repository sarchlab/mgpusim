package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type mshrStage struct {
	cache *pipelineMW
}

func (s *mshrStage) Tick() bool {
	next := s.cache.comp.GetNextState()

	if next.HasProcessingMSHREntry {
		return s.processOneReq()
	}

	mshrBuf := &next.MSHRStageBuf

	if mshrBuf.Size() == 0 {
		return false
	}

	transIdx := mshrBuf.Elements[0]
	mshrBuf.Elements = mshrBuf.Elements[1:]
	trans := &next.Transactions[transIdx]
	next.HasProcessingMSHREntry = true
	next.ProcessingMSHREntryIdx = transIdx

	// Copy data into local state fields for processing
	_ = trans

	return s.processOneReq()
}

func (s *mshrStage) Reset() {
	next := s.cache.comp.GetNextState()
	next.HasProcessingMSHREntry = false
	next.ProcessingMSHREntryIdx = 0
	next.MSHRStageBuf.Clear()
}

func (s *mshrStage) processOneReq() bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	next := s.cache.comp.GetNextState()
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

	spec := s.cache.comp.GetSpec()

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
	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = s.cache.topPort.AsRemote()
	dataReady.Dst = trans.ReadMeta.Src
	dataReady.RspTo = trans.ReadMeta.ID
	dataReady.Data = respondData
	dataReady.TrafficBytes = len(respondData) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"
	s.cache.topPort.Send(dataReady)

	tracing.TraceReqComplete(&trans.ReadMeta, s.cache.comp)
}

func (s *mshrStage) respondWrite(trans *transactionState) {
	writeDoneRsp := &mem.WriteDoneRsp{}
	writeDoneRsp.ID = sim.GetIDGenerator().Generate()
	writeDoneRsp.Src = s.cache.topPort.AsRemote()
	writeDoneRsp.Dst = trans.WriteMeta.Src
	writeDoneRsp.RspTo = trans.WriteMeta.ID
	writeDoneRsp.TrafficBytes = 4
	writeDoneRsp.TrafficClass = "mem.WriteDoneRsp"
	s.cache.topPort.Send(writeDoneRsp)

	tracing.TraceReqComplete(&trans.WriteMeta, s.cache.comp)
}

func (s *mshrStage) findTransaction(transIdx int) bool {
	next := s.cache.comp.GetNextState()
	if transIdx < 0 || transIdx >= len(next.Transactions) {
		return false
	}
	return !next.Transactions[transIdx].Removed
}
