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
	sendTime akita.VTimeInSec
	src, dst akita.Port
	ToReadFromPhyAddress uint64
	ToWriteToPhyAddress  uint64
	PMCPortOfRemoteGPU            akita.Port
	PageSize                  uint64
}

// WithSendTime sets the send time of the request to build.:w
func (b PageMigrationReqToPMCBuilder) WithSendTime(
	t akita.VTimeInSec,
) PageMigrationReqToPMCBuilder  {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b PageMigrationReqToPMCBuilder ) WithSrc(src akita.Port) PageMigrationReqToPMCBuilder  {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b PageMigrationReqToPMCBuilder) WithDst(dst akita.Port) PageMigrationReqToPMCBuilder  {
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
	b.PageSize= pageSize
	return b
}


// WithPageSize sets the page size.
func (b PageMigrationReqToPMCBuilder) WithPMCPortOfRemoteGPU(pmcPortOfRemoteGPU akita.Port) PageMigrationReqToPMCBuilder {
	b.PMCPortOfRemoteGPU= pmcPortOfRemoteGPU
	return b
}




// Build creats a new PageMigrationReqToPMC
func (b PageMigrationReqToPMCBuilder ) Build() *PageMigrationReqToPMC{
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
) PageMigrationRspFromPMCBuilder  {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b PageMigrationRspFromPMCBuilder ) WithSrc(src akita.Port) PageMigrationRspFromPMCBuilder  {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b PageMigrationRspFromPMCBuilder) WithDst(dst akita.Port) PageMigrationRspFromPMCBuilder  {
	b.dst = dst
	return b
}

// Build creats a new PageMigrationReqToPMC
func (b PageMigrationRspFromPMCBuilder ) Build() *PageMigrationRspFromPMC{
	r := &PageMigrationRspFromPMC{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime

	return r
}





// A DataPullReqToRemotePMC asks remote PMC that holds the page for a page
type DataPullReqToRemotePMC struct {
	akita.MsgMeta
	ToReadFromPhyAddress  uint64
	DataTransferSize uint64
}

// Meta returns the meta data associated with the message.
func (r *DataPullReqToRemotePMC) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// DataPullReqToRemotePMCBuilder can build new Data pull reqs
type DataPullReqToRemotePMCBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
	ToReadFromPhyAddress  uint64
	DataTransferSize uint64
}

// WithSendTime sets the send time of the request to build.:w
func (b DataPullReqToRemotePMCBuilder) WithSendTime(
	t akita.VTimeInSec,
) DataPullReqToRemotePMCBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b DataPullReqToRemotePMCBuilder ) WithSrc(src akita.Port) DataPullReqToRemotePMCBuilder  {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DataPullReqToRemotePMCBuilder) WithDst(dst akita.Port) DataPullReqToRemotePMCBuilder  {
	b.dst = dst
	return b
}

// WithReadFromPhyAddress sets the read address of the request to build.
func (b DataPullReqToRemotePMCBuilder) WithReadFromPhyAddress(toReadFromPhyAddress uint64) DataPullReqToRemotePMCBuilder  {
	b.ToReadFromPhyAddress = toReadFromPhyAddress
	return b
}

// WithDataTransferSize sets the data transfer size of the request to build.
func (b DataPullReqToRemotePMCBuilder) WithDataTransferSize(dataTransferSize uint64) DataPullReqToRemotePMCBuilder {
	b.DataTransferSize = dataTransferSize
	return b
}




// Build creats a new DataPullReqToRemotePMC
func (b DataPullReqToRemotePMCBuilder ) Build() *DataPullReqToRemotePMC{
	r := &DataPullReqToRemotePMC{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.ToReadFromPhyAddress = b.ToReadFromPhyAddress
	r.DataTransferSize = b.DataTransferSize

	return r
}

// A DataPullRspFromRemotePMC returns requested data
type DataPullRspFromRemotePMC struct {
	akita.MsgMeta
	Data      []byte
}

// Meta returns the meta data associated with the message.
func (r *DataPullRspFromRemotePMC) Meta() *akita.MsgMeta {
	return &r.MsgMeta
}

// DataPullRspFromRemotePMCBuilder can build new Data pull rsps
type DataPullRspFromRemotePMCBuilder struct {
	sendTime akita.VTimeInSec
	src, dst akita.Port
	Data []byte
}

// WithSendTime sets the send time of the request to build
func (b DataPullRspFromRemotePMCBuilder) WithSendTime(
	t akita.VTimeInSec,
) DataPullRspFromRemotePMCBuilder {
	b.sendTime = t
	return b
}

// WithSrc sets the source of the request to build.
func (b DataPullRspFromRemotePMCBuilder ) WithSrc(src akita.Port) DataPullRspFromRemotePMCBuilder  {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b DataPullRspFromRemotePMCBuilder) WithDst(dst akita.Port) DataPullRspFromRemotePMCBuilder  {
	b.dst = dst
	return b
}

// WithData sets the data to build
func (b DataPullRspFromRemotePMCBuilder) WithData(Data []byte) DataPullRspFromRemotePMCBuilder  {
	b.Data = Data
	return b
}





// Build creats a new DataPullRspFromRemotePMC
func (b DataPullRspFromRemotePMCBuilder) Build() *DataPullRspFromRemotePMC{
	r := &DataPullRspFromRemotePMC{}
	r.ID = akita.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.SendTime = b.sendTime
	r.Data = b.Data


	return r
}
















