package timing

import (
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/timing/wavefront"
	"gitlab.com/akita/mem"
)

type defaultCoalescer struct {
	log2CacheLineSize uint64
}

func (c defaultCoalescer) generateMemTransactions(
	wf *wavefront.Wavefront,
) []VectorMemAccessInfo {
	c.mustBeAFlatLoadOrStore(wf)
	c.executionMaskMustNotBeAllZero(wf)
	reqs := c.generateReadReqs(wf)
	transactions := c.generateReadTransactions(wf, reqs)
	return transactions
}

func (c defaultCoalescer) mustBeAFlatLoadOrStore(wf *wavefront.Wavefront) {
	if wf.Inst().FormatType != insts.FLAT {
		panic("must be a flat instruction")
	}

	if wf.Inst().Opcode < 16 || wf.Inst().Opcode > 31 {
		panic("must be a load or store instruction")
	}
}

func (c defaultCoalescer) executionMaskMustNotBeAllZero(wf *wavefront.Wavefront) {
	sp := wf.Scratchpad().AsFlat()
	exec := sp.EXEC
	if exec == 0 {
		panic("execution mask is all 0")
	}
}

func (c defaultCoalescer) generateReadReqs(
	wf *wavefront.Wavefront,
) []*mem.ReadReq {
	sp := wf.Scratchpad().AsFlat()
	exec := sp.EXEC
	addrs := sp.ADDR
	reqs := []*mem.ReadReq{}

	for i := uint(0); i < 64; i++ {
		if !laneMasked(exec, i) {
			continue
		}

		addr := addrs[i]
		c.findOrCreateReadReq(&reqs, addr)
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

func (c defaultCoalescer) findOrCreateReadReq(
	reqs *[]*mem.ReadReq,
	addr uint64,
) *mem.ReadReq {
	for _, req := range *reqs {
		if c.isInSameCacheLine(addr, req.Address) {
			return req
		}
	}

	req := mem.NewReadReq(0, nil, nil,
		c.cacheLineID(addr), 1<<c.log2CacheLineSize)
	*reqs = append(*reqs, req)
	return req
}

func (c defaultCoalescer) addLaneInfo(
	transaction *VectorMemAccessInfo,
	wf *wavefront.Wavefront,
) {
	sp := wf.Scratchpad().AsFlat()
	exec := sp.EXEC
	addrs := sp.ADDR
	req := transaction.Read

	for i := uint(0); i < 64; i++ {
		if !laneMasked(exec, i) {
			continue
		}

		addr := addrs[i]
		if c.isInSameCacheLine(addr, req.Address) {
			laneInfo := vectorMemAccessLaneInfo{
				laneID:                int(i),
				reg:                   wf.Inst().Dst.Register,
				regCount:              wf.Inst().Dst.RegCount,
				addrOffsetInCacheLine: c.addrOffsetInCacheLine(addr),
			}
			transaction.laneInfo = append(transaction.laneInfo, laneInfo)
		}
	}
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
