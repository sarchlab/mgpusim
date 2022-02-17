package driver

import (
	"bytes"
	"encoding/binary"

	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mgpusim/v2/protocol"
)

// defaultMemoryCopyMiddleware handles memory copy commands and related
// communication.
type defaultMemoryCopyMiddleware struct {
	driver *Driver
}

func (m *defaultMemoryCopyMiddleware) ProcessCommand(
	now sim.VTimeInSec,
	cmd Command,
	queue *CommandQueue,
) (processed bool) {
	switch cmd := cmd.(type) {
	case *MemCopyH2DCommand:
		return m.processMemCopyH2DCommand(now, cmd, queue)
	case *MemCopyD2HCommand:
		return m.processMemCopyD2HCommand(now, cmd, queue)
	}

	return false
}

func (m *defaultMemoryCopyMiddleware) processMemCopyH2DCommand(
	now sim.VTimeInSec,
	cmd *MemCopyH2DCommand,
	queue *CommandQueue,
) bool {
	if m.needFlushing(queue.Context, cmd.Dst, uint64(binary.Size(cmd.Src))) {
		m.sendFlushRequest(now, cmd)
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
		page, found := m.driver.pageTable.Find(queue.Context.pid, addr)
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
		req := protocol.NewMemCopyH2DReq(now,
			m.driver.gpuPort, m.driver.GPUs[gpuID-1],
			rawBytes[offset:offset+sizeToCopy],
			pAddr)
		cmd.Reqs = append(cmd.Reqs, req)
		m.driver.requestsToSend = append(m.driver.requestsToSend, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		m.driver.logTaskToGPUInitiate(now, cmd, req)
	}

	queue.IsRunning = true

	return true
}

func (m *defaultMemoryCopyMiddleware) processMemCopyD2HCommand(
	now sim.VTimeInSec,
	cmd *MemCopyD2HCommand,
	queue *CommandQueue,
) bool {
	if m.needFlushing(queue.Context, cmd.Src, uint64(binary.Size(cmd.Dst))) {
		m.sendFlushRequest(now, cmd)
		queue.Context.removeFreedBuffers()
	}

	cmd.RawData = make([]byte, binary.Size(cmd.Dst))

	offset := uint64(0)
	addr := uint64(cmd.Src)
	sizeLeft := uint64(len(cmd.RawData))
	for sizeLeft > 0 {
		page, found := m.driver.pageTable.Find(queue.Context.pid, addr)
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
		req := protocol.NewMemCopyD2HReq(now,
			m.driver.gpuPort, m.driver.GPUs[gpuID-1],
			pAddr, cmd.RawData[offset:offset+sizeToCopy])
		cmd.Reqs = append(cmd.Reqs, req)
		m.driver.requestsToSend = append(m.driver.requestsToSend, req)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy

		m.driver.logTaskToGPUInitiate(now, cmd, req)
	}

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
	if start1 <= start2 && end1 >= start2 {
		return true
	}

	if start1 <= end2 && end1 >= end2 {
		return true
	}

	return false
}

func (m *defaultMemoryCopyMiddleware) sendFlushRequest(
	now sim.VTimeInSec,
	cmd Command,
) {
	for _, gpu := range m.driver.GPUs {
		req := protocol.NewFlushReq(now, m.driver.gpuPort, gpu)
		m.driver.requestsToSend = append(m.driver.requestsToSend, req)
		cmd.AddReq(req)

		m.driver.logTaskToGPUInitiate(now, cmd, req)
	}
}

func (m *defaultMemoryCopyMiddleware) Tick(
	now sim.VTimeInSec,
) (madeProgress bool) {
	req := m.driver.gpuPort.Peek()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *protocol.FlushReq:
		return m.processFlushReturn(now, req)
	case *protocol.MemCopyH2DReq:
		return m.processMemCopyH2DReturn(now, req)
	case *protocol.MemCopyD2HReq:
		return m.processMemCopyD2HReturn(now, req)
	}

	return false
}

func (m *defaultMemoryCopyMiddleware) processMemCopyH2DReturn(
	now sim.VTimeInSec,
	req *protocol.MemCopyH2DReq,
) bool {
	m.driver.gpuPort.Retrieve(now)

	m.driver.logTaskToGPUClear(now, req)

	cmd, cmdQueue := m.driver.findCommandByReq(req)

	copyCmd := cmd.(*MemCopyH2DCommand)
	newReqs := make([]sim.Msg, 0, len(copyCmd.Reqs)-1)
	for _, r := range copyCmd.GetReqs() {
		if r != req {
			newReqs = append(newReqs, r)
		}
	}
	copyCmd.Reqs = newReqs

	if len(copyCmd.Reqs) == 0 {
		cmdQueue.IsRunning = false
		cmdQueue.Dequeue()

		m.driver.logCmdComplete(cmd, now)
	}

	return true
}

func (m *defaultMemoryCopyMiddleware) processMemCopyD2HReturn(
	now sim.VTimeInSec,
	req *protocol.MemCopyD2HReq,
) bool {
	m.driver.gpuPort.Retrieve(now)

	m.driver.logTaskToGPUClear(now, req)

	cmd, cmdQueue := m.driver.findCommandByReq(req)

	copyCmd := cmd.(*MemCopyD2HCommand)
	copyCmd.RemoveReq(req)

	if len(copyCmd.Reqs) == 0 {
		cmdQueue.IsRunning = false
		buf := bytes.NewReader(copyCmd.RawData)
		err := binary.Read(buf, binary.LittleEndian, copyCmd.Dst)
		if err != nil {
			panic(err)
		}

		cmdQueue.Dequeue()

		m.driver.logCmdComplete(copyCmd, now)
	}

	return true
}

func (m *defaultMemoryCopyMiddleware) processFlushReturn(
	now sim.VTimeInSec,
	req *protocol.FlushReq,
) bool {
	m.driver.gpuPort.Retrieve(now)

	m.driver.logTaskToGPUClear(now, req)

	cmd, _ := m.driver.findCommandByReq(req)

	cmd.RemoveReq(req)

	m.driver.logTaskToGPUClear(now, req)

	return true
}
