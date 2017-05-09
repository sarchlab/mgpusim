package cu

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/timing"
)

// MapWGEvent requires the Scheduler to reserve space for a workgroup.
// The workgroup will not run immediately. The dispatcher will wait for the
// scheduler to dispatch wavefronts to it.
type MapWGEvent struct {
	*core.BasicEvent

	Req *timing.MapWGReq
}

// NewMapWGEvent creates a new MapWGEvent
func NewMapWGEvent(
	handler core.Handler,
	time core.VTimeInSec,
	req *timing.MapWGReq,
) *MapWGEvent {
	e := new(MapWGEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.SetHandler(handler)
	e.SetTime(time)
	e.Req = req
	return e
}

// WGMapper defines the behavior of how a workgroup is mapped in the compute
// unit.
//
// It is responsible for allocating SIMD number, VGPRs offset, SGPRs
// offset and LDS offset for each wavefront in the workgroup.
// A WGMapper is not a component and we assume the mapping process is done
// within a cycle
type WGMapper interface {
	MapWG(req *timing.MapWGReq) bool
	UnmapWG(wg *WorkGroup)
}

// WGMapperImpl is a sub-component of scheduler. It is responsible for allocate
// and reserve resources for the incomming MapWgReq.
type WGMapperImpl struct {
	NumWfPool       int
	WfPoolFreeCount []int

	SGprCount       int
	SGprGranularity int
	SGprMask        *ResourceMask

	VGprCount       []int
	VGprGranularity int
	VGprMask        []*ResourceMask

	LDSByteSize    int
	LDSGranularity int
	LDSMask        *ResourceMask
}

// NewWGMapper returns a newly created WgMapper with default compute unit
// setting
func NewWGMapper(numWfPool int) *WGMapperImpl {
	m := new(WGMapperImpl)

	m.NumWfPool = numWfPool

	m.initWfInfo([]int{10, 10, 10, 10})
	m.initLDSInfo(64 * 1024) // 64K
	m.initSGPRInfo(2048)
	m.initVGPRInfo([]int{16384, 16384, 16384, 16384})

	return m
}

func (m *WGMapperImpl) initWfInfo(numWfsPerPool []int) {
	m.WfPoolFreeCount = numWfsPerPool
}

func (m *WGMapperImpl) initSGPRInfo(count int) {
	m.SGprCount = count
	m.SGprGranularity = 16
	m.SGprMask = NewResourceMask(m.SGprCount / m.SGprGranularity)
}

func (m *WGMapperImpl) initLDSInfo(byteSize int) {
	m.LDSByteSize = byteSize
	m.LDSGranularity = 256
	m.LDSMask = NewResourceMask(m.LDSByteSize / m.LDSGranularity)
}

func (m *WGMapperImpl) initVGPRInfo(count []int) {
	m.VGprCount = count
	m.VGprGranularity = 64 * 4 // 64 lanes, 4 register minimum allocation
	m.VGprMask = make([]*ResourceMask, 0, m.NumWfPool)
	for i := 0; i < m.NumWfPool; i++ {
		m.VGprMask = append(m.VGprMask,
			NewResourceMask(m.VGprCount[i]/m.VGprGranularity))
	}
}

// SetWfPoolSizes updates the number of WfPools and it number of wavefronts
// that a wavefront pool can handle.
func (m *WGMapperImpl) SetWfPoolSizes(numWfs []int) {
	m.NumWfPool = len(numWfs)
	m.initWfInfo(numWfs)

	vgprCount := make([]int, len(numWfs))
	for i := 0; i < len(numWfs); i++ {
		vgprCount[i] = 16384
	}
	m.initVGPRInfo(vgprCount)
}

// MapWG uses a first fit algorithm to allocate SGPR, VGPR, and LDS resources.
// In terms of SIMD selection, it uses a round robin policy.
func (m *WGMapperImpl) MapWG(req *timing.MapWGReq) bool {
	ok := true

	for _, wf := range req.WG.Wavefronts {
		req.WfDispatchMap[wf] = new(timing.WfDispatchInfo)
	}

	if !m.withinSGPRLimitation(req) || !m.withinLDSLimitation(req) {
		ok = false
	}

	if ok && !m.matchWfWithSIMDs(req) {
		ok = false
	}

	if ok {
		m.reserveResources(req)
	} else {
		m.clearTempReservation(req)
	}

	return ok
}

func (m *WGMapperImpl) withinSGPRLimitation(req *timing.MapWGReq) bool {
	co := req.KernelStatus.CodeObject
	required := m.unitsOccupy(int(co.WFSgprCount), m.SGprGranularity)
	for _, wf := range req.WG.Wavefronts {
		offset, ok := m.SGprMask.NextRegion(required, AllocStatusFree)
		if !ok {
			return false
		}
		req.WfDispatchMap[wf].SGPROffset = offset * 64 // 16 reg, 4 byte each
		m.SGprMask.SetStatus(offset, required, AllocStatusToReserve)
	}
	return true
}

func (m *WGMapperImpl) withinLDSLimitation(req *timing.MapWGReq) bool {
	co := req.KernelStatus.CodeObject
	required := m.unitsOccupy(int(co.WGGroupSegmentByteSize), m.LDSGranularity)
	offset, ok := m.LDSMask.NextRegion(required, AllocStatusFree)
	if !ok {
		return false
	}

	// Set the information
	for _, wf := range req.WG.Wavefronts {
		req.WfDispatchMap[wf].LDSOffset = offset * m.LDSGranularity
	}
	m.LDSMask.SetStatus(offset, required, AllocStatusToReserve)
	return true
}

// Maps the wfs of a workgroup to the SIMDs in the compute unit
// This function sets the value of req.WfDispatchMap, to keep the information
// about which SIMD should a wf dispatch to. This function also returns
// a boolean value for if the matching is successful.
func (m *WGMapperImpl) matchWfWithSIMDs(req *timing.MapWGReq) bool {
	nextSIMD := 0
	vgprToUse := make([]int, m.NumWfPool)
	wfPoolEntryUsed := make([]int, m.NumWfPool)
	co := req.KernelStatus.CodeObject

	for i := 0; i < len(req.WG.Wavefronts); i++ {
		firstSIMDTested := nextSIMD
		firstTry := true
		found := false
		required := m.unitsOccupy(int(co.WIVgprCount)*64, m.VGprGranularity)
		for firstTry || nextSIMD != firstSIMDTested {
			firstTry = false
			offset, ok := m.VGprMask[nextSIMD].NextRegion(required, AllocStatusFree)

			if ok && m.WfPoolFreeCount[nextSIMD]-wfPoolEntryUsed[nextSIMD] > 0 {
				found = true
				vgprToUse[nextSIMD] += required
				wfPoolEntryUsed[nextSIMD]++
				req.WfDispatchMap[req.WG.Wavefronts[i]].SIMDID = nextSIMD
				req.WfDispatchMap[req.WG.Wavefronts[i]].VGPROffset =
					offset * 4 * 64 * 4 // 4 regs per group, 64 lanes, 4 bytes
				m.VGprMask[nextSIMD].SetStatus(offset, required,
					AllocStatusToReserve)
			}
			nextSIMD++
			if nextSIMD >= m.NumWfPool {
				nextSIMD = 0
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (m *WGMapperImpl) reserveResources(req *timing.MapWGReq) {
	for _, info := range req.WfDispatchMap {
		m.WfPoolFreeCount[info.SIMDID]--
	}

	m.SGprMask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	m.LDSMask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	for i := 0; i < m.NumWfPool; i++ {
		m.VGprMask[i].ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	}
}

func (m *WGMapperImpl) clearTempReservation(req *timing.MapWGReq) {
	m.SGprMask.ConvertStatus(AllocStatusToReserve, AllocStatusFree)
	m.LDSMask.ConvertStatus(AllocStatusToReserve, AllocStatusFree)
	for i := 0; i < m.NumWfPool; i++ {
		m.VGprMask[i].ConvertStatus(AllocStatusToReserve,
			AllocStatusFree)
	}
}

// UnmapWG will remove all the resource reservation of a workgroup
func (m *WGMapperImpl) UnmapWG(wg *WorkGroup) {
	req := wg.MapReq
	co := req.KernelStatus.CodeObject
	for _, wf := range wg.Wfs {
		m.WfPoolFreeCount[wf.SIMDID]++

		ldsUnits := m.unitsOccupy(int(co.WGGroupSegmentByteSize),
			m.LDSGranularity)
		m.LDSMask.SetStatus(wf.LDSOffset/m.LDSGranularity, ldsUnits,
			AllocStatusFree)

		sgprUnits := m.unitsOccupy(int(co.WFSgprCount), m.SGprGranularity)
		m.SGprMask.SetStatus(wf.SRegOffset/4/m.SGprGranularity,
			sgprUnits, AllocStatusFree)

		vgprUnits := m.unitsOccupy(int(co.WIVgprCount)*64, m.VGprGranularity)
		m.VGprMask[wf.SIMDID].SetStatus(
			wf.VRegOffset/4/m.VGprGranularity, vgprUnits,
			AllocStatusFree)
	}
}

func (m *WGMapperImpl) unitsOccupy(amount, granularity int) int {
	if amount%granularity == 0 {
		return amount / granularity
	}
	return amount/granularity + 1
}
