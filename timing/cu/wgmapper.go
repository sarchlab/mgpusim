package cu

import "gitlab.com/yaotsu/gcn3/timing"

// WgMapper is a sub-component of scheduler. It is responsible for allocate
// and reserve resources for the incomming MapWgReq.
type WgMapper struct {
	Scheduler *Scheduler

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

// NewWgMapper returns a newly created WgMapper with default compute unit
// setting
func NewWgMapper() *WgMapper {
	m := new(WgMapper)

	m.initLDSInfo(64 * 1024) // 64K
	m.initSGPRInfo(2048)
	m.initVGPRInfo([]int{16384, 16384, 16384, 16384})

	return m
}

func (m *WgMapper) initSGPRInfo(count int) {
	m.SGprCount = count
	m.SGprGranularity = 16
	m.SGprMask = NewResourceMask(m.SGprCount / m.SGprGranularity)
}

func (m *WgMapper) initLDSInfo(byteSize int) {
	m.LDSByteSize = byteSize
	m.LDSGranularity = 256
	m.LDSMask = NewResourceMask(m.LDSByteSize / m.LDSGranularity)
}

func (m *WgMapper) initVGPRInfo(count []int) {
	m.VGprCount = count
	m.VGprGranularity = 64 * 4 // 64 lanes, 4 register minimum allocation
	m.VGprMask = make([]*ResourceMask, 0, m.Scheduler.NumWfPool)
	for i := 0; i < m.Scheduler.NumWfPool; i++ {
		m.VGprMask = append(m.VGprMask,
			NewResourceMask(m.VGprCount[i]/m.VGprGranularity))
	}
}

func (m *WgMapper) handleMapWGEvent(evt *MapWGEvent) error {
	req := evt.Req
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
	}

	req.SwapSrcAndDst()
	req.SetSendTime(evt.Time())
	req.Ok = ok
	m.Scheduler.GetConnection("ToDispatcher").Send(req)
	return nil
}

func (m *WgMapper) withinSGPRLimitation(req *timing.MapWGReq) bool {
	required := int(req.KernelStatus.CodeObject.WFSgprCount) / m.SGprGranularity
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

func (m *WgMapper) withinLDSLimitation(req *timing.MapWGReq) bool {
	required := int(req.KernelStatus.CodeObject.WGGroupSegmentByteSize) /
		m.LDSGranularity
	offset, ok := m.LDSMask.NextRegion(required, AllocStatusFree)
	if !ok {
		return false
	}

	// Set the information
	for _, wf := range req.WG.Wavefronts {
		req.WfDispatchMap[wf].LDSOffset = offset * 256
	}
	m.LDSMask.SetStatus(offset, required, AllocStatusToReserve)
	return true
}

// Maps the wfs of a workgroup to the SIMDs in the compute unit
// This function sets the value of req.WfDispatchMap, to keep the information
// about which SIMD should a wf dispatch to. This function also returns
// a boolean value for if the matching is successful.
func (m *WgMapper) matchWfWithSIMDs(req *timing.MapWGReq) bool {
	nextSIMD := 0
	vgprToUse := make([]int, m.Scheduler.NumWfPool)
	wfPoolEntryUsed := make([]int, m.Scheduler.NumWfPool)

	for i := 0; i < len(req.WG.Wavefronts); i++ {
		firstSIMDTested := nextSIMD
		firstTry := true
		found := false
		required := int(req.KernelStatus.CodeObject.WIVgprCount) * 64 /
			m.VGprGranularity
		for firstTry || nextSIMD != firstSIMDTested {
			firstTry = false
			offset, ok := m.VGprMask[nextSIMD].NextRegion(required, AllocStatusFree)

			if ok && m.Scheduler.WfPoolFreeCount[nextSIMD]-wfPoolEntryUsed[nextSIMD] > 0 {
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
			if nextSIMD >= m.Scheduler.NumWfPool {
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

func (m *WgMapper) reserveResources(req *timing.MapWGReq) {
	for _, info := range req.WfDispatchMap {
		m.Scheduler.WfPoolFreeCount[info.SIMDID]--
	}

	m.SGprMask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	m.LDSMask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	for i := 0; i < m.Scheduler.NumWfPool; i++ {
		m.VGprMask[i].ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	}
}
