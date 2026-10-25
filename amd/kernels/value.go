package kernels

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// PacketValue preserves an optional dispatch packet without an object pointer.
type PacketValue struct {
	Present bool
	Packet  HsaKernelDispatchPacket
}

func PacketSnapshot(p *HsaKernelDispatchPacket) PacketValue {
	if p == nil {
		return PacketValue{}
	}
	return PacketValue{Present: true, Packet: *p}
}

func (v PacketValue) Restore() *HsaKernelDispatchPacket {
	if !v.Present {
		return nil
	}
	return &v.Packet
}

// WorkGroupValue stores each work-item once. Wavefront membership uses indices
// into WorkItems; all parent links are reconstructed in the receiving process.
type WorkGroupValue struct {
	UID                             uint64
	CodeObject                      insts.CodeObjectValue
	Packet                          PacketValue
	PacketAddress                   uint64
	SizeX, SizeY, SizeZ             int
	IDX, IDY, IDZ                   int
	CurrSizeX, CurrSizeY, CurrSizeZ int
	WorkItems                       []WorkItemValue
	Wavefronts                      []WavefrontValue
}

type WorkItemValue struct{ IDX, IDY, IDZ int }

type WavefrontValue struct {
	UID                   uint64
	PacketAddress         uint64
	FirstWiFlatID         int
	InitExecMask          uint64
	WorkItems             []int
	FinishTime, IssueTime timing.VTimeInPicoSec
}

func WorkGroupSnapshot(wg *WorkGroup) WorkGroupValue {
	v := WorkGroupValue{
		UID:           wg.UID,
		CodeObject:    insts.CodeObjectSnapshot(wg.CodeObject),
		Packet:        PacketSnapshot(wg.Packet),
		PacketAddress: wg.PacketAddress,

		SizeX: wg.SizeX, SizeY: wg.SizeY, SizeZ: wg.SizeZ, IDX: wg.IDX, IDY: wg.IDY, IDZ: wg.IDZ,
		CurrSizeX: wg.CurrSizeX, CurrSizeY: wg.CurrSizeY, CurrSizeZ: wg.CurrSizeZ,
	}
	indices := make(map[*WorkItem]int, len(wg.WorkItems))
	for i, wi := range wg.WorkItems {
		indices[wi] = i
		v.WorkItems = append(v.WorkItems, WorkItemValue{wi.IDX, wi.IDY, wi.IDZ})
	}
	for _, wf := range wg.Wavefronts {
		if wf.WG != wg || wf.Packet != wg.Packet || wf.CodeObject != wg.CodeObject {
			panic("kernel wavefront does not belong to its work-group")
		}
		w := WavefrontValue{
			UID:           wf.UID,
			PacketAddress: wf.PacketAddress,
			FirstWiFlatID: wf.FirstWiFlatID,
			InitExecMask:  wf.InitExecMask,
			FinishTime:    wf.FinishTime,
			IssueTime:     wf.IssueTime,
		}
		for _, wi := range wf.WorkItems {
			index, ok := indices[wi]
			if !ok {
				panic("wavefront work-item missing from its work-group")
			}
			w.WorkItems = append(w.WorkItems, index)
		}
		v.Wavefronts = append(v.Wavefronts, w)
	}
	return v
}

// Restore recreates the local graph, preserving IDs and work-item identity.
func (v WorkGroupValue) Restore() (*WorkGroup, error) {
	wg := &WorkGroup{
		UID:           v.UID,
		CodeObject:    v.CodeObject.Restore(),
		Packet:        v.Packet.Restore(),
		PacketAddress: v.PacketAddress,
		SizeX:         v.SizeX,
		SizeY:         v.SizeY,
		SizeZ:         v.SizeZ,
		IDX:           v.IDX,
		IDY:           v.IDY,
		IDZ:           v.IDZ,
		CurrSizeX:     v.CurrSizeX,
		CurrSizeY:     v.CurrSizeY,
		CurrSizeZ:     v.CurrSizeZ,
	}
	for _, wi := range v.WorkItems {
		wg.WorkItems = append(wg.WorkItems, &WorkItem{WG: wg, IDX: wi.IDX, IDY: wi.IDY, IDZ: wi.IDZ})
	}
	for _, w := range v.Wavefronts {
		wf := &Wavefront{
			UID:           w.UID,
			WG:            wg,
			CodeObject:    wg.CodeObject,
			Packet:        wg.Packet,
			PacketAddress: w.PacketAddress,
			FirstWiFlatID: w.FirstWiFlatID,
			InitExecMask:  w.InitExecMask,
			FinishTime:    w.FinishTime,
			IssueTime:     w.IssueTime,
		}
		for _, i := range w.WorkItems {
			if i < 0 || i >= len(wg.WorkItems) {
				return nil, fmt.Errorf("wavefront %d has invalid work-item index %d", w.UID, i)
			}
			wf.WorkItems = append(wf.WorkItems, wg.WorkItems[i])
		}
		wg.Wavefronts = append(wg.Wavefronts, wf)
	}
	return wg, nil
}
