package gpubuilder

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/tlb"
	"gitlab.com/akita/mgpusim/timing/caches/l1v"
	"gitlab.com/akita/mgpusim/timing/caches/rob"
	"gitlab.com/akita/mgpusim/timing/cu"
	"gitlab.com/akita/util/tracing"
)

type shaderArray struct {
	cus []*cu.ComputeUnit

	l1vROBs []*rob.ReorderBuffer
	l1sROB  *rob.ReorderBuffer
	l1iROB  *rob.ReorderBuffer

	l1vATs []*addresstranslator.AddressTranslator
	l1sAT  *addresstranslator.AddressTranslator
	l1iAT  *addresstranslator.AddressTranslator

	l1vCaches []*l1v.Cache
	l1sCache  *l1v.Cache
	l1iCache  *l1v.Cache

	l1vTLBs []*tlb.TLB
	l1sTLB  *tlb.TLB
	l1iTLB  *tlb.TLB
}

type shaderArrayBuilder struct {
	gpuID uint64
	name  string
	numCU int

	engine            akita.Engine
	freq              akita.Freq
	log2CacheLineSize uint64
	log2PageSize      uint64
	visTracer         tracing.Tracer
	memTracer         tracing.Tracer
}

func makeShaderArrayBuilder() shaderArrayBuilder {
	b := shaderArrayBuilder{
		gpuID:             0,
		name:              "SA",
		numCU:             4,
		freq:              1 * akita.GHz,
		log2CacheLineSize: 6,
		log2PageSize:      12,
	}
	return b
}

func (b shaderArrayBuilder) withEngine(e akita.Engine) shaderArrayBuilder {
	b.engine = e
	return b
}

func (b shaderArrayBuilder) withFreq(f akita.Freq) shaderArrayBuilder {
	b.freq = f
	return b
}

func (b shaderArrayBuilder) withGPUID(id uint64) shaderArrayBuilder {
	b.gpuID = id
	return b
}

func (b shaderArrayBuilder) withNumCU(n int) shaderArrayBuilder {
	b.numCU = n
	return b
}

func (b shaderArrayBuilder) withLog2CachelineSize(
	log2Size uint64,
) shaderArrayBuilder {
	b.log2CacheLineSize = log2Size
	return b
}

func (b shaderArrayBuilder) withLog2PageSize(
	log2Size uint64,
) shaderArrayBuilder {
	b.log2PageSize = log2Size
	return b
}

func (b shaderArrayBuilder) withVisTracer(
	visTracer tracing.Tracer,
) shaderArrayBuilder {
	b.visTracer = visTracer
	return b
}

func (b shaderArrayBuilder) withMemTracer(
	memTracer tracing.Tracer,
) shaderArrayBuilder {
	b.memTracer = memTracer
	return b
}

func (b shaderArrayBuilder) Build(name string) shaderArray {
	b.name = name
	sa := shaderArray{}

	b.buildComponents(&sa)
	b.connectComponents(&sa)

	return sa
}

func (b *shaderArrayBuilder) buildComponents(sa *shaderArray) {
	b.buildCUs(sa)

	b.buildL1VTLBs(sa)
	b.buildL1VAddressTranslators(sa)
	b.buildL1VReorderBuffers(sa)
	b.buildL1VCaches(sa)

	b.buildL1STLB(sa)
	b.buildL1SAddressTranslator(sa)
	b.buildL1SReorderBuffer(sa)
	b.buildL1SCache(sa)

	b.buildL1ITLB(sa)
	b.buildL1IAddressTranslator(sa)
	b.buildL1IReorderBuffer(sa)
	b.buildL1ICache(sa)
}

func (b *shaderArrayBuilder) connectComponents(sa *shaderArray) {
	b.connectVectorMem(sa)
	b.connectScalarMem(sa)
	b.connectInstMem(sa)
}

func (b *shaderArrayBuilder) connectVectorMem(sa *shaderArray) {
	for i := 0; i < b.numCU; i++ {
		cu := sa.cus[i]
		rob := sa.l1vROBs[i]
		at := sa.l1vATs[i]
		l1v := sa.l1vCaches[i]
		tlb := sa.l1vTLBs[i]

		cu.VectorMemModules = &cache.SingleLowModuleFinder{
			LowModule: rob.TopPort,
		}
		b.connectWithDirectConnection(cu.ToVectorMem, rob.TopPort, 8)

		rob.BottomUnit = at.TopPort
		b.connectWithDirectConnection(rob.BottomPort, at.TopPort, 8)

		at.SetTranslationProvider(tlb.TopPort)
		b.connectWithDirectConnection(at.TranslationPort, tlb.TopPort, 8)

		at.SetLowModuleFinder(&cache.SingleLowModuleFinder{
			LowModule: l1v.TopPort,
		})
		b.connectWithDirectConnection(l1v.TopPort, at.BottomPort, 8)
	}
}

func (b *shaderArrayBuilder) connectScalarMem(sa *shaderArray) {
	rob := sa.l1sROB
	at := sa.l1sAT
	tlb := sa.l1sTLB
	l1s := sa.l1sCache

	rob.BottomUnit = at.TopPort
	b.connectWithDirectConnection(rob.BottomPort, at.TopPort, 8)

	at.SetTranslationProvider(tlb.TopPort)
	b.connectWithDirectConnection(at.TranslationPort, tlb.TopPort, 8)

	at.SetLowModuleFinder(&cache.SingleLowModuleFinder{
		LowModule: l1s.TopPort,
	})
	b.connectWithDirectConnection(l1s.TopPort, at.BottomPort, 8)

	conn := akita.NewDirectConnection(b.name, b.engine, b.freq)
	conn.PlugIn(rob.TopPort, 8)
	for i := 0; i < b.numCU; i++ {
		cu := sa.cus[i]
		cu.ScalarMem = rob.TopPort
		conn.PlugIn(cu.ToScalarMem, 8)
	}
}

func (b *shaderArrayBuilder) connectInstMem(sa *shaderArray) {
	rob := sa.l1iROB
	at := sa.l1iAT
	tlb := sa.l1iTLB
	l1i := sa.l1iCache

	rob.BottomUnit = l1i.TopPort
	b.connectWithDirectConnection(rob.BottomPort, l1i.TopPort, 8)

	l1i.SetLowModuleFinder(&cache.SingleLowModuleFinder{
		LowModule: at.TopPort,
	})
	b.connectWithDirectConnection(l1i.BottomPort, at.TopPort, 8)

	at.SetTranslationProvider(tlb.TopPort)
	b.connectWithDirectConnection(at.TranslationPort, tlb.TopPort, 8)

	conn := akita.NewDirectConnection(b.name, b.engine, b.freq)
	conn.PlugIn(rob.TopPort, 8)
	for i := 0; i < b.numCU; i++ {
		cu := sa.cus[i]
		cu.InstMem = rob.TopPort
		conn.PlugIn(cu.ToInstMem, 8)
	}
}

func (b *shaderArrayBuilder) connectWithDirectConnection(
	port1, port2 akita.Port,
	bufferSize int,
) {
	name := fmt.Sprintf("%s-%s", port1.Name(), port2.Name())
	conn := akita.NewDirectConnection(
		name,
		b.engine, b.freq,
	)
	conn.PlugIn(port1, bufferSize)
	conn.PlugIn(port2, bufferSize)
}

func (b *shaderArrayBuilder) buildCUs(sa *shaderArray) {
	cuBuilder := cu.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLog2CachelineSize(b.log2CacheLineSize)

	for i := 0; i < b.numCU; i++ {
		cuName := fmt.Sprintf("%s.CU_%02d", b.name, i)
		cu := cuBuilder.Build(cuName)
		sa.cus = append(sa.cus, cu)

		if b.visTracer != nil {
			tracing.CollectTrace(cu, b.visTracer)
		}
	}
}

func (b *shaderArrayBuilder) buildL1VReorderBuffers(sa *shaderArray) {
	builder := rob.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBufferSize(128).
		WithNumReqPerCycle(4)

	for i := 0; i < b.numCU; i++ {
		name := fmt.Sprintf("%s.L1VROB_%02d", b.name, i)
		rob := builder.Build(name)
		sa.l1vROBs = append(sa.l1vROBs, rob)

		if b.visTracer != nil {
			tracing.CollectTrace(rob, b.visTracer)
		}
	}
}

func (b *shaderArrayBuilder) buildL1VAddressTranslators(sa *shaderArray) {
	builder := addresstranslator.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithGPUID(b.gpuID).
		WithLog2PageSize(b.log2PageSize)

	for i := 0; i < b.numCU; i++ {
		name := fmt.Sprintf("%s.L1VAddrTrans_%02d", b.name, i)
		at := builder.Build(name)
		sa.l1vATs = append(sa.l1vATs, at)

		if b.visTracer != nil {
			tracing.CollectTrace(at, b.visTracer)
		}
	}
}

func (b *shaderArrayBuilder) buildL1VTLBs(sa *shaderArray) {
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumMSHREntry(4).
		WithNumMSHREntry(4).
		WithNumSets(1).
		WithNumWays(64).
		WithNumReqPerCycle(4)

	for i := 0; i < b.numCU; i++ {
		name := fmt.Sprintf("%s.L1VTLB_%02d", b.name, i)
		tlb := builder.Build(name)
		sa.l1vTLBs = append(sa.l1vTLBs, tlb)

		if b.visTracer != nil {
			tracing.CollectTrace(tlb, b.visTracer)
		}
	}
}

func (b *shaderArrayBuilder) buildL1VCaches(sa *shaderArray) {
	builder := l1v.NewBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBankLatency(60).
		WithNumBanks(1).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssocitivity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(16 * mem.KB)

	if b.visTracer != nil {
		builder = builder.WithVisTracer(b.visTracer)
	}

	for i := 0; i < b.numCU; i++ {
		name := fmt.Sprintf("%s.L1VCache_%02d", b.name, i)
		cache := builder.Build(name)
		sa.l1vCaches = append(sa.l1vCaches, cache)

		if b.memTracer != nil {
			tracing.CollectTrace(cache, b.memTracer)
		}
	}
}

func (b *shaderArrayBuilder) buildL1SReorderBuffer(sa *shaderArray) {
	builder := rob.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBufferSize(128).
		WithNumReqPerCycle(4)

	name := fmt.Sprintf("%s.L1SROB", b.name)
	rob := builder.Build(name)
	sa.l1sROB = rob

	if b.visTracer != nil {
		tracing.CollectTrace(rob, b.visTracer)
	}
}

func (b *shaderArrayBuilder) buildL1SAddressTranslator(sa *shaderArray) {
	builder := addresstranslator.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithGPUID(b.gpuID).
		WithLog2PageSize(b.log2PageSize)

	name := fmt.Sprintf("%s.L1SAddrTrans", b.name)
	at := builder.Build(name)
	sa.l1sAT = at

	if b.visTracer != nil {
		tracing.CollectTrace(at, b.visTracer)
	}
}

func (b *shaderArrayBuilder) buildL1STLB(sa *shaderArray) {
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumMSHREntry(4).
		WithNumMSHREntry(4).
		WithNumSets(1).
		WithNumWays(64).
		WithNumReqPerCycle(4)

	name := fmt.Sprintf("%s.L1STLB", b.name)
	tlb := builder.Build(name)
	sa.l1sTLB = tlb

	if b.visTracer != nil {
		tracing.CollectTrace(tlb, b.visTracer)
	}
}

func (b *shaderArrayBuilder) buildL1SCache(sa *shaderArray) {
	builder := l1v.NewBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBankLatency(1).
		WithNumBanks(1).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssocitivity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(16 * mem.KB)

	name := fmt.Sprintf("%s.L1SCache", b.name)
	cache := builder.Build(name)
	sa.l1sCache = cache

	if b.visTracer != nil {
		tracing.CollectTrace(cache, b.visTracer)
	}

	if b.memTracer != nil {
		tracing.CollectTrace(cache, b.memTracer)
	}
}

func (b *shaderArrayBuilder) buildL1IReorderBuffer(sa *shaderArray) {
	builder := rob.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBufferSize(128).
		WithNumReqPerCycle(4)

	name := fmt.Sprintf("%s.L1IROB", b.name)
	rob := builder.Build(name)
	sa.l1iROB = rob

	if b.visTracer != nil {
		tracing.CollectTrace(rob, b.visTracer)
	}
}

func (b *shaderArrayBuilder) buildL1IAddressTranslator(sa *shaderArray) {
	builder := addresstranslator.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithGPUID(b.gpuID).
		WithLog2PageSize(b.log2PageSize)

	name := fmt.Sprintf("%s.L1IAddrTrans", b.name)
	at := builder.Build(name)
	sa.l1iAT = at

	if b.visTracer != nil {
		tracing.CollectTrace(at, b.visTracer)
	}
}

func (b *shaderArrayBuilder) buildL1ITLB(sa *shaderArray) {
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumMSHREntry(4).
		WithNumSets(1).
		WithNumWays(64).
		WithNumReqPerCycle(4)

	name := fmt.Sprintf("%s.L1ITLB", b.name)
	tlb := builder.Build(name)
	sa.l1iTLB = tlb

	if b.visTracer != nil {
		tracing.CollectTrace(tlb, b.visTracer)
	}
}

func (b *shaderArrayBuilder) buildL1ICache(sa *shaderArray) {
	builder := l1v.NewBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBankLatency(1).
		WithNumBanks(1).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssocitivity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(32 * mem.KB).
		WithNumReqsPerCycle(4)

	name := fmt.Sprintf("%s.L1ICache", b.name)
	cache := builder.Build(name)
	sa.l1iCache = cache

	if b.visTracer != nil {
		tracing.CollectTrace(cache, b.visTracer)
	}

	if b.memTracer != nil {
		tracing.CollectTrace(cache, b.memTracer)
	}
}
