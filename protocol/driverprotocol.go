package protocol

import (
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mem/v3/vm"
	"gitlab.com/akita/mgpusim/v3/insts"
	"gitlab.com/akita/mgpusim/v3/kernels"
)

// FlushReq requests the GPU to flush all the cache to the main memory
type FlushReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *FlushReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewFlushReq Creates a new flush command, setting the request send time
// with time and the source and destination.
func NewFlushReq(time sim.VTimeInSec, src, dst sim.Port) *FlushReq {
	cmd := new(FlushReq)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// A LaunchKernelReq is a request that asks a GPU to launch a kernel
type LaunchKernelReq struct {
	sim.MsgMeta

	PID vm.PID

	Packet        *kernels.HsaKernelDispatchPacket
	PacketAddress uint64
	HsaCo         *insts.HsaCo
	WGFilter      kernels.WGFilterFunc
}

// Meta returns the meta data associated with the message.
func (m *LaunchKernelReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewLaunchKernelReq returns a new LaunchKernelReq
func NewLaunchKernelReq(
	time sim.VTimeInSec,
	src, dst sim.Port) *LaunchKernelReq {
	r := new(LaunchKernelReq)
	r.ID = sim.GetIDGenerator().Generate()
	r.SendTime = time
	r.Src = src
	r.Dst = dst
	return r
}

// A MemCopyH2DReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyH2DReq struct {
	sim.MsgMeta
	SrcBuffer  []byte
	DstAddress uint64
}

// Meta returns the meta data associated with the message.
func (m *MemCopyH2DReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewMemCopyH2DReq created a new MemCopyH2DReq
func NewMemCopyH2DReq(
	time sim.VTimeInSec,
	src, dst sim.Port,
	srcBuffer []byte,
	dstAddress uint64,
) *MemCopyH2DReq {
	req := new(MemCopyH2DReq)
	req.ID = sim.GetIDGenerator().Generate()
	req.SendTime = time
	req.Src = src
	req.Dst = dst
	req.SrcBuffer = srcBuffer
	req.DstAddress = dstAddress
	return req
}

// A MemCopyD2HReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyD2HReq struct {
	sim.MsgMeta
	SrcAddress uint64
	DstBuffer  []byte
}

// Meta returns the meta data associated with the message.
func (m *MemCopyD2HReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewMemCopyD2HReq created a new MemCopyH2DReq
func NewMemCopyD2HReq(
	time sim.VTimeInSec,
	src, dst sim.Port,
	srcAddress uint64,
	dstBuffer []byte,
) *MemCopyD2HReq {
	req := new(MemCopyD2HReq)
	req.ID = sim.GetIDGenerator().Generate()
	req.SendTime = time
	req.Src = src
	req.Dst = dst
	req.SrcAddress = srcAddress
	req.DstBuffer = dstBuffer
	return req
}

//ShootDownCommand requests the GPU to perform a TLB shootdown and invalidate
// the corresponding PTE's
type ShootDownCommand struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec

	VAddr []uint64
	PID   vm.PID
}

// Meta returns the meta data associated with the message.
func (m *ShootDownCommand) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

//NewShootdownCommand tells the CP to drain all CU and invalidate PTE's in TLB and Page Tables
func NewShootdownCommand(
	time sim.VTimeInSec,
	src, dst sim.Port,
	vAddr []uint64,
	pID vm.PID,
) *ShootDownCommand {
	cmd := new(ShootDownCommand)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	cmd.VAddr = vAddr
	cmd.PID = pID
	return cmd
}

// ShootDownCompleteRsp defines a respond
type ShootDownCompleteRsp struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *ShootDownCompleteRsp) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewShootdownCompleteRsp creates a new respond
func NewShootdownCompleteRsp(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *ShootDownCompleteRsp {
	cmd := new(ShootDownCompleteRsp)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//RDMADrainCmdFromDriver is driver asking CP to drain local RDMA
type RDMADrainCmdFromDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMADrainCmdFromDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewRDMADrainCmdFromDriver creates a new RDMADrainCmdFromDriver
func NewRDMADrainCmdFromDriver(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *RDMADrainCmdFromDriver {
	cmd := new(RDMADrainCmdFromDriver)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//RDMADrainRspToDriver is  a rsp to driver indicating completion of RDMA drain
type RDMADrainRspToDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMADrainRspToDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

//NewRDMADrainRspToDriver creates a new RDMADrainRspToDriver
func NewRDMADrainRspToDriver(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *RDMADrainRspToDriver {
	cmd := new(RDMADrainRspToDriver)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//RDMARestartCmdFromDriver is  a cmd to unpause the RDMA
type RDMARestartCmdFromDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMARestartCmdFromDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewRDMARestartCmdFromDriver creates a new RDMARestartCmdFromDriver
func NewRDMARestartCmdFromDriver(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *RDMARestartCmdFromDriver {
	cmd := new(RDMARestartCmdFromDriver)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//GPURestartReq is  a req to GPU to start the pipeline and unpause all paused components
type GPURestartReq struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *GPURestartReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewGPURestartReq creates a GPURestart request
func NewGPURestartReq(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *GPURestartReq {
	cmd := new(GPURestartReq)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//GPURestartRsp is  a rsp indicating the restart is complete
type GPURestartRsp struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *GPURestartRsp) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewGPURestartRsp creates a GPURestart respond
func NewGPURestartRsp(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *GPURestartRsp {
	cmd := new(GPURestartRsp)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//PageMigrationReqToCP is a request to CP to start the page migration process
type PageMigrationReqToCP struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec

	ToReadFromPhysicalAddress uint64
	ToWriteToPhysicalAddress  uint64
	DestinationPMCPort        sim.Port
	PageSize                  uint64
}

// Meta returns the meta data associated with the message.
func (m *PageMigrationReqToCP) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewPageMigrationReqToCP creates a PageMigrationReqToCP
func NewPageMigrationReqToCP(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *PageMigrationReqToCP {
	cmd := new(PageMigrationReqToCP)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//PageMigrationRspToDriver is a rsp to driver indicating completion of Page Migration requests
type PageMigrationRspToDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *PageMigrationRspToDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewPageMigrationRspToDriver creates a PageMigrationRspToCP
func NewPageMigrationRspToDriver(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *PageMigrationRspToDriver {
	cmd := new(PageMigrationRspToDriver)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// RDMARestartRspToDriver defines a respond
type RDMARestartRspToDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMARestartRspToDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewRDMARestartRspToDriver creates a RDMARestartRspToDriver
func NewRDMARestartRspToDriver(
	time sim.VTimeInSec,
	src, dst sim.Port,
) *RDMARestartRspToDriver {
	cmd := new(RDMARestartRspToDriver)
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}
