package cu

import "github.com/sarchlab/mgpusim/v5/amd/insts"

// A built-in disassembler owns constructor-fixed ISA tables; IsCDNA3 is its
// mutable decoding configuration. Custom decoders can depend on other
// context and deliberately bypass this cache.
type decodeKey struct {
	decoder  *insts.Disassembler
	cdna3    bool
	encoding string
}

// decodeDescription returns a read-only decoded description. Cache entries
// contain only immutable descriptions keyed by full encoding and ISA mode. PC
// and dynamic IDs never enter the cache. Operand and format descriptions remain
// read-only, as they are in the disassembler's own ISA tables.
func (cu *ComputeUnit) decodeDescription(raw []byte) (*insts.Inst, error) {
	decoder, ok := cu.Decoder.(*insts.Disassembler)
	if !ok {
		return cu.Decoder.Decode(raw)
	}
	key := decodeKey{decoder: decoder, cdna3: decoder.IsCDNA3, encoding: string(raw)}
	if value, ok := cu.decoded[key]; ok {
		return value, nil
	}
	value, err := decoder.Decode(raw)
	if err != nil {
		return nil, err
	}
	if cu.decoded == nil {
		cu.decoded = make(map[decodeKey]*insts.Inst)
	}
	cu.decoded[key] = value
	return value, nil
}

func (cu *ComputeUnit) decodeInstruction(raw []byte) (*insts.Inst, error) {
	value, err := cu.decodeDescription(raw)
	if err != nil {
		return nil, err
	}
	if _, ok := cu.Decoder.(*insts.Disassembler); !ok {
		return value, nil
	}
	copy := *value
	return &copy, nil
}
