	.text
	.hsa_code_object_version 2,1
	.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"
	.globl	microbench              ; -- Begin function microbench
	.p2align	8
	.type	microbench,@function
	.amdgpu_hsa_kernel microbench
microbench:                             ; @microbench
	.amd_kernel_code_t
	enable_sgpr_kernarg_segment_ptr = 1
	is_ptr64 = 1
	kernarg_segment_byte_size = 24
	workitem_vgpr_count = 4
	wavefront_sgpr_count = 4
	.end_amd_kernel_code_t

	s_load_dwordx2 s[2:3], s[0:1], 0x0
	v_lshlrev_b32 v0, 2, v0
	s_waitcnt lgkmcnt(0)
	v_add_u32 v0, vcc, s2, v0
	v_mov_b32 v1, s3
	v_addc_u32 v1, vcc, v1, 0, vcc

	flat_load_dword v2, v[0:1]
	v_add_u32 v0, vcc, v0, 64
	v_addc_u32 v1, vcc, v1, 0, vcc
	s_waitcnt vmcnt(0) & lgkmcnt(0)

	flat_load_dword v2, v[0:1]
	s_waitcnt vmcnt(0) & lgkmcnt(0)

	s_endpgm


