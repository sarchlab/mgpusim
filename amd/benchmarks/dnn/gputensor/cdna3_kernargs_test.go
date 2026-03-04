package gputensor

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// TestCDNA3HiddenArgsLayout verifies CDNA3HiddenArgs internal layout.
func TestCDNA3HiddenArgsLayout(t *testing.T) {
	h := CDNA3HiddenArgs{}
	if off := unsafe.Offsetof(h.HiddenBlockCountX); off != 0 {
		t.Errorf("HiddenBlockCountX offset: got %d, want 0", off)
	}
	if off := unsafe.Offsetof(h.HiddenGlobalOffsetX); off != 40 {
		t.Errorf("HiddenGlobalOffsetX offset: got %d, want 40", off)
	}
	if off := unsafe.Offsetof(h.HiddenGridDims); off != 64 {
		t.Errorf("HiddenGridDims offset: got %d, want 64", off)
	}
	if sz := binary.Size(h); sz != 66 {
		t.Errorf("CDNA3HiddenArgs binary size: got %d, want 66", sz)
	}
}

// TestCDNA3KernargOffsetsBasic verifies hidden arg offsets for
// basic kernel arg structs (repeat, im2col, gemm, transpose).
func TestCDNA3KernargOffsetsBasic(t *testing.T) {
	r := cdna3RepeatArgs{}
	if off := unsafe.Offsetof(r.HiddenBlockCountX); off != 24 {
		t.Errorf("repeat HiddenBlockCountX offset: got %d, want 24", off)
	}

	im := cdna3Im2ColKernelArg{}
	if off := unsafe.Offsetof(im.HiddenBlockCountX); off != 64 {
		t.Errorf("im2col HiddenBlockCountX offset: got %d, want 64", off)
	}

	g := cdna3GemmKernArgs{}
	if off := unsafe.Offsetof(g.HiddenBlockCountX); off != 56 {
		t.Errorf("gemm HiddenBlockCountX offset: got %d, want 56", off)
	}

	tr := cdna3TransposeKernelArgs{}
	if off := unsafe.Offsetof(tr.HiddenBlockCountX); off != 64 {
		t.Errorf("transpose HiddenBlockCountX offset: got %d, want 64", off)
	}
}

// TestCDNA3KernargOffsetsPooling verifies hidden arg offsets
// for pooling and softmax kernel arg structs.
func TestCDNA3KernargOffsetsPooling(t *testing.T) {
	mp := cdna3MaxPoolingForwardKernelArgs{}
	if off := unsafe.Offsetof(mp.HiddenBlockCountX); off != 80 {
		t.Errorf("MaxPoolForward HiddenBlockCountX offset: got %d, want 80", off)
	}

	se := cdna3SoftmaxExpArgs{}
	if off := unsafe.Offsetof(se.HiddenBlockCountX); off != 24 {
		t.Errorf("softmax_exp HiddenBlockCountX: got %d, want 24", off)
	}

	sd := cdna3SoftmaxDivArgs{}
	if off := unsafe.Offsetof(sd.HiddenBlockCountX); off != 32 {
		t.Errorf("softmax_div HiddenBlockCountX: got %d, want 32", off)
	}

	sa := cdna3SumOneAxisKernelArgs{}
	if off := unsafe.Offsetof(sa.HiddenBlockCountX); off != 56 {
		t.Errorf("sum_one_axis HiddenBlockCountX: got %d, want 56", off)
	}

	ce := cdna3CrossEntropyDerivativeArgs{}
	if off := unsafe.Offsetof(ce.HiddenBlockCountX); off != 32 {
		t.Errorf("cross_entropy HiddenBlockCountX: got %d, want 32", off)
	}
}

// TestCDNA3KernargOffsetsOptimizers verifies hidden arg offsets
// for optimizer and activation kernel arg structs.
func TestCDNA3KernargOffsetsOptimizers(t *testing.T) {
	a := cdna3AdamArgs{}
	if off := unsafe.Offsetof(a.HiddenBlockCountX); off != 48 {
		t.Errorf("adam HiddenBlockCountX: got %d, want 48", off)
	}

	sc := cdna3ScaleAddArgs{}
	if off := unsafe.Offsetof(sc.HiddenBlockCountX); off != 40 {
		t.Errorf("scaleAdd HiddenBlockCountX: got %d, want 40", off)
	}

	m := cdna3MulArgs{}
	if off := unsafe.Offsetof(m.HiddenBlockCountX); off != 32 {
		t.Errorf("mul HiddenBlockCountX: got %d, want 32", off)
	}

	rp := cdna3RmsPropArgs{}
	if off := unsafe.Offsetof(rp.HiddenBlockCountX); off != 40 {
		t.Errorf("rmsProp HiddenBlockCountX: got %d, want 40", off)
	}

	rf := cdna3ReluForwardArgs{}
	if off := unsafe.Offsetof(rf.HiddenBlockCountX); off != 24 {
		t.Errorf("reluForward HiddenBlockCountX: got %d, want 24", off)
	}

	rb := cdna3ReluBackwardArgs{}
	if off := unsafe.Offsetof(rb.HiddenBlockCountX); off != 32 {
		t.Errorf("reluBackward HiddenBlockCountX: got %d, want 32", off)
	}
}
