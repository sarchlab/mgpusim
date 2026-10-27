package insts

import (
	"debug/elf"
	"encoding/json"
	"fmt"
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

// CodeObjectMetadata is dispatch metadata. Executable bytes belong to GPU
// storage and are addressed by the dispatch packet's KernelObject and PID.
// Keeping this distinct from CodeObjectValue prevents per-work-group binaries.
type CodeObjectMetadata struct {
	Present   bool
	HasMeta   bool
	Meta      KernelCodeObjectMeta
	HasSymbol bool
	Symbol    elf.Symbol
	Version   CodeObjectVersion
}

func CodeObjectMetadataSnapshot(o *KernelCodeObject) CodeObjectMetadata {
	if o == nil {
		return CodeObjectMetadata{}
	}
	v := CodeObjectMetadata{Present: true, Version: o.Version}
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

// Restore builds a metadata-only execution view; it does not recreate a binary.
func (v CodeObjectMetadata) Restore() *KernelCodeObject {
	if !v.Present {
		return nil
	}
	o := &KernelCodeObject{Version: v.Version}
	if v.HasMeta {
		o.KernelCodeObjectMeta = &v.Meta
	}
	if v.HasSymbol {
		o.Symbol = &v.Symbol
	}
	return o
}

// UnmarshalJSON rejects the old executable-bearing snapshot explicitly instead
// of silently accepting its unknown Data field and discarding the binary.
//
//nolint:musttag // Flatten the existing checkpoint field names through a method-free alias.
func (v *CodeObjectMetadata) UnmarshalJSON(data []byte) error {
	type metadata CodeObjectMetadata
	var value struct {
		metadata
		Data json.RawMessage
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Data != nil {
		return fmt.Errorf("code metadata: incompatible executable-bearing checkpoint; rebuild checkpoint")
	}
	*v = CodeObjectMetadata(value.metadata)
	return nil
}

// Metadata returns the launch metadata without cloning executable storage.
func (v CodeObjectValue) Metadata() CodeObjectMetadata {
	return CodeObjectMetadata{Present: v.Present, HasMeta: v.HasMeta, Meta: v.Meta,
		HasSymbol: v.HasSymbol, Symbol: v.Symbol, Version: v.Version}
}
