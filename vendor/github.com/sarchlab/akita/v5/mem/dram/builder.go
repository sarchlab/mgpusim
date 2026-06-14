package dram

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

var defaultSpec = Spec{
	Freq:                 1600 * timing.MHz,
	Protocol:             int(protoDDR3),
	TAL:                  0,
	TCL:                  11,
	TCWL:                 8,
	TRCD:                 11,
	TRP:                  11,
	TRAS:                 28,
	TCCDL:                4,
	TCCDS:                4,
	TRTRS:                1,
	TRTP:                 6,
	TWTRL:                6,
	TWTRS:                6,
	TWR:                  12,
	TPPD:                 0,
	TRRDL:                5,
	TRRDS:                5,
	TRCDRD:               24,
	TRCDWR:               20,
	TREFI:                6240,
	TRFC:                 208,
	TRFCb:                1950,
	TCKESR:               5,
	TXS:                  216,
	BusWidth:             64,
	BurstLength:          8,
	DeviceWidth:          16,
	NumChannel:           1,
	NumRank:              2,
	NumBankGroup:         1,
	NumBank:              8,
	NumRow:               32768,
	NumCol:               1024,
	TransactionQueueSize: 32,
	CommandQueueCapacity: 8,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder can build new memory controllers. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithRegistrar and
// WithResources. The component declares its "Top" and "Control" ports; the
// port instances are supplied externally after Build with AssignPort (the
// caller chooses the buffer sizes).
type Builder struct {
	registrar modeling.Registrar
	spec      Spec
	resources Resources

	tracers []tracing.Tracer
}

// MakeBuilder creates a builder with default configuration.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithResources injects the component's shared resources (a storage shared
// with other components). If not set, the controller builds its own, sized
// from the geometry spec.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build builds a new MemController. It declares the component's "Top" and
// "Control" ports; assign the port instances after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("dram: WithRegistrar is required")
	}

	b.normalizeSpec()

	spec := b.buildSpec()
	timing := b.generateTiming()
	cmdCycles := b.buildCmdCycles()

	initialState := State{
		SubTransQueue: subTransQueueState{
			Entries: []subTransRef{},
		},
		CommandQueues: commandQueueState{
			NumQueues: b.spec.NumChannel * b.spec.NumRank,
			Entries:   []queueEntry{},
		},
		BankStates: initBankStatesFlat(
			b.spec.NumRank, b.spec.NumBankGroup, b.spec.NumBank),
	}

	storage := b.resolveStorage(name)

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(Resources{Storage: storage}).
		Build(name)
	modelComp.State = initialState

	modelComp.DeclarePort("Top", memprotocol.Responder)
	modelComp.DeclarePort("Control", memcontrolprotocol.Responder)

	b.addMiddlewares(modelComp, timing, cmdCycles)

	for _, tracer := range b.tracers {
		tracing.CollectTrace(modelComp, tracer)
	}

	b.registrar.RegisterComponent(modelComp)

	return modelComp
}

// normalizeSpec computes the derived timing fields from the configured spec.
func (b *Builder) normalizeSpec() {
	b.calculateBurstCycle()
	b.spec.TRL = b.spec.TAL + b.spec.TCL
	b.spec.TWL = b.spec.TAL + b.spec.TCWL
	b.spec.ReadDelay = b.spec.TRL + b.spec.BurstCycle
	b.spec.WriteDelay = b.spec.TRL + b.spec.BurstCycle
	b.spec.TRC = b.spec.TRAS + b.spec.TRP
}

func (b Builder) resolveStorage(name string) *mem.Storage {
	if b.resources.Storage != nil {
		return b.resources.Storage
	}

	devicePerRank := b.spec.BusWidth / b.spec.DeviceWidth
	bankSize := b.spec.NumCol * b.spec.NumRow * b.spec.DeviceWidth / 8
	rankSize := bankSize * b.spec.NumBank * devicePerRank
	totalSize := rankSize * b.spec.NumRank * b.spec.NumChannel

	return mem.MakeStorageBuilder().
		WithCapacity(uint64(totalSize)).
		WithSimulation(b.registrar).
		Build(name + ".Storage")
}

func (b Builder) addMiddlewares(
	modelComp *modeling.Component[Spec, State, Resources],
	timing dramTiming, cmdCycles map[commandKind]int,
) {
	cMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cMW)

	rMW := &respondMW{
		comp: modelComp,
	}
	modelComp.AddMiddleware(rMW)

	btMW := &bankTickMW{
		comp:      modelComp,
		timing:    timing,
		cmdCycles: cmdCycles,
	}
	modelComp.AddMiddleware(btMW)

	ptMW := &parseTopMW{
		comp: modelComp,
	}
	modelComp.AddMiddleware(ptMW)
}

func (b Builder) buildSpec() Spec {
	numAccessUnitBit, _ := log2(uint64(b.spec.BusWidth / 8 * b.spec.BurstLength))
	addrMapping := b.buildAddressMapping()

	spec := b.spec

	b.applyAddrMapping(&spec, addrMapping)

	spec.Log2AccessUnitSize = numAccessUnitBit

	return spec
}

func (b Builder) buildCmdCycles() map[commandKind]int {
	proto := protocol(b.spec.Protocol)

	cmdCycles := map[commandKind]int{
		cmdKindRead:           b.spec.ReadDelay,
		cmdKindReadPrecharge:  b.spec.TRP,
		cmdKindWrite:          b.spec.WriteDelay,
		cmdKindWritePrecharge: b.spec.TRP,
		cmdKindActivate:       b.spec.TRCD - b.spec.TAL,
		cmdKindPrecharge:      b.spec.TRP,
		cmdKindRefreshBank:    1,
		cmdKindRefresh:        1,
		cmdKindSRefEnter:      1,
		cmdKindSRefExit:       1,
	}

	if proto.isGDDR() || proto.isHBM() {
		cmdCycles[cmdKindActivate] = b.spec.TRCDRD - b.spec.TAL
	}

	return cmdCycles
}

func (b Builder) applyAddrMapping(spec *Spec, m addrMappingResult) {
	spec.ChannelPos = m.channelPos
	spec.ChannelMask = m.channelMask
	spec.RankPos = m.rankPos
	spec.RankMask = m.rankMask
	spec.BankGroupPos = m.bankGroupPos
	spec.BankGroupMask = m.bankGroupMask
	spec.BankPos = m.bankPos
	spec.BankMask = m.bankMask
	spec.RowPos = m.rowPos
	spec.RowMask = m.rowMask
	spec.ColPos = m.colPos
	spec.ColMask = m.colMask
}

type addrMappingResult struct {
	channelPos    int
	channelMask   uint64
	rankPos       int
	rankMask      uint64
	bankGroupPos  int
	bankGroupMask uint64
	bankPos       int
	bankMask      uint64
	rowPos        int
	rowMask       uint64
	colPos        int
	colMask       uint64
}

func (b Builder) buildAddressMapping() addrMappingResult {
	channelBit, _ := log2(uint64(b.spec.NumChannel))
	rankBit, _ := log2(uint64(b.spec.NumRank))
	bankGroupBit, _ := log2(uint64(b.spec.NumBankGroup))
	bankBit, _ := log2(uint64(b.spec.NumBank))
	rowBit, _ := log2(uint64(b.spec.NumRow))
	colBit, _ := log2(uint64(b.spec.NumCol))
	colLoBit, _ := log2(uint64(b.spec.BurstLength))
	colHiBit := colBit - colLoBit
	accessUnitBit, _ := log2(uint64(b.spec.BusWidth / 8 * b.spec.BurstLength))

	r := addrMappingResult{
		channelMask:   (1 << channelBit) - 1,
		rankMask:      (1 << rankBit) - 1,
		bankGroupMask: (1 << bankGroupBit) - 1,
		bankMask:      (1 << bankBit) - 1,
		rowMask:       (1 << rowBit) - 1,
		colMask:       (1 << colHiBit) - 1,
	}

	bitWidths := addrBitWidths{
		channel:   channelBit,
		rank:      rankBit,
		bankGroup: bankGroupBit,
		bank:      bankBit,
		row:       rowBit,
		colHi:     colHiBit,
	}

	r.assignBitPositions(accessUnitBit, bitWidths)

	return r
}

type addrBitWidths struct {
	channel, rank, bankGroup, bank, row, colHi uint64
}

func (r *addrMappingResult) assignBitPositions(
	startPos uint64, w addrBitWidths,
) {
	// Default bit order high→low: Row, Channel, Rank, Bank, BankGroup, Column
	type locItem int
	const (
		liChannel locItem = iota
		liRank
		liBankGroup
		liBank
		liRow
		liColumn
	)

	bitOrder := []locItem{
		liRow, liChannel, liRank, liBank, liBankGroup, liColumn,
	}

	pos := startPos
	for i := len(bitOrder) - 1; i >= 0; i-- {
		switch bitOrder[i] {
		case liChannel:
			r.channelPos = int(pos)
			pos += w.channel
		case liRank:
			r.rankPos = int(pos)
			pos += w.rank
		case liBankGroup:
			r.bankGroupPos = int(pos)
			pos += w.bankGroup
		case liBank:
			r.bankPos = int(pos)
			pos += w.bank
		case liRow:
			r.rowPos = int(pos)
			pos += w.row
		case liColumn:
			r.colPos = int(pos)
			pos += w.colHi
		}
	}
}

//nolint:gocyclo,funlen
func (b *Builder) generateTiming() dramTiming {
	s := &b.spec
	proto := protocol(s.Protocol)

	t := dramTiming{
		SameBank:              makeTimeTable(),
		OtherBanksInBankGroup: makeTimeTable(),
		SameRank:              makeTimeTable(),
		OtherRanks:            makeTimeTable(),
	}

	readToReadL := max(s.BurstCycle, s.TCCDL)
	readToReadS := max(s.BurstCycle, s.TCCDS)
	readToReadO := s.BurstCycle + s.TRTRS
	readToWrite := s.TRL + s.BurstCycle - s.TWL + s.TRTRS
	readToWriteO := s.ReadDelay + s.BurstCycle +
		s.TRTRS - s.WriteDelay
	readToPrecharge := s.TAL + s.TRTP
	readpToAct := s.TAL + s.BurstCycle + s.TRTP + s.TRP

	writeToReadL := s.WriteDelay + s.TWTRL
	writeToReadS := s.WriteDelay + s.TWTRS
	writeToReadO := s.WriteDelay + s.BurstCycle +
		s.TRTRS - s.ReadDelay
	writeToWriteL := max(s.BurstCycle, s.TCCDL)
	writeToWriteS := max(s.BurstCycle, s.TCCDS)
	writeToWriteO := s.BurstCycle
	writeToPrecharge := s.TWL + s.BurstCycle + s.TWR

	prechargeToActivate := s.TRP
	prechargeToPrecharge := s.TPPD
	readToActivate := readToPrecharge + prechargeToActivate
	writeToActivate := writeToPrecharge + prechargeToActivate

	activateToActivate := s.TRC
	activateToActivateL := s.TRRDL
	activateToActivateS := s.TRRDS
	activateToPrecharge := s.TRAS
	activateToRead := s.TRCD - s.TAL
	activateToWrite := s.TRCD - s.TAL

	if proto.isGDDR() || proto.isHBM() {
		activateToRead = s.TRCDRD
		activateToWrite = s.TRCDWR
	}

	activateToRefresh := s.TRC

	refreshToRefresh := s.TREFI
	refreshToActivate := s.TRFC
	refreshToActivateBank := s.TRFCb

	selfRefreshEntryToExit := s.TCKESR
	selfRefreshExit := s.TXS

	if s.NumBankGroup == 1 {
		readToReadL = max(s.BurstCycle, s.TCCDS)
		writeToReadL = s.WriteDelay + s.TWTRS
		writeToWriteL = max(s.BurstCycle, s.TCCDS)
		activateToActivateL = s.TRRDS
	}

	t.SameBank[cmdKindRead] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: readToPrecharge},
	}
	t.OtherBanksInBankGroup[cmdKindRead] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.SameRank[cmdKindRead] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadS},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadS},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.OtherRanks[cmdKindRead] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadO},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWriteO},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadO},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWriteO},
	}

	t.SameBank[cmdKindWrite] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: writeToPrecharge},
	}
	t.OtherBanksInBankGroup[cmdKindWrite] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
	}
	t.SameRank[cmdKindWrite] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadS},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteS},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadS},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteS},
	}
	t.OtherRanks[cmdKindWrite] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadO},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteO},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadO},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteO},
	}

	// READ_PRECHARGE
	t.SameBank[cmdKindReadPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: readpToAct},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: readToActivate},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: readToActivate},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: readToActivate},
	}
	t.OtherBanksInBankGroup[cmdKindReadPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.SameRank[cmdKindReadPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadS},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadS},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWrite},
	}
	t.OtherRanks[cmdKindReadPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: readToReadO},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: readToWriteO},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: readToReadO},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: readToWriteO},
	}

	// WRITE_PRECHARGE
	t.SameBank[cmdKindWritePrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: writeToActivate},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: writeToActivate},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: writeToActivate},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: writeToActivate},
	}
	t.OtherBanksInBankGroup[cmdKindWritePrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteL},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadL},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteL},
	}
	t.SameRank[cmdKindWritePrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadS},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteS},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadS},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteS},
	}
	t.OtherRanks[cmdKindWritePrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindRead, MinCycleInBetween: writeToReadO},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: writeToWriteO},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: writeToReadO},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: writeToWriteO},
	}

	// ACTIVATE
	t.SameBank[cmdKindActivate] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: activateToActivate},
		{NextCmdKind: cmdKindRead, MinCycleInBetween: activateToRead},
		{NextCmdKind: cmdKindWrite, MinCycleInBetween: activateToWrite},
		{NextCmdKind: cmdKindReadPrecharge, MinCycleInBetween: activateToRead},
		{NextCmdKind: cmdKindWritePrecharge, MinCycleInBetween: activateToWrite},
		{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: activateToPrecharge},
	}
	t.OtherBanksInBankGroup[cmdKindActivate] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: activateToActivateL},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: activateToRefresh},
	}
	t.SameRank[cmdKindActivate] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: activateToActivateS},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: activateToRefresh},
	}

	// PRECHARGE
	t.SameBank[cmdKindPrecharge] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: prechargeToActivate},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: prechargeToActivate},
	}

	if proto.isGDDR() || proto == protoLPDDR4 {
		t.OtherBanksInBankGroup[cmdKindPrecharge] = []timeTableEntry{
			{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: prechargeToPrecharge},
		}
		t.SameRank[cmdKindPrecharge] = []timeTableEntry{
			{NextCmdKind: cmdKindPrecharge, MinCycleInBetween: prechargeToPrecharge},
		}
	}

	// REFRESH_BANK
	t.SameRank[cmdKindRefreshBank] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: max(refreshToActivateBank, refreshToActivate)},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: refreshToActivateBank},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: max(refreshToActivateBank, refreshToRefresh)},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: refreshToActivateBank},
	}
	t.OtherBanksInBankGroup[cmdKindRefreshBank] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: refreshToRefresh},
	}

	// REFRESH
	t.SameRank[cmdKindRefresh] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: refreshToActivate},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: refreshToActivate},
	}

	// SREF_ENTER
	t.SameRank[cmdKindSRefEnter] = []timeTableEntry{
		{NextCmdKind: cmdKindSRefExit, MinCycleInBetween: selfRefreshEntryToExit},
	}

	// SREF_EXIT
	t.SameRank[cmdKindSRefExit] = []timeTableEntry{
		{NextCmdKind: cmdKindActivate, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: cmdKindRefresh, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: cmdKindRefreshBank, MinCycleInBetween: selfRefreshExit},
		{NextCmdKind: cmdKindSRefEnter, MinCycleInBetween: selfRefreshExit},
	}

	return t
}

func (b *Builder) calculateBurstCycle() {
	b.burstLengthMustNotBeZero()

	switch protocol(b.spec.Protocol) {
	case protoGDDR5:
		b.spec.BurstCycle = b.spec.BurstLength / 4
	case protoGDDR5X:
		b.spec.BurstCycle = b.spec.BurstLength / 8
	case protoGDDR6:
		b.spec.BurstCycle = b.spec.BurstLength / 16
	default:
		b.spec.BurstCycle = b.spec.BurstLength / 2
	}
}

func (b *Builder) burstLengthMustNotBeZero() {
	if b.spec.BurstLength == 0 {
		panic("burst length cannot be 0")
	}
}

// log2 returns the log2 of a number. It also returns false if it is not a log2
// number.
func log2(n uint64) (uint64, bool) {
	oneCount := 0
	onePos := uint64(0)

	for i := range uint64(64) {
		if n&(1<<i) > 0 {
			onePos = i
			oneCount++
		}
	}

	return onePos, oneCount == 1
}
