package insts_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/insts"
)

func TestDisassembler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCN3 Disassembler")
}

var _ = Describe("Disassembler", func() {

	var (
		disassembler *insts.Disassembler
	)

	BeforeEach(func() {
		disassembler = insts.NewDisassembler()
	})

	It("should decode BF8C0F70", func() {
		buf := []byte{0x70, 0x0f, 0x8c, 0xbf}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("s_waitcnt vmcnt(0)"))
	})

	It("should decode BF8C0171", func() {
		buf := []byte{0x71, 0x01, 0x8c, 0xbf}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("s_waitcnt vmcnt(1) lgkmcnt(1)"))
	})

	It("should decode D81A0004 00000210", func() {
		buf := []byte{0x04, 0x00, 0x1A, 0xd8, 0x10, 0x02, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("ds_write_b32 v16, v2 offset:4"))
	})

	It("should decode D86C0008 01000010", func() {
		buf := []byte{0x08, 0x00, 0x6c, 0xd8, 0x10, 0x00, 0x00, 0x01}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("ds_read_b32 v1, v16 offset:8"))
	})

	It("should decode D2850001 00000503", func() {
		buf := []byte{0x01, 0x00, 0x85, 0xd2, 0x03, 0x05, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("v_mul_lo_u32 v1, v3, s2"))
	})

	It("should decode D2860004 00000503", func() {
		buf := []byte{0x04, 0x00, 0x86, 0xd2, 0x03, 0x05, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("v_mul_hi_u32 v4, v3, s2"))
	})

	It("should decode 041C0D0E", func() {
		buf := []byte{0x0e, 0x0d, 0x1c, 0x04}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("v_sub_f32_e32 v14, v14, v6"))
	})

	It("should decode 7E224911", func() {
		buf := []byte{0x11, 0x49, 0x22, 0x7e}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("v_rsq_f32_e32 v17, v17"))
	})

	It("should decode 7E540900", func() {
		buf := []byte{0x00, 0x09, 0x54, 0x7e}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("v_cvt_f64_i32_e32 v[42:43], v0"))
	})

	It("should decode 7E221F0F", func() {
		buf := []byte{0x0f, 0x1f, 0x22, 0x7e}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("v_cvt_f32_f64_e32 v17, v[15:16]"))
	})

	It("should decode D281000F 0000012A", func() {
		buf := []byte{0x0f, 0x00, 0x81, 0xd2, 0x2a, 0x01, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_mul_f64 v[15:16], v[42:43], s[0:1]"))
	})

	It("should decode D04E0100 00000111", func() {
		buf := []byte{0x00, 0x01, 0x4e, 0xd0, 0x11, 0x01, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_cmp_nlt_f32_e64 s[0:1], |v17|, s0"))
	})

	It("should decode 309E9F16 BE2AAA9D", func() {
		buf := []byte{0x16, 0x9f, 0x9e, 0x30, 0x9d, 0xaa, 0x2a, 0xbe}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_madak_f32 v79, v22, v79, 0xbe2aaa9d"))
	})

	It("should decode D81C4200 002E1411", func() {
		buf := []byte{0x00, 0x42, 0x1c, 0xd8, 0x11, 0x14, 0x2e, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("ds_write2_b32 v17, v20, v46 offset1:66"))
	})

	It("should decode D81A03C0 0000211F", func() {
		buf := []byte{0xC0, 0x03, 0x1a, 0xd8, 0x1F, 0x21, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("ds_write_b32 v31, v33 offset:960"))
	})

	It("should decode D11B0006 0002060D", func() {
		buf := []byte{0x06, 0x00, 0x1b, 0xd1, 0x0d, 0x06, 0x02, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_subrev_u32_e64 v6, s[0:1], s13, v3"))
	})

	It("should decode D11A0222 00020081", func() {
		buf := []byte{0x22, 0x02, 0x1a, 0xd1, 0x81, 0x00, 0x02, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_sub_u32_e64 v34, s[2:3], 1, v0"))
	})

	It("should decode D1CC0002 041A0504", func() {
		buf := []byte{0x02, 0x00, 0xcc, 0xd1, 0x04, 0x05, 0x1a, 0x04}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_fma_f64 v[2:3], v[4:5], v[2:3], v[6:7]"))
	})

	It("should decode 7E0C2106", func() {
		buf := []byte{0x06, 0x21, 0x0c, 0x7e}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_cvt_f64_f32_e32 v[6:7], v6"))
	})

	It("should decode D2800004 00000104", func() {
		buf := []byte{0x04, 0x00, 0x80, 0xd2, 0x04, 0x01, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_add_f64 v[4:5], v[4:5], s[0:1]"))
	})

	It("should decode D1E10008 03CE0904", func() {
		buf := []byte{0x08, 0x00, 0xe1, 0xd1, 0x04, 0x09, 0xce, 0x03}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_div_scale_f64 v[8:9], s[0:1], v[4:5], v[4:5], -1.0"))
	})

	It("should devoce 7E144B08", func() {
		buf := []byte{0x08, 0x4b, 0x14, 0x7e}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_rcp_f64_e32 v[10:11], v[8:9]"))
	})

	It("should decode D1E30008 043A1508", func() {
		buf := []byte{0x08, 0x00, 0xe3, 0xd1, 0x08, 0x15, 0x3a, 0x04}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_div_fmas_f64 v[8:9], v[8:9], v[10:11], v[14:15]"))
	})

	It("should decode D1DF0002 03CE0D08", func() {
		buf := []byte{0x02, 0x00, 0xdf, 0xd1, 0x08, 0x0d, 0xce, 0x03}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("v_div_fixup_f64 v[2:3], v[8:9], v[6:7], -1.0"))
	})

	It("should decode D9BE0000 00000D09", func() {
		buf := []byte{0x00, 0x00, 0xbe, 0xd9, 0x09, 0x0d, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("ds_write_b128 v9, v[13:16]"))
	})

	It("should decode D9FE0080 11000001", func() {
		buf := []byte{0x80, 0x00, 0xfe, 0xd9, 0x01, 0x00, 0x00, 0x11}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).
			To(Equal("ds_read_b128 v[17:20], v1 offset:128"))
	})
})
