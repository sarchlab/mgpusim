package resource

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// CUResourceImpl implements CUResource
type CUResourceImpl struct {
	config CUConfig
	state  func() *CUState
}

// DispatchingPort returns the port that the dispatcher send message to.
func (r *CUResourceImpl) DispatchingPort() messaging.RemotePort {
	return r.config.Port
}

// ReserveResourceForWG checks if there is space to hold the work-group. If so,
// this function reserves the resouces for the work-group and returns how the
// resources are allocated.
func (r *CUResourceImpl) ReserveResourceForWG(wg *kernels.WorkGroup) (
	locations []WfLocation,
	ok bool,
) {
	ok = true
	locations = make([]WfLocation, len(wg.Wavefronts))

	for i, wf := range wg.Wavefronts {
		locations[i].WavefrontID = wf.UID
	}

	if !r.withinSGPRLimitation(wg, locations) {
		ok = false
	}

	if ok && !r.withinLDSLimitation(wg, locations) {
		ok = false
	}

	if ok && !r.matchWfWithSIMDs(wg, locations) {
		ok = false
	}

	if ok {
		r.reserveResources(wg, locations)
		return locations, true
	}

	r.clearTempReservation(wg)
	return nil, ok
}

func (r *CUResourceImpl) withinSGPRLimitation(
	wg *kernels.WorkGroup,
	locations []WfLocation,
) bool {
	co := wg.CodeObject
	required := r.unitsOccupy(int(co.WFSgprCount), 16)

	for i := range wg.Wavefronts {
		location := &locations[i]
		offset, ok := r.state().Scalar.nextRegion(required, allocStatusFree)
		if !ok {
			return false
		}
		location.SGPROffset = offset * 16 * 4 // 16 reg, 4 byte each
		r.state().Scalar.setStatus(offset, required, allocStatusToReserve)
	}

	return true
}

func (r *CUResourceImpl) withinLDSLimitation(
	wg *kernels.WorkGroup,
	locations []WfLocation,
) bool {
	co := wg.CodeObject
	required := r.unitsOccupy(int(co.GroupSegmentByteSize), 256)
	offset, ok := r.state().LDS.nextRegion(required, allocStatusFree)
	if !ok {
		return false
	}

	for i := range wg.Wavefronts {
		location := &locations[i]
		location.LDSOffset = offset * 256
	}

	r.state().LDS.setStatus(offset, required, allocStatusToReserve)
	return true
}

// Maps the wfs of a work-group to the SIMDs in the compute unit
// This function sets the value of req.WfDispatchMap, to keep the information
// about which SIMD should a wf dispatch to. This function also returns
// a boolean value for if the matching is successful.
func (r *CUResourceImpl) matchWfWithSIMDs(
	wg *kernels.WorkGroup,
	locations []WfLocation,
) bool {
	vgprToUse := make([]int, len(r.state().FreeWavefronts))
	wfPoolEntryUsed := make([]int, len(r.state().FreeWavefronts))
	co := wg.CodeObject

	for i := range wg.Wavefronts {
		location := &locations[i]
		firstSIMDTested := r.state().NextSIMD
		firstTry := true
		found := false
		required := r.unitsOccupy(int(co.WIVgprCount), 4)

		for firstTry || r.state().NextSIMD != firstSIMDTested {
			firstTry = false
			offset, ok := r.state().Vector[r.state().NextSIMD].
				nextRegion(required, allocStatusFree)

			if ok && r.state().FreeWavefronts[r.state().NextSIMD]-wfPoolEntryUsed[r.state().NextSIMD] > 0 {
				found = true
				vgprToUse[r.state().NextSIMD] += required
				wfPoolEntryUsed[r.state().NextSIMD]++
				location.SIMDID = r.state().NextSIMD
				location.VGPROffset = offset * 4 * 4 //  4 bytes per register
				r.state().Vector[r.state().NextSIMD].setStatus(offset, required,
					allocStatusToReserve)
			}

			r.state().NextSIMD++
			if r.state().NextSIMD >= len(r.state().FreeWavefronts) {
				r.state().NextSIMD = 0
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

func (r *CUResourceImpl) unitsOccupy(amount, granularity int) int {
	if amount%granularity == 0 {
		return amount / granularity
	}
	return amount/granularity + 1
}

func (r *CUResourceImpl) reserveResources(
	wg *kernels.WorkGroup,
	locations []WfLocation,
) {
	for _, location := range locations {
		r.state().FreeWavefronts[location.SIMDID]--
	}

	r.state().Scalar.convertStatus(allocStatusToReserve, allocStatusReserved)
	r.state().LDS.convertStatus(allocStatusToReserve, allocStatusReserved)
	for i := 0; i < len(r.state().FreeWavefronts); i++ {
		r.state().Vector[i].convertStatus(allocStatusToReserve, allocStatusReserved)
	}

	r.neverReserveTwice(wg)
	r.state().Reserved[wg.UID] = locations
}

func (r *CUResourceImpl) neverReserveTwice(wg *kernels.WorkGroup) {
	if _, found := r.state().Reserved[wg.UID]; found {
		panic("reserving a work-group twice")
	}
}

func (r *CUResourceImpl) clearTempReservation(wg *kernels.WorkGroup) {
	r.state().Scalar.convertStatus(allocStatusToReserve, allocStatusFree)
	r.state().LDS.convertStatus(allocStatusToReserve, allocStatusFree)
	for i := 0; i < len(r.state().FreeWavefronts); i++ {
		r.state().Vector[i].convertStatus(allocStatusToReserve, allocStatusFree)
	}
}

// FreeResourcesForWG marks all the resources used by a work-group available.
func (r *CUResourceImpl) FreeResourcesForWG(wg *kernels.WorkGroup) {
	locations, found := r.state().Reserved[wg.UID]
	if !found {
		panic("work-group not found")
	}

	co := wg.CodeObject
	for _, location := range locations {
		r.state().FreeWavefronts[location.SIMDID]++

		ldsUnits := r.unitsOccupy(int(co.GroupSegmentByteSize),
			256)
		r.state().LDS.setStatus(location.LDSOffset/256, ldsUnits,
			allocStatusFree)

		sgprUnits := r.unitsOccupy(int(co.WFSgprCount), 16)
		r.state().Scalar.setStatus(location.SGPROffset/4/16,
			sgprUnits, allocStatusFree)

		vgprUnits := r.unitsOccupy(int(co.WIVgprCount), 4)
		r.state().Vector[location.SIMDID].setStatus(
			location.VGPROffset/4/4, vgprUnits,
			allocStatusFree)
	}

	delete(r.state().Reserved, wg.UID)
}
