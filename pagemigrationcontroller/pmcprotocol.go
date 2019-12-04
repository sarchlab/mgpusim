package pagemigrationcontroller

import (
	"gitlab.com/akita/akita"
)

// A PageMigrationReqToPMC asks the local GPU PMC to transfer a given page from another GPU PMC
type PageMigrationReqToPMC struct {
	akita.MsgMeta
	ToReadFromPhysicalAddress uint64
	ToWriteToPhysicalAddress  uint64
	PMCPortOfRemoteGPU        akita.Port
	PageSize                  uint64
}

// Meta returns the meta data associated with the message.
func (r *PageMigrationReqToPMC) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// PageMigrationReqToPMCBuilder can build new PMC mgiration requests
type PageMigrationReqToPMCBuilder struct {
	sendTime             akita.VTimeInSec
	src, dst             akita.Port
	ToReadFromPhyAddress uint64
	ToWriteToPhyAddress  uint64
	PMCPortOfRemoteGPU   akita.Port
	PageSize             uint64
}

// WithSendTime sets the send time of the request to build.:w
func (b PageMigrationReqToPMCBuilder) WithSendTime(
	t akita.VTimeInSec,
) PageMigrationReqToPMCBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b PageMigrationReqToPMCBuilder) WithSrc(src akita.Port) PageMigrationReqToPMCBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b PageMigrationReqToPMCBuilder) WithDst(dst akita.Port) PageMigrationReqToPMCBuilder {
	b.dst = dst
	return b
}

// WithReadFrom sets the read address of the request to build.
func (b PageMigrationReqToPMCBuilder) WithReadFrom(toReadFromPhyAddress uint64) PageMigrationReqToPMCBuilder {
	b.ToReadFromPhyAddress = toReadFromPhyAddress
	return b
}

// WithWriteTo sets the write address of the request to build.
func (b PageMigrationReqToPMCBuilder) WithWriteTo(toWriteToPhyAddress uint64) PageMigrationReqToPMCBuilder {
	b.ToWriteToPhyAddress = toWriteToPhyAddress
	return b
}

// WithPageSize sets the page size.
func (b PageMigrationReqToPMCBuilder) WithPageSize(pageSize uint64) PageMigrationReqToPMCBuilder {
	b.PageSize = pageSize
	return b
}

// WithPageSize sets the page size.
func (b PageMigrationReqToPMCBuilder) WithPMCPortOfRemoteGPU(
	pmcPortOfRemoteGPU akita.Port,
) PageMigrationReqToPMCBuilder {
	b.PMCPortOfRemoteGPU = pmcPortOfRemoteGPU
	return b
}

// Build creats a new PageMigrationReqToPMC
func (b PageMigrationReqToPMCBuilder) Build() *PageMigrationReqToPMC {
	r := &PageMigrationReqToPMC{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.ToReadFromPhysicalAddress = b.ToReadFromPhyAddress
	r.ToWriteToPhysicalAddress = b.ToWriteToPhyAddress
	r.PageSize = b.PageSize
	r.PMCPortOfRemoteGPU = b.PMCPortOfRemoteGPU
	return r
}

// A PageMigrationRspFromPMC notifies the PMC controlling device of page transfer completion
type PageMigrationRspFromPMC struct {
	akita.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *PageMigrationRspFromPMC) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// PageMigrationRspFromPMCBuilder can build new PMC migration responses
type PageMigrationRspFromPMCBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
}

// WithSendTime sets the send time of the request to build.:w
func (b PageMigrationRspFromPMCBuilder) WithSendTime(
	t akita.VTimeInSec,
) PageMigrationRspFromPMCBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b PageMigrationRspFromPMCBuilder) WithSrc(src akita.Port) PageMigrationRspFromPMCBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b PageMigrationRspFromPMCBuilder) WithDst(dst akita.Port) PageMigrationRspFromPMCBuilder {
	b.dst = dst
	return b
}

// Build creats a new PageMigrationReqToPMC
func (b PageMigrationRspFromPMCBuilder) Build() *PageMigrationRspFromPMC {
	r := &PageMigrationRspFromPMC{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime

	return r
}

// A DataPullReq asks remote PMC that holds the page for a page
type DataPullReq struct {
	akita.MsgMeta
	ToReadFromPhyAddress uint64
	DataTransferSize     uint64
}

// Meta returns the meta data associated with the message.
func (r *DataPullReq) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// DataPullReqBuilder can build new Data pull reqs
type DataPullReqBuilder struct {
	sendTime             akita.VTimeInSec
	src, dst             akita.Port
	ToReadFromPhyAddress uint64
	DataTransferSize     uint64
}

// WithSendTime sets the send time of the request to build.:w
func (b DataPullReqBuilder) WithSendTime(
	t akita.VTimeInSec,
) DataPullReqBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b DataPullReqBuilder) WithSrc(src akita.Port) DataPullReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DataPullReqBuilder) WithDst(dst akita.Port) DataPullReqBuilder {
	b.dst = dst
	return b
}

// WithReadFromPhyAddress sets the read address of the request to build.
func (b DataPullReqBuilder) WithReadFromPhyAddress(toReadFromPhyAddress uint64) DataPullReqBuilder {
	b.ToReadFromPhyAddress = toReadFromPhyAddress
	return b
}

// WithDataTransferSize sets the data transfer size of the request to build.
func (b DataPullReqBuilder) WithDataTransferSize(dataTransferSize uint64) DataPullReqBuilder {
	b.DataTransferSize = dataTransferSize
	return b
}

// Build creats a new DataPullReq
func (b DataPullReqBuilder) Build() *DataPullReq {
	r := &DataPullReq{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.ToReadFromPhyAddress = b.ToReadFromPhyAddress
	r.DataTransferSize = b.DataTransferSize
	r.TrafficBytes = 12

	return r
}

// A DataPullRsp returns requested data
type DataPullRsp struct {
	akita.MsgMeta
	Data []byte
}

// Meta returns the meta data associated with the message.
func (r *DataPullRsp) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// DataPullRspBuilder can build new Data pull rsps
type DataPullRspBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
	Data     []byte
}

// WithSendTime sets the send time of the request to build
func (b DataPullRspBuilder) WithSendTime(
	t akita.VTimeInSec,
) DataPullRspBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b DataPullRspBuilder) WithSrc(src akita.Port) DataPullRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DataPullRspBuilder) WithDst(dst akita.Port) DataPullRspBuilder {
	b.dst = dst
	return b
}

// WithData sets the data to build
func (b DataPullRspBuilder) WithData(Data []byte) DataPullRspBuilder {
	b.Data = Data
	return b
}

// Build creats a new DataPullRsp
func (b DataPullRspBuilder) Build() *DataPullRsp {
	r := &DataPullRsp{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.Data = b.Data
	r.TrafficBytes = len(r.Data) + 12

	return r
}
