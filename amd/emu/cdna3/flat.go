package cdna3

import (
	"encoding/binary"
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// scratchOffset computes the byte offset into the per-wave scratch buffer
// for a given lane.
//
// For SCRATCH segment instructions in CDNA3 (GFX942):
//   - SADDR=OFF (0x7F): address = inst_offset only (VADDR is ignored)
//   - SADDR=SGPR pair:  address = SGPR[saddr] + VGPR[vaddr] + inst_offset
//
// Each lane's private region starts at lane * perLaneSize.
func (u *ALU) scratchOffset(state emu.InstEmuState, laneID int) uint64 {
	inst := state.Inst()

	var offset uint64

	// Check if SADDR is OFF (0x7F). When OFF, the VADDR field is ignored
	// and only the immediate inst_offset provides the scratch address.
	// The compiler reuses the VADDR register freely between scratch
	// stores and loads, so reading it would give wrong offsets.
	sAddrOff := inst.SAddr != nil && inst.SAddr.IntValue == 0x7F
	if !sAddrOff {
		// SADDR mode: use SGPR base + VGPR offset
		vgprVal := state.ReadOperand(inst.Addr, laneID) & 0xFFFFFFFF
		offset = vgprVal
		if inst.SAddr != nil {
			sAddrReg := int(inst.SAddr.IntValue)
			sAddrOperand := insts.NewSRegOperand(sAddrReg, sAddrReg, 1)
			offset += state.ReadOperand(sAddrOperand, 0) & 0xFFFFFFFF
		}
	}

	if inst.Offset0 != 0 {
		offset += uint64(int64(int32(inst.Offset0)))
	}

	return uint64(laneID)*uint64(u.scratchSize) + offset
}

// scratchRead reads bytes from the per-wave scratch buffer.
func (u *ALU) scratchRead(offset uint64, byteSize int) []byte {
	end := offset + uint64(byteSize)
	if u.scratch == nil || end > uint64(len(u.scratch)) {
		// Scratch not allocated or out of range — return zeros
		return make([]byte, byteSize)
	}
	buf := make([]byte, byteSize)
	copy(buf, u.scratch[offset:end])
	return buf
}

// scratchWrite writes bytes to the per-wave scratch buffer.
func (u *ALU) scratchWrite(offset uint64, data []byte) {
	end := offset + uint64(len(data))
	if u.scratch == nil || end > uint64(len(u.scratch)) {
		// Scratch not allocated or out of range — silently drop
		return
	}
	copy(u.scratch[offset:end], data)
}

// flatIsLDS returns true if the address should be routed to LDS instead of
// global memory. In GFX9/CDNA3, FLAT instructions use address-based routing.
//
// Only addresses below the page boundary (4096) with zero upper bits
// are unambiguously LDS offsets. Global virtual addresses always
// start at or above the first page, so this check is safe.
func (u *ALU) flatIsLDS(addr uint64) bool {
	ldsSize := uint64(len(u.lds))
	if ldsSize == 0 {
		return false
	}

	// Only treat addresses below the page boundary (4096) as LDS.
	// Global virtual addresses always start at or above 4096.
	return addr < 4096 && addr < ldsSize
}

// flatLDSOffset extracts the LDS byte offset from a FLAT address.
// On real hardware, this is the lower 32 bits of the aperture-tagged address.
func (u *ALU) flatLDSOffset(addr uint64) uint64 {
	return addr & 0xFFFFFFFF
}

// flatReadLDS reads byteSize bytes from LDS at the given FLAT address.
func (u *ALU) flatReadLDS(addr uint64, byteSize int) []byte {
	offset := u.flatLDSOffset(addr)
	end := offset + uint64(byteSize)
	if end > uint64(len(u.lds)) {
		log.Panicf("FLAT LDS read out of range: addr=0x%x, offset=0x%x, size=%d, ldsSize=%d",
			addr, offset, byteSize, len(u.lds))
	}
	buf := make([]byte, byteSize)
	copy(buf, u.lds[offset:end])
	return buf
}

// flatWriteLDS writes data to LDS at the given FLAT address.
func (u *ALU) flatWriteLDS(addr uint64, data []byte) {
	offset := u.flatLDSOffset(addr)
	end := offset + uint64(len(data))
	if end > uint64(len(u.lds)) {
		log.Panicf("FLAT LDS write out of range: addr=0x%x, offset=0x%x, size=%d, ldsSize=%d",
			addr, offset, len(data), len(u.lds))
	}
	copy(u.lds[offset:end], data)
}

// flatPrecomputeScalarBase reads the scalar base register once (lane-invariant).
// Returns (hasSAddr, scalarBase). If hasSAddr is false, scalarBase is unused.
// CDNA3 rules: SAddr is OFF only when value is 0x7F.
func (u *ALU) flatPrecomputeScalarBase(
	state emu.InstEmuState,
) (bool, uint64) {
	inst := state.Inst()
	if inst.SAddr != nil && inst.SAddr.IntValue != 0x7F {
		sAddrReg := int(inst.SAddr.IntValue)
		sAddrOperand := insts.NewSRegOperand(sAddrReg, sAddrReg, 2)
		scalarBase := state.ReadOperand(sAddrOperand, 0)
		return true, scalarBase
	}
	return false, 0
}

// flatAddr computes the effective address for a flat/global instruction
// for a given lane. It handles:
//   - SAddr mode (scalar base + VGPR offset) vs OFF mode (VGPR pair as 64-bit addr)
//   - Signed 13-bit immediate offset (Offset0)
//
// CDNA3 rules: SAddr is OFF only when value is 0x7F.
func (u *ALU) flatAddr(state emu.InstEmuState, laneID int) uint64 {
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)
	return u.flatAddrWithScalar(state, laneID, hasSAddr, scalarBase)
}

// flatAddrWithScalar computes the effective address using a precomputed
// scalar base, avoiding redundant scalar register reads across lanes.
func (u *ALU) flatAddrWithScalar(
	state emu.InstEmuState, laneID int,
	hasSAddr bool, scalarBase uint64,
) uint64 {
	inst := state.Inst()

	// Read the VGPR address component
	addr := state.ReadOperand(inst.Addr, laneID)

	if hasSAddr {
		// SAddr mode: addr = scalar_base + zero_extend(VGPR_32) + offset
		addr = scalarBase + (addr & 0xFFFFFFFF)
	}

	// Add signed immediate offset
	if inst.Offset0 != 0 {
		addr += uint64(int64(int32(inst.Offset0)))
	}

	return addr
}

//nolint:gocyclo,funlen
func (u *ALU) runFlat(state emu.InstEmuState) {
	inst := state.Inst()

	// SCRATCH segment (SEG=1): route loads/stores to per-wave scratch buffer.
	if inst.FlatSeg == 1 {
		u.runScratch(state)
		return
	}

	// In GFX9 (CDNA3), FLAT and GLOBAL instructions share the same 7-bit
	// opcode space. The segment (FLAT vs GLOBAL) is encoded in separate
	// SEG bits, not in the opcode. The emulator handles both identically.
	switch inst.Opcode {
	// D16 load variants (opcodes 0-5)
	case 0: // flat/global_load_ubyte_d16
		u.runFlatLoadUByte(state)
	case 1: // flat/global_load_ubyte_d16_hi
		u.runFlatLoadUByte(state)
	case 2: // flat/global_load_sbyte_d16
		u.runFlatLoadSByte(state)
	case 3: // flat/global_load_sbyte_d16_hi
		u.runFlatLoadSByte(state)
	case 4: // flat/global_load_short_d16
		u.runFlatLoadUShort(state)
	case 5: // flat/global_load_short_d16_hi
		u.runFlatLoadUShort(state)
	// Regular loads (opcodes 16-23)
	case 16:
		u.runFlatLoadUByte(state)
	case 17:
		u.runFlatLoadSByte(state)
	case 18:
		u.runFlatLoadUShort(state)
	case 20:
		u.runFlatLoadDWord(state)
	case 21:
		u.runFlatLoadDWordX2(state)
	case 22:
		u.runFlatLoadDWordX3(state)
	case 23:
		u.runFlatLoadDWordX4(state)
	// Stores (opcodes 24-31)
	case 24:
		u.runFlatStoreByte(state)
	case 26:
		u.runFlatStoreShort(state)
	case 28:
		u.runFlatStoreDWord(state)
	case 29:
		u.runFlatStoreDWordX2(state)
	case 30:
		u.runFlatStoreDWordX3(state)
	case 31:
		u.runFlatStoreDWordX4(state)
	// Atomics (GFX9 opcodes 64-76)
	case 64: // flat/global_atomic_swap
		u.runFlatAtomicSwap(state)
	case 65: // flat/global_atomic_cmpswap
		u.runFlatAtomicCmpSwap(state)
	case 66: // flat/global_atomic_add
		u.runFlatAtomicAdd(state)
	case 67: // flat/global_atomic_sub
		u.runFlatAtomicSub(state)
	// Atomics X2 (64-bit variants, GFX9 opcodes 96-108)
	case 96: // flat/global_atomic_swap_x2
		u.runFlatAtomicSwapX2(state)
	case 97: // flat/global_atomic_cmpswap_x2
		u.runFlatAtomicCmpSwapX2(state)
	case 98: // flat/global_atomic_add_x2
		u.runFlatAtomicAddX2(state)
	case 99: // flat/global_atomic_sub_x2
		u.runFlatAtomicSubX2(state)
	default:
		log.Panicf("Opcode %d for FLAT format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runFlatLoadUByte(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	var result [4]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		var buf []byte
		if u.flatIsLDS(addr) {
			buf = u.flatReadLDS(addr, 1)
		} else {
			buf = u.storageAccessor.Read(pid, addr, 1)
		}

		result[0] = buf[0]
		result[1] = 0
		result[2] = 0
		result[3] = 0
		state.WriteOperandBytes(inst.Dst, i, result[:])
	}
}

func (u *ALU) runFlatLoadSByte(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	var result [4]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		var buf []byte
		if u.flatIsLDS(addr) {
			buf = u.flatReadLDS(addr, 1)
		} else {
			buf = u.storageAccessor.Read(pid, addr, 1)
		}

		signedByte := int8(buf[0])
		binary.LittleEndian.PutUint32(result[:], uint32(int32(signedByte)))
		state.WriteOperandBytes(inst.Dst, i, result[:])
	}
}

func (u *ALU) runFlatLoadUShort(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	var result [4]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		var buf []byte
		if u.flatIsLDS(addr) {
			buf = u.flatReadLDS(addr, 2)
		} else {
			buf = u.storageAccessor.Read(pid, addr, 2)
		}

		result[0] = buf[0]
		result[1] = buf[1]
		result[2] = 0
		result[3] = 0
		state.WriteOperandBytes(inst.Dst, i, result[:])
	}
}

func (u *ALU) runFlatLoadDWord(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		var buf []byte
		if u.flatIsLDS(addr) {
			buf = u.flatReadLDS(addr, 4)
		} else {
			buf = u.storageAccessor.Read(pid, addr, 4)
		}
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALU) runFlatLoadDWordX2(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		var buf []byte
		if u.flatIsLDS(addr) {
			buf = u.flatReadLDS(addr, 8)
		} else {
			buf = u.storageAccessor.Read(pid, addr, 8)
		}
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALU) runFlatLoadDWordX3(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		var buf []byte
		if u.flatIsLDS(addr) {
			buf = u.flatReadLDS(addr, 12)
		} else {
			buf = u.storageAccessor.Read(pid, addr, 12)
		}
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALU) runFlatLoadDWordX4(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		var buf []byte
		if u.flatIsLDS(addr) {
			buf = u.flatReadLDS(addr, 16)
		} else {
			buf = u.storageAccessor.Read(pid, addr, 16)
		}
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALU) runFlatStoreDWord(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 4)
		if u.flatIsLDS(addr) {
			u.flatWriteLDS(addr, data)
		} else {
			u.storageAccessor.Write(pid, addr, data)
		}
	}
}

func (u *ALU) runFlatStoreDWordX2(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()

	if inst.FlatSeg == 1 { // SCRATCH
		for i := 0; i < 64; i++ {
			if exec&(1<<uint(i)) == 0 {
				continue
			}
			off := u.scratchOffset(state, i)
			data := state.ReadOperandBytes(inst.Data, i, 8)
			u.scratchWrite(off, data)
		}
		return
	}

	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 8)
		if u.flatIsLDS(addr) {
			u.flatWriteLDS(addr, data)
		} else {
			u.storageAccessor.Write(pid, addr, data)
		}
	}
}

func (u *ALU) runFlatStoreDWordX3(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 12)
		if u.flatIsLDS(addr) {
			u.flatWriteLDS(addr, data)
		} else {
			u.storageAccessor.Write(pid, addr, data)
		}
	}
}

func (u *ALU) runFlatStoreDWordX4(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 16)
		if u.flatIsLDS(addr) {
			u.flatWriteLDS(addr, data)
		} else {
			u.storageAccessor.Write(pid, addr, data)
		}
	}
}

func (u *ALU) runFlatStoreByte(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 4)
		if u.flatIsLDS(addr) {
			u.flatWriteLDS(addr, data[:1])
		} else {
			u.storageAccessor.Write(pid, addr, data[:1])
		}
	}
}

func (u *ALU) runFlatStoreShort(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 4)
		if u.flatIsLDS(addr) {
			u.flatWriteLDS(addr, data[:2])
		} else {
			u.storageAccessor.Write(pid, addr, data[:2])
		}
	}
}

func (u *ALU) runFlatAtomicSwap(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		isLDS := u.flatIsLDS(addr)
		var old []byte
		if isLDS {
			old = u.flatReadLDS(addr, 4)
		} else {
			old = u.storageAccessor.Read(pid, addr, 4)
		}
		data := state.ReadOperandBytes(inst.Data, i, 4)
		if isLDS {
			u.flatWriteLDS(addr, data)
		} else {
			u.storageAccessor.Write(pid, addr, data)
		}
		if inst.GlobalLevelCoherent {
			state.WriteOperandBytes(inst.Dst, i, old)
		}
	}
}

func (u *ALU) runFlatAtomicCmpSwap(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		isLDS := u.flatIsLDS(addr)
		data := state.ReadOperandBytes(inst.Data, i, 8)
		newVal := data[0:4]
		cmpVal := binary.LittleEndian.Uint32(data[4:8])
		var old []byte
		if isLDS {
			old = u.flatReadLDS(addr, 4)
		} else {
			old = u.storageAccessor.Read(pid, addr, 4)
		}
		oldVal := binary.LittleEndian.Uint32(old)
		if oldVal == cmpVal {
			if isLDS {
				u.flatWriteLDS(addr, newVal)
			} else {
				u.storageAccessor.Write(pid, addr, newVal)
			}
		}
		if inst.GlobalLevelCoherent {
			state.WriteOperandBytes(inst.Dst, i, old)
		}
	}
}

func (u *ALU) runFlatAtomicAdd(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		isLDS := u.flatIsLDS(addr)
		var old []byte
		if isLDS {
			old = u.flatReadLDS(addr, 4)
		} else {
			old = u.storageAccessor.Read(pid, addr, 4)
		}
		oldVal := binary.LittleEndian.Uint32(old)
		data := state.ReadOperandBytes(inst.Data, i, 4)
		addVal := binary.LittleEndian.Uint32(data)
		newVal := oldVal + addVal
		var result [4]byte
		binary.LittleEndian.PutUint32(result[:], newVal)
		if isLDS {
			u.flatWriteLDS(addr, result[:])
		} else {
			u.storageAccessor.Write(pid, addr, result[:])
		}
		if inst.GlobalLevelCoherent {
			state.WriteOperandBytes(inst.Dst, i, old)
		}
	}
}

func (u *ALU) runFlatAtomicSub(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		isLDS := u.flatIsLDS(addr)
		var old []byte
		if isLDS {
			old = u.flatReadLDS(addr, 4)
		} else {
			old = u.storageAccessor.Read(pid, addr, 4)
		}
		oldVal := binary.LittleEndian.Uint32(old)
		data := state.ReadOperandBytes(inst.Data, i, 4)
		subVal := binary.LittleEndian.Uint32(data)
		newVal := oldVal - subVal
		var result [4]byte
		binary.LittleEndian.PutUint32(result[:], newVal)
		if isLDS {
			u.flatWriteLDS(addr, result[:])
		} else {
			u.storageAccessor.Write(pid, addr, result[:])
		}
		if inst.GlobalLevelCoherent {
			state.WriteOperandBytes(inst.Dst, i, old)
		}
	}
}

// runFlatAtomicSwapX2 implements 64-bit atomic swap (opcode 96)
func (u *ALU) runFlatAtomicSwapX2(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		isLDS := u.flatIsLDS(addr)
		var old []byte
		if isLDS {
			old = u.flatReadLDS(addr, 8)
		} else {
			old = u.storageAccessor.Read(pid, addr, 8)
		}
		data := state.ReadOperandBytes(inst.Data, i, 8)
		if isLDS {
			u.flatWriteLDS(addr, data)
		} else {
			u.storageAccessor.Write(pid, addr, data)
		}
		if inst.GlobalLevelCoherent {
			state.WriteOperandBytes(inst.Dst, i, old)
		}
	}
}

// runFlatAtomicCmpSwapX2 implements 64-bit atomic compare-and-swap (opcode 97)
func (u *ALU) runFlatAtomicCmpSwapX2(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		isLDS := u.flatIsLDS(addr)
		data := state.ReadOperandBytes(inst.Data, i, 16)
		newVal := data[0:8]
		cmpVal := binary.LittleEndian.Uint64(data[8:16])
		var old []byte
		if isLDS {
			old = u.flatReadLDS(addr, 8)
		} else {
			old = u.storageAccessor.Read(pid, addr, 8)
		}
		oldVal := binary.LittleEndian.Uint64(old)
		if oldVal == cmpVal {
			if isLDS {
				u.flatWriteLDS(addr, newVal)
			} else {
				u.storageAccessor.Write(pid, addr, newVal)
			}
		}
		if inst.GlobalLevelCoherent {
			state.WriteOperandBytes(inst.Dst, i, old)
		}
	}
}

// runFlatAtomicAddX2 implements 64-bit atomic add (opcode 98)
func (u *ALU) runFlatAtomicAddX2(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		isLDS := u.flatIsLDS(addr)
		var old []byte
		if isLDS {
			old = u.flatReadLDS(addr, 8)
		} else {
			old = u.storageAccessor.Read(pid, addr, 8)
		}
		oldVal := binary.LittleEndian.Uint64(old)
		data := state.ReadOperandBytes(inst.Data, i, 8)
		addVal := binary.LittleEndian.Uint64(data)
		newVal := oldVal + addVal
		var result [8]byte
		binary.LittleEndian.PutUint64(result[:], newVal)
		if isLDS {
			u.flatWriteLDS(addr, result[:])
		} else {
			u.storageAccessor.Write(pid, addr, result[:])
		}
		if inst.GlobalLevelCoherent {
			state.WriteOperandBytes(inst.Dst, i, old)
		}
	}
}

// runFlatAtomicSubX2 implements 64-bit atomic subtract (opcode 99)
func (u *ALU) runFlatAtomicSubX2(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		isLDS := u.flatIsLDS(addr)
		var old []byte
		if isLDS {
			old = u.flatReadLDS(addr, 8)
		} else {
			old = u.storageAccessor.Read(pid, addr, 8)
		}
		oldVal := binary.LittleEndian.Uint64(old)
		data := state.ReadOperandBytes(inst.Data, i, 8)
		subVal := binary.LittleEndian.Uint64(data)
		newVal := oldVal - subVal
		var result [8]byte
		binary.LittleEndian.PutUint64(result[:], newVal)
		if isLDS {
			u.flatWriteLDS(addr, result[:])
		} else {
			u.storageAccessor.Write(pid, addr, result[:])
		}
		if inst.GlobalLevelCoherent {
			state.WriteOperandBytes(inst.Dst, i, old)
		}
	}
}

// runScratch handles SCRATCH segment (SEG=1) load/store instructions.
// These access per-wavefront private (scratch) memory used for register
// spilling and local variables.
//
//nolint:gocyclo
func (u *ALU) runScratch(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()

	byteSize, isStore := u.scratchOpInfo(inst.Opcode)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		off := u.scratchOffset(state, i)

		if isStore {
			data := state.ReadOperandBytes(inst.Data, i, byteSize)
			u.scratchWrite(off, data)
		} else {
			data := u.scratchRead(off, byteSize)
			state.WriteOperandBytes(inst.Dst, i, data)
		}
	}
}

// scratchOpInfo returns the byte size and whether the opcode is a store.
func (u *ALU) scratchOpInfo(opcode insts.Opcode) (byteSize int, isStore bool) {
	switch opcode {
	case 0, 1, 16: // load_ubyte
		return 1, false
	case 2, 3, 17: // load_sbyte
		return 1, false
	case 4, 5, 18: // load_ushort
		return 2, false
	case 20: // load_dword
		return 4, false
	case 21: // load_dwordx2
		return 8, false
	case 22: // load_dwordx3
		return 12, false
	case 23: // load_dwordx4
		return 16, false
	case 24: // store_byte
		return 1, true
	case 26: // store_short
		return 2, true
	case 28: // store_dword
		return 4, true
	case 29: // store_dwordx2
		return 8, true
	case 30: // store_dwordx3
		return 12, true
	case 31: // store_dwordx4
		return 16, true
	default:
		log.Panicf("Opcode %d for SCRATCH segment is not implemented",
			opcode)
		return 0, false
	}
}
