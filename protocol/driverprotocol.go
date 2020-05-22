package protocol

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/util/ca"
)

// FlushCommand requests the GPU to flush all the cache to the main memory
type FlushCommand struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (m *FlushCommand) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewFlushCommand Creates a new flush command, setting the request send time
// with time and the source and destination.
func NewFlushCommand(time akita.VTimeInSec, src, dst akita.Port) *FlushCommand {
	cmd := new(FlushCommand)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// A LaunchKernelReq is a request that asks a GPU to launch a kernel
type LaunchKernelReq struct {
	akita.MsgMeta

	PID ca.PID

	Packet        *kernels.HsaKernelDispatchPacket
	PacketAddress uint64
	HsaCo         *insts.HsaCo
	WGFilter      kernels.WGFilterFunc
}

// Meta returns the meta data associated with the message.
func (m *LaunchKernelReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewLaunchKernelReq returns a new LaunchKernelReq
func NewLaunchKernelReq(
	time akita.VTimeInSec,
	src, dst akita.Port) *LaunchKernelReq {
	r := new(LaunchKernelReq)
	r.ID = akita.GetIDGenerator().Generate()
	r.SendTime = time
	r.Src = src
	r.Dst = dst
	return r
}

// A MemCopyH2DReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyH2DReq struct {
	akita.MsgMeta
	SrcBuffer  []byte
	DstAddress uint64
}

// Meta returns the meta data associated with the message.
func (m *MemCopyH2DReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewMemCopyH2DReq created a new MemCopyH2DReq
func NewMemCopyH2DReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
	srcBuffer []byte,
	dstAddress uint64,
) *MemCopyH2DReq {
	req := new(MemCopyH2DReq)
	req.ID = akita.GetIDGenerator().Generate()
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
	akita.MsgMeta
	SrcAddress uint64
	DstBuffer  []byte
}

// Meta returns the meta data associated with the message.
func (m *MemCopyD2HReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewMemCopyD2HReq created a new MemCopyH2DReq
func NewMemCopyD2HReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
	srcAddress uint64,
	dstBuffer []byte,
) *MemCopyD2HReq {
	req := new(MemCopyD2HReq)
	req.ID = akita.GetIDGenerator().Generate()
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
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec

	VAddr []uint64
	PID   ca.PID
}

// Meta returns the meta data associated with the message.
func (m *ShootDownCommand) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

//NewShootdownCommand tells the CP to drain all CU and invalidate PTE's in TLB and Page Tables
func NewShootdownCommand(
	time akita.VTimeInSec,
	src, dst akita.Port,
	vAddr []uint64,
	pID ca.PID,
) *ShootDownCommand {
	cmd := new(ShootDownCommand)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	cmd.VAddr = vAddr
	cmd.PID = pID
	return cmd
}

// ShootDownCompleteRsp defines a respond
type ShootDownCompleteRsp struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *ShootDownCompleteRsp) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewShootdownCompleteRsp creates a new respond
func NewShootdownCompleteRsp(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *ShootDownCompleteRsp {
	cmd := new(ShootDownCompleteRsp)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//RDMADrainCmdFromDriver is driver asking CP to drain local RDMA
type RDMADrainCmdFromDriver struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMADrainCmdFromDriver) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewRDMADrainCmdFromDriver creates a new RDMADrainCmdFromDriver
func NewRDMADrainCmdFromDriver(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *RDMADrainCmdFromDriver {
	cmd := new(RDMADrainCmdFromDriver)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//RDMADrainRspToDriver is  a rsp to driver indicating completion of RDMA drain
type RDMADrainRspToDriver struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMADrainRspToDriver) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

//NewRDMADrainRspToDriver creates a new RDMADrainRspToDriver
func NewRDMADrainRspToDriver(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *RDMADrainRspToDriver {
	cmd := new(RDMADrainRspToDriver)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//RDMARestartCmdFromDriver is  a cmd to unpause the RDMA
type RDMARestartCmdFromDriver struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMARestartCmdFromDriver) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewRDMARestartCmdFromDriver creates a new RDMARestartCmdFromDriver
func NewRDMARestartCmdFromDriver(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *RDMARestartCmdFromDriver {
	cmd := new(RDMARestartCmdFromDriver)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//GPURestartReq is  a req to GPU to start the pipeline and unpause all paused components
type GPURestartReq struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *GPURestartReq) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewGPURestartReq creates a GPURestart request
func NewGPURestartReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *GPURestartReq {
	cmd := new(GPURestartReq)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//GPURestartRsp is  a rsp indicating the restart is complete
type GPURestartRsp struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *GPURestartRsp) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewGPURestartRsp creates a GPURestart respond
func NewGPURestartRsp(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *GPURestartRsp {
	cmd := new(GPURestartRsp)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//PageMigrationReqToCP is a request to CP to start the page migration process
type PageMigrationReqToCP struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec

	ToReadFromPhysicalAddress uint64
	ToWriteToPhysicalAddress  uint64
	DestinationPMCPort        akita.Port
	PageSize                  uint64
}

// Meta returns the meta data associated with the message.
func (m *PageMigrationReqToCP) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewPageMigrationReqToCP creates a PageMigrationReqToCP
func NewPageMigrationReqToCP(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *PageMigrationReqToCP {
	cmd := new(PageMigrationReqToCP)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

//PageMigrationRspToDriver is a rsp to driver indicating completion of Page Migration requests
type PageMigrationRspToDriver struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *PageMigrationRspToDriver) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewPageMigrationRspToDriver creates a PageMigrationRspToCP
func NewPageMigrationRspToDriver(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *PageMigrationRspToDriver {
	cmd := new(PageMigrationRspToDriver)
	cmd.ID = akita.GetIDGenerator().Generate()
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}

// RDMARestartRspToDriver defines a respond
type RDMARestartRspToDriver struct {
	akita.MsgMeta

	StartTime akita.VTimeInSec
	EndTime   akita.VTimeInSec
}

// Meta returns the meta data associated with the message.
func (m *RDMARestartRspToDriver) Meta() *akita.MsgMeta {
	return &m.MsgMeta
}

// NewRDMARestartRspToDriver creates a RDMARestartRspToDriver
func NewRDMARestartRspToDriver(
	time akita.VTimeInSec,
	src, dst akita.Port,
) *RDMARestartRspToDriver {
	cmd := new(RDMARestartRspToDriver)
	cmd.SendTime = time
	cmd.Src = src
	cmd.Dst = dst
	return cmd
}
