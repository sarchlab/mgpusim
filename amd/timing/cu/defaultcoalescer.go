package cu

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

type defaultCoalescer struct {
	log2CacheLineSize uint64
}

func (c defaultCoalescer) generateMemTransactions(
	wf *wavefront.Wavefront,
) []VectorMemAccessInfo {
	c.mustBeAFlatLoadOrStore(wf)
	var transactions []VectorMemAccessInfo
	if c.isLoadInst(wf.Inst()) {
		reqs := c.generateReadReqs(wf)
		transactions = c.generateReadTransactions(wf, reqs)
	} else {
		reqs := c.generateWriteReqs(wf)
		transactions = c.generateWriteTransactions(wf, reqs)
	}
	return transactions
}

func (c defaultCoalescer) mustBeAFlatLoadOrStore(
	wf *wavefront.Wavefront,
) {
	if wf.Inst().FormatType != insts.FLAT {
		panic("must be a flat instruction")
	}

	if wf.Inst().Opcode < 16 || wf.Inst().Opcode > 31 {
		panic("must be a load or store instruction")
	}
}

func (c defaultCoalescer) generateReadReqs(
	wf *wavefront.Wavefront,
) []*mem.ReadReq {
	exec := wf.EXEC()
	inst := wf.Inst()
	reqs := []*mem.ReadReq{}
	regCount := c.instRegCount(inst)

	for i := uint(0); i < 64; i++ {
		if !laneMasked(exec, i) {
			continue
		}

		addr := c.readFlatAddr(wf, int(i))
		for j := 0; j < regCount; j++ {
			c.findOrCreateReadReq(&reqs, addr+uint64(4*j))
		}
	}

	return reqs
}

func (c defaultCoalescer) generateWriteReqs(
	wf *wavefront.Wavefront,
) []*mem.WriteReq {
	exec := wf.EXEC()
	inst := wf.Inst()
	reqs := []*mem.WriteReq{}

	for i := uint(0); i < 64; i++ {
		if !laneMasked(exec, i) {
			continue
		}

		addr := c.readFlatAddr(wf, int(i))
		regCount := uint(c.instRegCount(inst))
		for j := uint(0); j < regCount; j++ {
			dataReg := insts.NewVRegOperand(
				inst.Data.Register.RegIndex()+int(j),
				inst.Data.Register.RegIndex()+int(j),
				1,
			)
			dataVal := uint32(wf.ReadOperand(dataReg, int(i)))
			c.findOrCreateWriteReq(&reqs, addr+uint64(j*4),
				insts.Uint32ToBytes(dataVal))
		}
	}

	return reqs
}

func (c defaultCoalescer) generateReadTransactions(
	wf *wavefront.Wavefront,
	reqs []*mem.ReadReq,
) []VectorMemAccessInfo {
	transactions := []VectorMemAccessInfo{}
	for _, req := range reqs {
		transaction := VectorMemAccessInfo{
			Read:      req,
			Wavefront: wf,
			Inst:      wf.DynamicInst(),
		}

		c.addLaneInfo(&transaction, wf)

		transactions = append(transactions, transaction)
	}
	return transactions
}

func (c defaultCoalescer) generateWriteTransactions(
	wf *wavefront.Wavefront,
	reqs []*mem.WriteReq,
) []VectorMemAccessInfo {
	transactions := []VectorMemAccessInfo{}
	for _, req := range reqs {
		transaction := VectorMemAccessInfo{
			Write:     req,
			Wavefront: wf,
			Inst:      wf.DynamicInst(),
		}

		transactions = append(transactions, transaction)
	}
	return transactions
}

func (c defaultCoalescer) findOrCreateReadReq(
	reqs *[]*mem.ReadReq,
	addr uint64,
) *mem.ReadReq {
	for _, req := range *reqs {
		if c.isInSameCacheLine(addr, req.Address) {
			return req
		}
	}

	req := mem.ReadReqBuilder{}.
		WithAddress(c.cacheLineID(addr)).
		WithByteSize(1 << c.log2CacheLineSize).
		Build()
	*reqs = append(*reqs, req)
	return req
}

func (c defaultCoalescer) findOrCreateWriteReq(
	reqs *[]*mem.WriteReq,
	addr uint64,
	data []byte,
) *mem.WriteReq {
	for _, req := range *reqs {
		if c.isInSameCacheLine(addr, req.Address) {
			c.mergeDataWithReq(req, addr, data)
			return req
		}
	}

	req := mem.WriteReqBuilder{}.
		WithAddress(c.cacheLineID(addr)).
		WithData(make([]byte, 1<<c.log2CacheLineSize)).
		WithDirtyMask(make([]bool, 1<<c.log2CacheLineSize)).
		Build()
	c.mergeDataWithReq(req, addr, data)
	*reqs = append(*reqs, req)
	return req
}

func (c defaultCoalescer) mergeDataWithReq(
	req *mem.WriteReq,
	addr uint64,
	data []byte,
) {
	c.addressRangeMustFallInReq(req, addr, data)

	offset := c.addrOffsetInCacheLine(addr)

	for i, b := range data {
		req.Data[int(offset)+i] = b
		req.DirtyMask[int(offset)+i] = true
	}
}

func (c defaultCoalescer) addressRangeMustFallInReq(
	req *mem.WriteReq,
	addr uint64,
	data []byte,
) {
	if addr < req.Address {
		panic("addr < req.Address")
	}

	if addr+uint64(len(data)) > req.Address+uint64(len(req.Data)) {
		panic("req cannot hold data")
	}
}

func (c defaultCoalescer) addLaneInfo(
	transaction *VectorMemAccessInfo,
	wf *wavefront.Wavefront,
) {
	exec := wf.EXEC()
	inst := wf.Inst()
	req := transaction.Read
	regCount := c.instRegCount(inst)

	for i := uint(0); i < 64; i++ {
		if !laneMasked(exec, i) {
			continue
		}

		for j := 0; j < regCount; j++ {
			addr := c.readFlatAddr(wf, int(i)) + uint64(j*4)
			reg := insts.VReg(inst.Dst.Register.RegIndex() + j)
			if c.isInSameCacheLine(addr, req.Address) {
				laneInfo := vectorMemAccessLaneInfo{
					laneID:                int(i),
					reg:                   reg,
					regCount:              1,
					addrOffsetInCacheLine: c.addrOffsetInCacheLine(addr),
				}
				transaction.laneInfo = append(
					transaction.laneInfo, laneInfo)
			}
		}
	}
}

func (c defaultCoalescer) readFlatAddr(
	wf *wavefront.Wavefront,
	laneID int,
) uint64 {
	inst := wf.Inst()

	// Handle SAddr mode
	// Use inst.Addr.RegCount to determine mode (consistent with disassembler):
	// RegCount=1 means SAddr mode (32-bit VGPR offset + scalar base)
	// RegCount=2 means OFF mode (64-bit VGPR pair as full address)
	hasSAddr := inst.Addr.RegCount == 1
	var scalarBase uint64
	if hasSAddr && inst.SAddr != nil {
		sAddrReg := int(inst.SAddr.IntValue)
		sAddrOperand := insts.NewSRegOperand(sAddrReg, sAddrReg, 2)
		scalarBase = wf.ReadOperand(sAddrOperand, 0)
	}

	var signedOffset int64
	if inst.Offset0 != 0 {
		signedOffset = int64(int32(inst.Offset0))
	}

	vgprAddr := wf.ReadOperand(inst.Addr, laneID)

	var finalAddr uint64
	if hasSAddr {
		finalAddr = scalarBase + (vgprAddr & 0xFFFFFFFF)
	} else {
		finalAddr = vgprAddr
	}

	finalAddr = uint64(int64(finalAddr) + signedOffset)

	return finalAddr
}

func (c defaultCoalescer) isInSameCacheLine(addr1, addr2 uint64) bool {
	return c.cacheLineID(addr1) == c.cacheLineID(addr2)
}

func (c defaultCoalescer) cacheLineID(addr uint64) uint64 {
	return addr >> c.log2CacheLineSize << c.log2CacheLineSize
}

func (c defaultCoalescer) addrOffsetInCacheLine(addr uint64) uint64 {
	return addr & ((1 << c.log2CacheLineSize) - 1)
}

func (c defaultCoalescer) isLoadInst(inst *insts.Inst) bool {
	return inst.Opcode >= 6 && inst.Opcode <= 23
}

func (c defaultCoalescer) instRegCount(inst *insts.Inst) int {
	switch inst.Opcode {
	case 16, 17, 18, 19, 20:
		return 1
	case 24, 25, 26, 27, 28:
		return 1
	case 21, 29:
		return 2
	case 22, 30:
		return 3
	case 23, 31:
		return 4
	default:
		panic("not supported opcode")
	}
}

func laneMasked(exec uint64, laneID uint) bool {
	return exec&(1<<laneID) > 0
}
