package timing

import (
	"gitlab.com/akita/mgpusim"
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/mgpusim/timing/wavefront"
)

// WGMapper defines the behavior of how a workgroup is mapped in the compute
// unit.
//
// It is responsible for allocating SIMD number, VGPRs offset, SGPRs
// offset and LDS offset for each wavefront in the workgroup.
// A WGMapper is not a component and we assume the mapping process is done
// within a cycle
type WGMapper interface {
	MapWG(req *mgpusim.MapWGReq) bool
	UnmapWG(wg *wavefront.WorkGroup)
}

// WGMapperImpl is a sub-component of scheduler. It is responsible for allocate
// and reserve resources for the incoming MapWgReq.
type WGMapperImpl struct {
	cu *ComputeUnit

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

	nextSIMD int
}

// NewWGMapper returns a newly created WgMapper with default compute unit
// setting
func NewWGMapper(cu *ComputeUnit, numWfPool int) *WGMapperImpl {
	m := new(WGMapperImpl)

	m.cu = cu

	m.NumWfPool = numWfPool

	m.initWfInfo([]int{10, 10, 10, 10})
	m.initLDSInfo(64 * 1024) // 64K
	m.initSGPRInfo(3200)
	m.initVGPRInfo([]int{256, 256, 256, 256}) // 64KB per SIMD, 64 lanes, 4 bytes

	m.nextSIMD = 0

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
	m.VGprGranularity = 4 // 4 register minimum allocation
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
		vgprCount[i] = 1024
	}
	m.initVGPRInfo(vgprCount)
}

// MapWG uses a first fit algorithm to allocate SGPR, VGPR, and LDS resources.
// In terms of SIMD selection, it uses a round robin policy.
func (m *WGMapperImpl) MapWG(req *mgpusim.MapWGReq) bool {
	ok := true

	m.cu.WfToDispatch = make(map[*kernels.Wavefront]*WfDispatchInfo)
	for _, wf := range req.WG.Wavefronts {
		info := new(WfDispatchInfo)
		info.Wavefront = wf
		m.cu.WfToDispatch[wf] = info
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

func (m *WGMapperImpl) withinSGPRLimitation(req *mgpusim.MapWGReq) bool {
	co := req.WG.CodeObject
	required := m.unitsOccupy(int(co.WFSgprCount), m.SGprGranularity)
	for _, wf := range req.WG.Wavefronts {
		// for _, info := range m.cu.WfToDispatch {
		info := m.cu.WfToDispatch[wf]
		offset, ok := m.SGprMask.NextRegion(required, AllocStatusFree)
		if !ok {
			return false
		}
		info.SGPROffset = offset * 16 * 4 // 16 reg, 4 byte each
		m.SGprMask.SetStatus(offset, required, AllocStatusToReserve)
	}
	return true
}

func (m *WGMapperImpl) withinLDSLimitation(req *mgpusim.MapWGReq) bool {
	co := req.WG.CodeObject
	required := m.unitsOccupy(int(co.WGGroupSegmentByteSize), m.LDSGranularity)
	offset, ok := m.LDSMask.NextRegion(required, AllocStatusFree)
	if !ok {
		return false
	}

	// Set the information
	for _, wf := range req.WG.Wavefronts {
		info := m.cu.WfToDispatch[wf]
		info.LDSOffset = offset * m.LDSGranularity
	}
	m.LDSMask.SetStatus(offset, required, AllocStatusToReserve)
	return true
}

// Maps the wfs of a work-group to the SIMDs in the compute unit
// This function sets the value of req.WfDispatchMap, to keep the information
// about which SIMD should a wf dispatch to. This function also returns
// a boolean value for if the matching is successful.
func (m *WGMapperImpl) matchWfWithSIMDs(req *mgpusim.MapWGReq) bool {
	vgprToUse := make([]int, m.NumWfPool)
	wfPoolEntryUsed := make([]int, m.NumWfPool)
	co := req.WG.CodeObject

	for _, wf := range req.WG.Wavefronts {
		info := m.cu.WfToDispatch[wf]
		firstSIMDTested := m.nextSIMD
		firstTry := true
		found := false
		required := m.unitsOccupy(int(co.WIVgprCount), m.VGprGranularity)
		for firstTry || m.nextSIMD != firstSIMDTested {
			firstTry = false
			offset, ok := m.VGprMask[m.nextSIMD].NextRegion(required, AllocStatusFree)

			if ok && m.WfPoolFreeCount[m.nextSIMD]-wfPoolEntryUsed[m.nextSIMD] > 0 {
				found = true
				vgprToUse[m.nextSIMD] += required
				wfPoolEntryUsed[m.nextSIMD]++
				info.SIMDID = m.nextSIMD
				info.VGPROffset = offset * 4 * 4 // 4 regs per group, 4 bytes
				m.VGprMask[m.nextSIMD].SetStatus(offset, required,
					AllocStatusToReserve)
			}
			m.nextSIMD++
			if m.nextSIMD >= m.NumWfPool {
				m.nextSIMD = 0
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

func (m *WGMapperImpl) reserveResources(req *mgpusim.MapWGReq) {
	for _, info := range m.cu.WfToDispatch {
		m.WfPoolFreeCount[info.SIMDID]--
	}

	m.SGprMask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	m.LDSMask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	for i := 0; i < m.NumWfPool; i++ {
		m.VGprMask[i].ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	}
}

func (m *WGMapperImpl) clearTempReservation(req *mgpusim.MapWGReq) {
	m.cu.WfToDispatch = nil
	m.SGprMask.ConvertStatus(AllocStatusToReserve, AllocStatusFree)
	m.LDSMask.ConvertStatus(AllocStatusToReserve, AllocStatusFree)
	for i := 0; i < m.NumWfPool; i++ {
		m.VGprMask[i].ConvertStatus(AllocStatusToReserve,
			AllocStatusFree)
	}
}

// UnmapWG will remove all the resource reservation of a work-group
func (m *WGMapperImpl) UnmapWG(wg *wavefront.WorkGroup) {
	co := wg.CodeObject
	for _, wf := range wg.Wfs {
		m.WfPoolFreeCount[wf.SIMDID]++

		ldsUnits := m.unitsOccupy(int(co.WGGroupSegmentByteSize),
			m.LDSGranularity)
		m.LDSMask.SetStatus(wf.LDSOffset/m.LDSGranularity, ldsUnits,
			AllocStatusFree)

		sgprUnits := m.unitsOccupy(int(co.WFSgprCount), m.SGprGranularity)
		m.SGprMask.SetStatus(wf.SRegOffset/4/m.SGprGranularity,
			sgprUnits, AllocStatusFree)

		vgprUnits := m.unitsOccupy(int(co.WIVgprCount), m.VGprGranularity)
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
