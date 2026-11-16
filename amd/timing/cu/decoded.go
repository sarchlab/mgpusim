package cu

import "github.com/sarchlab/mgpusim/v5/amd/insts"

// A built-in disassembler owns constructor-fixed ISA tables; IsCDNA3 is its
// mutable decoding configuration. Custom decoders can depend on other
// context and deliberately bypass this cache.
type decodeKey struct {
	decoder  *insts.Disassembler
	cdna3    bool
	encoding string
	pc       uint64
}

// decodeDescription returns a read-only decoded description. Cache entries
// contain immutable descriptions keyed by encoding, decoder, ISA mode and PC.
// Dynamic IDs never enter the cache. Operands and formats also remain read-only.
func (cu *ComputeUnit) decodeDescription(raw []byte, pc uint64) (*insts.Inst, error) {
	decoder, ok := cu.Decoder.(*insts.Disassembler)
	if !ok {
		value, err := cu.Decoder.Decode(raw)
		if err == nil {
			value.PC = pc
		}
		return value, err
	}
	if value, ok := cu.decoded[decodeKey{decoder: decoder, cdna3: decoder.IsCDNA3, encoding: string(raw), pc: pc}]; ok {
		return value, nil
	}
	value, err := decoder.Decode(raw)
	if err != nil {
		return nil, err
	}
	if cu.decoded == nil {
		cu.decoded = make(map[decodeKey]*insts.Inst)
	}
	value.PC = pc
	cu.decoded[decodeKey{decoder: decoder, cdna3: decoder.IsCDNA3, encoding: string(raw), pc: pc}] = value
	return value, nil
}
