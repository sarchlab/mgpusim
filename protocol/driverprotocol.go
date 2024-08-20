package protocol

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/insts"
	"github.com/sarchlab/mgpusim/v4/kernels"
)

// FlushReq requests the GPU to flush all the cache to the main memory
type FlushReq struct {
	sim.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *FlushReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the FlushReq with diffrent ID.
func (m *FlushReq) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewFlushReq Creates a new flush command, setting the request send time
// with time and the source and destination.
func NewFlushReq(src, dst sim.Port) *FlushReq {
	cmd := new(FlushReq)
	cmd.ID = sim.GetIDGenerator().Generate()
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

// Clone returns a clone of the LaunchKernelReq with diffrent ID.
func (m *LaunchKernelReq) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewLaunchKernelReq returns a new LaunchKernelReq
func NewLaunchKernelReq(
	src, dst sim.Port,
) *LaunchKernelReq {
	r := new(LaunchKernelReq)
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = src
	r.Dst = dst
	return r
}

// LaunchKernelRsp is the response that is send by the GPU to the driver when
// the kernel completes execution.
type LaunchKernelRsp struct {
	sim.MsgMeta

	RspTo string
}

// Meta returns the meta data associated with the message.
func (m *LaunchKernelRsp) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the LaunchKernelRsp with diffrent ID.
func (m *LaunchKernelRsp) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewLaunchKernelRsp returns a new LaunchKernelRsp.
func NewLaunchKernelRsp(
	src, dst sim.Port,
	rspTo string,
) *LaunchKernelRsp {
	r := new(LaunchKernelRsp)
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = src
	r.Dst = dst

	r.RspTo = rspTo

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

// Clone returns a clone of the MemCopyH2DReq with diffrent ID.
func (m *MemCopyH2DReq) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewMemCopyH2DReq created a new MemCopyH2DReq
func NewMemCopyH2DReq(
	src, dst sim.Port,
	srcBuffer []byte,
	dstAddress uint64,
) *MemCopyH2DReq {
	req := new(MemCopyH2DReq)
	req.ID = sim.GetIDGenerator().Generate()
	req.MsgMeta.TrafficBytes = len(srcBuffer)
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

// Clone returns a clone of the MemCopyD2HReq with diffrent ID.
func (m *MemCopyD2HReq) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewMemCopyD2HReq created a new MemCopyD2HReq
func NewMemCopyD2HReq(
	src, dst sim.Port,
	srcAddress uint64,
	dstBuffer []byte,
) *MemCopyD2HReq {
	req := new(MemCopyD2HReq)
	req.ID = sim.GetIDGenerator().Generate()
	req.MsgMeta.TrafficBytes = len(dstBuffer)
	req.Src = src
	req.Dst = dst
	req.SrcAddress = srcAddress
	req.DstBuffer = dstBuffer
	return req
}

// ShootDownCommand requests the GPU to perform a TLB shootdown and invalidate
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

// Clone returns a clone of the ShootDownCommand with diffrent ID.
func (m *ShootDownCommand) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewShootdownCommand tells the CP to drain all CU and invalidate PTE's in TLB and Page Tables
func NewShootdownCommand(
	src, dst sim.Port,
	vAddr []uint64,
	pID vm.PID,
) *ShootDownCommand {
	cmd := new(ShootDownCommand)
	cmd.ID = sim.GetIDGenerator().Generate()
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

// Clone returns a clone of the ShootDownCompleteRsp with diffrent ID.
func (m *ShootDownCompleteRsp) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewShootdownCompleteRsp creates a new respond
func NewShootdownCompleteRsp(
	src, dst sim.Port,
) *ShootDownCompleteRsp {
	cmd := new(ShootDownCompleteRsp)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// RDMADrainCmdFromDriver is driver asking CP to drain local RDMA
type RDMADrainCmdFromDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMADrainCmdFromDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the RDMADrainCmdFromDriver with diffrent ID.
func (m *RDMADrainCmdFromDriver) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewRDMADrainCmdFromDriver creates a new RDMADrainCmdFromDriver
func NewRDMADrainCmdFromDriver(
	src, dst sim.Port,
) *RDMADrainCmdFromDriver {
	cmd := new(RDMADrainCmdFromDriver)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// RDMADrainRspToDriver is  a rsp to driver indicating completion of RDMA drain
type RDMADrainRspToDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMADrainRspToDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the RDMADrainRspToDriver with diffrent ID.
func (m *RDMADrainRspToDriver) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewRDMADrainRspToDriver creates a new RDMADrainRspToDriver
func NewRDMADrainRspToDriver(
	src, dst sim.Port,
) *RDMADrainRspToDriver {
	cmd := new(RDMADrainRspToDriver)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// RDMARestartCmdFromDriver is  a cmd to unpause the RDMA
type RDMARestartCmdFromDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMARestartCmdFromDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the RDMARestartCmdFromDriver with diffrent ID.
func (m *RDMARestartCmdFromDriver) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewRDMARestartCmdFromDriver creates a new RDMARestartCmdFromDriver
func NewRDMARestartCmdFromDriver(
	src, dst sim.Port,
) *RDMARestartCmdFromDriver {
	cmd := new(RDMARestartCmdFromDriver)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// GPURestartReq is  a req to GPU to start the pipeline and unpause all paused components
type GPURestartReq struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *GPURestartReq) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the GPURestartReq with diffrent ID.
func (m *GPURestartReq) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewGPURestartReq creates a GPURestart request
func NewGPURestartReq(
	src, dst sim.Port,
) *GPURestartReq {
	cmd := new(GPURestartReq)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// GPURestartRsp is  a rsp indicating the restart is complete
type GPURestartRsp struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *GPURestartRsp) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the GPURestartRsp with diffrent ID.
func (m *GPURestartRsp) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewGPURestartRsp creates a GPURestart respond
func NewGPURestartRsp(
	src, dst sim.Port,
) *GPURestartRsp {
	cmd := new(GPURestartRsp)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// PageMigrationReqToCP is a request to CP to start the page migration process
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

// Clone returns a clone of the PageMigrationReqToCP with diffrent ID.
func (m *PageMigrationReqToCP) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewPageMigrationReqToCP creates a PageMigrationReqToCP
func NewPageMigrationReqToCP(
	src, dst sim.Port,
) *PageMigrationReqToCP {
	cmd := new(PageMigrationReqToCP)
	cmd.ID = sim.GetIDGenerator().Generate()
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// PageMigrationRspToDriver is a rsp to driver indicating completion of Page Migration requests
type PageMigrationRspToDriver struct {
	sim.MsgMeta

	StartTime sim.VTimeInSec
	EndTime   sim.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *PageMigrationRspToDriver) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the PageMigrationRspToDriver with diffrent ID.
func (m *PageMigrationRspToDriver) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewPageMigrationRspToDriver creates a PageMigrationRspToCP
func NewPageMigrationRspToDriver(
	src, dst sim.Port,
) *PageMigrationRspToDriver {
	cmd := new(PageMigrationRspToDriver)
	cmd.ID = sim.GetIDGenerator().Generate()
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

// Clone returns a clone of the RDMARestartRspToDriver with diffrent ID.
func (m *RDMARestartRspToDriver) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// NewRDMARestartRspToDriver creates a RDMARestartRspToDriver
func NewRDMARestartRspToDriver(
	src, dst sim.Port,
) *RDMARestartRspToDriver {
	cmd := new(RDMARestartRspToDriver)
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}
