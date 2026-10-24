package insts

import (
	"debug/elf"
	"slices"
)

// CodeObjectValue is the value representation of an executable kernel. Optional
// metadata and symbols retain their presence independently of their zero values.
type CodeObjectValue struct {
	Present   bool
	HasMeta   bool
	Meta      KernelCodeObjectMeta
	HasSymbol bool
	Symbol    elf.Symbol
	Data      []byte
	Version   CodeObjectVersion
}

// CodeObjectSnapshot detaches a kernel from caller-owned storage.
func CodeObjectSnapshot(o *KernelCodeObject) CodeObjectValue {
	if o == nil {
		return CodeObjectValue{}
	}
	v := CodeObjectValue{Present: true, Data: slices.Clone(o.Data), Version: o.Version}
	if o.KernelCodeObjectMeta != nil {
		v.HasMeta = true
		v.Meta = *o.KernelCodeObjectMeta
	}
	if o.Symbol != nil {
		v.HasSymbol = true
		v.Symbol = *o.Symbol
	}
	return v
}

// Restore reconstructs the local execution view of an immutable code value.
func (v CodeObjectValue) Restore() *KernelCodeObject {
	if !v.Present {
		return nil
	}
	o := &KernelCodeObject{Data: slices.Clone(v.Data), Version: v.Version}
	if v.HasMeta {
		o.KernelCodeObjectMeta = &v.Meta
	}
	if v.HasSymbol {
		o.Symbol = &v.Symbol
	}
	return o
}
