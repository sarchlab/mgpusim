package driver

import (
	"bytes"
	"encoding/binary"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// defaultMemoryCopyMiddleware handles memory copy commands and related
// communication.
type defaultMemoryCopyMiddleware struct {
	driver *Driver

	cyclesPerH2D int
	cyclesPerD2H int
	cyclesLeft   int

	awaitingReqs []messaging.Msg
}

func (m *defaultMemoryCopyMiddleware) ProcessCommand(
	cmd Command,
	queue *CommandQueue,
) (processed bool) {
	switch cmd := cmd.(type) {
	case *MemCopyH2DCommand:
		m.driver.logCmdStart(cmd)
		return m.processMemCopyH2DCommand(cmd, queue)
	case *MemCopyD2HCommand:
		m.driver.logCmdStart(cmd)
		return m.processMemCopyD2HCommand(cmd, queue)
	}

	return false
}

func (m *defaultMemoryCopyMiddleware) processMemCopyH2DCommand(
	cmd *MemCopyH2DCommand,
	queue *CommandQueue,
) bool {
	if m.needFlushing(queue.Context, cmd.Dst, uint64(binary.Size(cmd.Src))) {
		m.sendFlushRequest(cmd)
	}

	buffer := bytes.NewBuffer(nil)
	err := binary.Write(buffer, binary.LittleEndian, cmd.Src)
	if err != nil {
		panic(err)
	}
	rawBytes := buffer.Bytes()

	offset := uint64(0)
	addr := uint64(cmd.Dst)
	sizeLeft := uint64(len(rawBytes))
	for sizeLeft > 0 {
		page, found := m.driver.Resources.PageTable.
			Find(queue.Context.pid, addr)
		if !found {
			panic("page not found")
		}

		pAddr := page.PAddr + (addr - page.VAddr)
		sizeLeftInPage := page.PageSize - (addr - page.VAddr)
		sizeToCopy := sizeLeftInPage
		if sizeLeft < sizeLeftInPage {
			sizeToCopy = sizeLeft
		}

		gpuID := m.driver.memAllocator.GetDeviceIDByPAddr(pAddr)
		req := messaging.Msg{ID: m.driver.NewID(),
			Src: m.driver.gpuPort().AsRemote(),
			Dst: m.driver.GPUs[gpuID-1],
			Payload: protocol.MemCopyH2DReq{SrcBuffer: rawBytes[offset : offset+sizeToCopy],

				DstAddress: pAddr}}

		cmd.Reqs = append(cmd.Reqs, req)
		m.awaitingReqs = append(m.awaitingReqs, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		m.driver.logTaskToGPUInitiate(cmd, req)
	}

	m.cyclesLeft = m.cyclesPerH2D

	queue.IsRunning = true

	return true
}

func (m *defaultMemoryCopyMiddleware) processMemCopyD2HCommand(
	cmd *MemCopyD2HCommand,
	queue *CommandQueue,
) bool {
	if m.needFlushing(queue.Context, cmd.Src, uint64(copyByteSize(cmd))) {
		m.sendFlushRequest(cmd)
		queue.Context.removeFreedBuffers()
	}

	cmd.RawData = make([]byte, copyByteSize(cmd))

	offset := uint64(0)
	addr := uint64(cmd.Src)
	sizeLeft := uint64(len(cmd.RawData))
	for sizeLeft > 0 {
		page, found := m.driver.Resources.PageTable.
			Find(queue.Context.pid, addr)
		if !found {
			panic("page not found")
		}

		pAddr := page.PAddr + (addr - page.VAddr)
		sizeLeftInPage := page.PageSize - (addr - page.VAddr)
		sizeToCopy := sizeLeftInPage
		if sizeLeft < sizeLeftInPage {
			sizeToCopy = sizeLeft
		}

		gpuID := m.driver.memAllocator.GetDeviceIDByPAddr(pAddr)
		req := messaging.Msg{ID: m.driver.NewID(),
			Src: m.driver.gpuPort().AsRemote(),
			Dst: m.driver.GPUs[gpuID-1], Payload: protocol.MemCopyD2HReq{SrcAddress: pAddr,
				ByteSize: sizeToCopy, DstOffset: offset}}

		cmd.Reqs = append(cmd.Reqs, req)
		m.awaitingReqs = append(m.awaitingReqs, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		m.driver.logTaskToGPUInitiate(cmd, req)
	}

	m.cyclesLeft = m.cyclesPerD2H

	queue.IsRunning = true
	return true
}

func (m *defaultMemoryCopyMiddleware) needFlushing(
	ctx *Context,
	vAddr Ptr,
	size uint64,
) bool {
	startAddr := uint64(vAddr)
	endAddr := uint64(vAddr) + size
	for _, buf := range ctx.buffers {
		bufStartAddr := uint64(buf.vAddr)
		bufEndAddr := uint64(buf.vAddr) + buf.size
		if memRangeOverlap(bufStartAddr, bufEndAddr, startAddr, endAddr) {
			if buf.l2Dirty {
				return true
			}
		}
	}

	return false
}

func memRangeOverlap(
	start1, end1, start2, end2 uint64,
) bool {
	if start1 <= start2 && end1 > start2 {
		return true
	}

	if start1 < end2 && end1 >= end2 {
		return true
	}

	return false
}

func (m *defaultMemoryCopyMiddleware) sendFlushRequest(
	cmd Command,
) {
	for _, gpu := range m.driver.GPUs {
		req := messaging.Msg{ID: m.driver.NewID(),
			Src: m.driver.gpuPort().AsRemote(),
			Dst: gpu, Payload: protocol.FlushReq{}}

		m.driver.requestsToSend = append(m.driver.requestsToSend, req)
		cmd.AddReq(req)

		m.driver.logTaskToGPUInitiate(cmd, req)
	}
}

func (m *defaultMemoryCopyMiddleware) Tick() (madeProgress bool) {
	madeProgress = false

	if m.cyclesLeft > 0 {
		m.cyclesLeft--
		madeProgress = true
	} else if m.cyclesLeft == 0 {
		m.driver.requestsToSend = append(
			m.driver.requestsToSend, m.awaitingReqs...)
		m.awaitingReqs = nil
		m.cyclesLeft = -1
		madeProgress = true
	}

	req, ok := m.driver.gpuPort().PeekIncoming()
	if !ok {
		return madeProgress
	}

	switch req.Payload.(type) {
	case protocol.MemCopyD2HRsp:
		madeProgress = m.processMemCopyD2HRsp(req) || madeProgress
	case protocol.GeneralRsp:

		madeProgress = m.processGeneralRsp(req) || madeProgress
	}

	return madeProgress
}

// processGeneralRsp recovers the original request acknowledged by the
// response (the v5 GeneralRsp only carries the request ID in RspTo, while
// the v4 sim.GeneralRsp embedded the original request) and dispatches on the
// request type.
func (m *defaultMemoryCopyMiddleware) processGeneralRsp(
	rsp messaging.Msg,
) bool {
	madeProgress := false
	originalReq, _, _ := m.driver.findCommandByReqID(rsp.RspTo)

	switch originalReq.Payload.(type) {
	case protocol.FlushReq:

		madeProgress = m.processFlushReturn(originalReq)
	case protocol.MemCopyH2DReq:

		madeProgress = m.processMemCopyH2DReturn(originalReq)
	case protocol.MemCopyD2HReq:

		panic("D2H completion requires returned data")
	}

	return madeProgress
}

func (m *defaultMemoryCopyMiddleware) processMemCopyH2DReturn(
	req messaging.Msg,
) bool {
	m.driver.gpuPort().RetrieveIncoming()

	m.driver.logTaskToGPUClear(req)

	_, cmd, cmdQueue := m.driver.findCommandByReqID(req.ID)

	cmd.RemoveReq(req)
	m.completeCopyIfReady(cmd, cmdQueue)

	return true
}

func (m *defaultMemoryCopyMiddleware) processMemCopyD2HReturn(
	req messaging.Msg,
) bool {
	m.driver.gpuPort().RetrieveIncoming()

	m.driver.logTaskToGPUClear(req)

	_, cmd, cmdQueue := m.driver.findCommandByReqID(req.ID)

	cmd.RemoveReq(req)
	m.completeCopyIfReady(cmd, cmdQueue)

	return true
}

func (m *defaultMemoryCopyMiddleware) processFlushReturn(
	req messaging.Msg,
) bool {
	m.driver.gpuPort().RetrieveIncoming()

	m.driver.logTaskToGPUClear(req)

	_, cmd, cmdQueue := m.driver.findCommandByReqID(req.ID)

	cmd.RemoveReq(req)
	m.completeCopyIfReady(cmd, cmdQueue)

	return true
}

// completeCopyIfReady handles both data-copy and cache-flush replies. A copy
// spans multiple GPUs, so a flush reply may be the last outstanding request.
// Completing only on a data-copy reply leaves the queue permanently running
// when a remote GPU finishes flushing after the data transfer.
func (m *defaultMemoryCopyMiddleware) completeCopyIfReady(
	cmd Command,
	queue *CommandQueue,
) {
	if len(cmd.GetReqs()) != 0 {
		return
	}

	if copyCmd, ok := cmd.(*MemCopyD2HCommand); ok {
		m.driver.deliverResult(copyCmd)
	}

	queue.IsRunning = false
	queue.Dequeue()
	m.driver.logCmdComplete(cmd)
}

func (m *defaultMemoryCopyMiddleware) processMemCopyD2HRsp(rsp messaging.Msg) bool {
	req, command, _ := m.driver.findCommandByReqID(rsp.RspTo)
	cmd := command.(*MemCopyD2HCommand)
	request := req.Payload.(protocol.MemCopyD2HReq)
	data := rsp.Payload.(protocol.MemCopyD2HRsp).Data
	if uint64(len(data)) != request.ByteSize {
		panic("D2H response size differs from request")
	}
	copy(cmd.RawData[request.DstOffset:request.DstOffset+request.ByteSize], data)
	return m.processMemCopyD2HReturn(req)
}
