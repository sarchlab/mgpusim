package simplebankedmemory

import (
	"fmt"
	"strings"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
)

// Builder constructs SimpleBankedMemory components.
type Builder struct {
	engine sim.Engine
	freq   sim.Freq

	numBanks            int
	bankPipelineWidth   int
	bankPipelineDepth   int
	stageLatency        int
	topPortBufferSize   int
	postPipelineBufSize int

	bankSelectorType   string
	log2InterleaveSize uint64
	customBankSelector bankSelector

	capacity             uint64
	storage              *mem.Storage
	addressConverter     mem.AddressConverter
	bankAddrConverter    mem.AddressConverter
}

// MakeBuilder creates a builder with reasonable defaults.
func MakeBuilder() Builder {
	return Builder{
		freq:                1 * sim.GHz,
		numBanks:            4,
		bankPipelineWidth:   1,
		bankPipelineDepth:   1,
		stageLatency:        10,
		topPortBufferSize:   16,
		postPipelineBufSize: 1,
		bankSelectorType:    "interleaved",
		log2InterleaveSize:  6,
		capacity:            4 * mem.GB,
	}
}

// WithEngine sets the simulation engine.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the component frequency.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithNumBanks sets the number of banks.
func (b Builder) WithNumBanks(numBanks int) Builder {
	b.numBanks = numBanks
	return b
}

// WithBankPipelineWidth sets the pipeline width inside each bank.
func (b Builder) WithBankPipelineWidth(width int) Builder {
	b.bankPipelineWidth = width
	return b
}

// WithBankPipelineDepth sets the pipeline depth inside each bank.
func (b Builder) WithBankPipelineDepth(depth int) Builder {
	b.bankPipelineDepth = depth
	return b
}

// WithStageLatency sets the latency of each pipeline stage in cycles.
func (b Builder) WithStageLatency(latency int) Builder {
	b.stageLatency = latency
	return b
}

// WithTopPortBufferSize sets the buffer size of the top port.
func (b Builder) WithTopPortBufferSize(size int) Builder {
	b.topPortBufferSize = size
	return b
}

// WithPostPipelineBufferSize sets the post-pipeline buffer capacity per bank.
func (b Builder) WithPostPipelineBufferSize(size int) Builder {
	b.postPipelineBufSize = size
	return b
}

// WithBankSelectorType selects the bank selector implementation by name.
// Supported selectors:
//   - "interleaved": addresses are interleaved across banks using log2InterleaveSize.
func (b Builder) WithBankSelectorType(selectorType string) Builder {
	b.bankSelectorType = selectorType
	return b
}

// WithLog2InterleaveSize sets the log2 interleave size used by the default selector.
func (b Builder) WithLog2InterleaveSize(log2Size uint64) Builder {
	b.log2InterleaveSize = log2Size
	return b
}

// WithBankSelector overrides the bank selector with a custom implementation.
func (b Builder) WithBankSelector(selector bankSelector) Builder {
	b.customBankSelector = selector
	return b
}

// WithStorage reuses an existing storage object.
func (b Builder) WithStorage(storage *mem.Storage) Builder {
	b.storage = storage
	return b
}

// WithNewStorage creates a new storage with the given capacity.
func (b Builder) WithNewStorage(capacity uint64) Builder {
	b.capacity = capacity
	return b
}

// WithAddressConverter sets the address converter used for storage read/write.
func (b Builder) WithAddressConverter(
	addressConverter mem.AddressConverter,
) Builder {
	b.addressConverter = addressConverter
	return b
}

// WithBankAddressConverter sets a separate address converter used ONLY for
// bank selection in dispatchPending. When set, this converter is used instead
// of AddressConverter for choosing which bank to route a request to, while
// storage read/write continues to use AddressConverter (or no conversion).
func (b Builder) WithBankAddressConverter(
	converter mem.AddressConverter,
) Builder {
	b.bankAddrConverter = converter
	return b
}

// Build creates a SimpleBankedMemory component.
func (b Builder) Build(name string) *Comp {
	b.configurationMustBeValid()

	var storage *mem.Storage
	if b.storage != nil {
		storage = b.storage
	} else {
		storage = mem.NewStorage(b.capacity)
	}

	c := &Comp{
		Storage:              storage,
		AddressConverter:     b.addressConverter,
		BankAddressConverter: b.bankAddrConverter,
		bankSelector:         b.determineBankSelector(),
	}

	c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, c)

	c.topPort = sim.NewPort(c, b.topPortBufferSize, b.topPortBufferSize, name+".TopPort")
	c.AddPort("Top", c.topPort)

	c.banks = make([]bank, b.numBanks)

	for i := range c.banks {
		postPipelineBuf := sim.NewBuffer(
			fmt.Sprintf("%s.Bank[%d].PostPipelineBuffer", name, i),
			b.postPipelineBufSize,
		)

		pipeline := pipelining.MakeBuilder().
			WithPipelineWidth(b.bankPipelineWidth).
			WithNumStage(b.bankPipelineDepth).
			WithCyclePerStage(b.stageLatency).
			WithPostPipelineBuffer(postPipelineBuf).
			Build(fmt.Sprintf("%s.Bank[%d].Pipeline", name, i))

		c.banks[i] = bank{
			pipeline:        pipeline,
			postPipelineBuf: postPipelineBuf,
		}
	}

	c.AddMiddleware(&middleware{Comp: c})

	return c
}

func (b Builder) configurationMustBeValid() {
	if b.engine == nil {
		panic("simplebankedmemory.Builder: engine is nil; call WithEngine")
	}

	if b.numBanks <= 0 {
		panic("simplebankedmemory.Builder: numBanks must be > 0")
	}

	if b.bankPipelineWidth <= 0 {
		panic("simplebankedmemory.Builder: bankPipelineWidth must be > 0")
	}

	if b.bankPipelineDepth <= 0 {
		panic("simplebankedmemory.Builder: bankPipelineDepth must be > 0")
	}

	if b.stageLatency <= 0 {
		panic("simplebankedmemory.Builder: stageLatency must be > 0")
	}

	if b.topPortBufferSize <= 0 {
		panic("simplebankedmemory.Builder: topPortBufferSize must be > 0")
	}

	if b.postPipelineBufSize <= 0 {
		panic("simplebankedmemory.Builder: postPipelineBufSize must be > 0")
	}
}

func (b Builder) determineBankSelector() bankSelector {
	if b.customBankSelector != nil {
		return b.customBankSelector
	}

	selectorType := strings.ToLower(b.bankSelectorType)
	switch selectorType {
	case "", "interleaved":
		return interleavedBankSelector{
			Log2InterleaveSize: b.log2InterleaveSize,
		}
	default:
		panic(fmt.Sprintf("simplebankedmemory.Builder: unsupported bank selector %q", b.bankSelectorType))
	}
}
