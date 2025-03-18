package driver

import (
	"bytes"
	"encoding/binary"
)

// defaultMemoryCopyMiddleware handles memory copy commands and related
// communication.
type globalStorageMemoryCopyMiddleware struct {
	driver *Driver
}

func (m *globalStorageMemoryCopyMiddleware) ProcessCommand(
	cmd Command,
	queue *CommandQueue,
) (processed bool) {
	switch cmd := cmd.(type) {
	case *MemCopyH2DCommand:
		return m.processMemCopyH2DCommand(cmd, queue)
	case *MemCopyD2HCommand:
		return m.processMemCopyD2HCommand(cmd, queue)
	}

	return false
}

func (m *globalStorageMemoryCopyMiddleware) processMemCopyH2DCommand(
	cmd *MemCopyH2DCommand,
	queue *CommandQueue,
) bool {
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

		m.driver.globalStorage.Write(pAddr, rawBytes[offset:offset+sizeToCopy])

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy
	}

	queue.IsRunning = false
	queue.Dequeue()

	return true
}

func (m *globalStorageMemoryCopyMiddleware) processMemCopyD2HCommand(
	cmd *MemCopyD2HCommand,
	queue *CommandQueue,
) bool {
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

		data, _ := m.driver.globalStorage.Read(pAddr, sizeToCopy)
		copy(cmd.RawData[offset:], data)

		sizeLeft -= sizeToCopy
		addr += sizeToCopy
		offset += sizeToCopy
	}

	buf := bytes.NewReader(cmd.RawData)
	err := binary.Read(buf, binary.LittleEndian, cmd.Dst)
	if err != nil {
		panic(err)
	}

	queue.IsRunning = false
	queue.Dequeue()
	return true
}

func (m *globalStorageMemoryCopyMiddleware) Tick() (madeProgress bool) {
	return false
}
