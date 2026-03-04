package gputensor

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// TestCDNA3KernargOffsets verifies that the critical field offsets in
// CDNA3 kernel argument structs match the HSACO metadata. The
// kernarg_segment_size from the HSACO is typically larger than the
// Go struct size because the driver zero-fills the allocation.
func TestCDNA3KernargOffsets(t *testing.T) {
	// Verify cdna3HiddenArgs internal layout
	h := cdna3HiddenArgs{}
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
		t.Errorf("cdna3HiddenArgs binary size: got %d, want 66", sz)
	}

	// Verify repeat struct (280 byte kernarg, hidden at offset 24)
	r := cdna3RepeatArgs{}
	if off := unsafe.Offsetof(r.HiddenBlockCountX); off != 24 {
		t.Errorf("repeat HiddenBlockCountX offset: got %d, want 24", off)
	}

	// Verify im2col struct (320 byte kernarg, hidden at offset 64)
	im := cdna3Im2ColKernelArg{}
	if off := unsafe.Offsetof(im.HiddenBlockCountX); off != 64 {
		t.Errorf("im2col HiddenBlockCountX offset: got %d, want 64", off)
	}

	// Verify gemm_old struct (312 byte kernarg, hidden at offset 56)
	g := cdna3GemmKernArgs{}
	if off := unsafe.Offsetof(g.HiddenBlockCountX); off != 56 {
		t.Errorf("gemm HiddenBlockCountX offset: got %d, want 56", off)
	}

	// Verify transpose struct (320 byte kernarg, hidden at offset 64)
	tr := cdna3TransposeKernelArgs{}
	if off := unsafe.Offsetof(tr.HiddenBlockCountX); off != 64 {
		t.Errorf("transpose HiddenBlockCountX offset: got %d, want 64", off)
	}

	// Verify MaxPoolForward (336 byte kernarg, hidden at offset 80)
	mp := cdna3MaxPoolingForwardKernelArgs{}
	if off := unsafe.Offsetof(mp.HiddenBlockCountX); off != 80 {
		t.Errorf("MaxPoolForward HiddenBlockCountX offset: got %d, want 80", off)
	}

	// Verify softmax_exp (280 byte kernarg, hidden at offset 24)
	se := cdna3SoftmaxExpArgs{}
	if off := unsafe.Offsetof(se.HiddenBlockCountX); off != 24 {
		t.Errorf("softmax_exp HiddenBlockCountX: got %d, want 24", off)
	}

	// Verify softmax_div (288 byte kernarg, hidden at offset 32)
	sd := cdna3SoftmaxDivArgs{}
	if off := unsafe.Offsetof(sd.HiddenBlockCountX); off != 32 {
		t.Errorf("softmax_div HiddenBlockCountX: got %d, want 32", off)
	}

	// Verify sum_one_axis (312 byte kernarg, hidden at offset 56)
	sa := cdna3SumOneAxisKernelArgs{}
	if off := unsafe.Offsetof(sa.HiddenBlockCountX); off != 56 {
		t.Errorf("sum_one_axis HiddenBlockCountX: got %d, want 56", off)
	}

	// Verify cross_entropy (288 byte kernarg, hidden at offset 32)
	ce := cdna3CrossEntropyDerivativeArgs{}
	if off := unsafe.Offsetof(ce.HiddenBlockCountX); off != 32 {
		t.Errorf("cross_entropy HiddenBlockCountX: got %d, want 32", off)
	}

	// Verify adam (304 byte kernarg, hidden at offset 48)
	a := cdna3AdamArgs{}
	if off := unsafe.Offsetof(a.HiddenBlockCountX); off != 48 {
		t.Errorf("adam HiddenBlockCountX: got %d, want 48", off)
	}

	// Verify scaleAdd (296 byte kernarg, hidden at offset 40)
	sc := cdna3ScaleAddArgs{}
	if off := unsafe.Offsetof(sc.HiddenBlockCountX); off != 40 {
		t.Errorf("scaleAdd HiddenBlockCountX: got %d, want 40", off)
	}

	// Verify mul (288 byte kernarg, hidden at offset 32)
	m := cdna3MulArgs{}
	if off := unsafe.Offsetof(m.HiddenBlockCountX); off != 32 {
		t.Errorf("mul HiddenBlockCountX: got %d, want 32", off)
	}

	// Verify rmsProp (296 byte kernarg, hidden at offset 40)
	rp := cdna3RmsPropArgs{}
	if off := unsafe.Offsetof(rp.HiddenBlockCountX); off != 40 {
		t.Errorf("rmsProp HiddenBlockCountX: got %d, want 40", off)
	}

	// Verify reluForward (280 byte kernarg, hidden at offset 24)
	rf := cdna3ReluForwardArgs{}
	if off := unsafe.Offsetof(rf.HiddenBlockCountX); off != 24 {
		t.Errorf("reluForward HiddenBlockCountX: got %d, want 24", off)
	}

	// Verify reluBackward (288 byte kernarg, hidden at offset 32)
	rb := cdna3ReluBackwardArgs{}
	if off := unsafe.Offsetof(rb.HiddenBlockCountX); off != 32 {
		t.Errorf("reluBackward HiddenBlockCountX: got %d, want 32", off)
	}
}
