	.text
	.hsa_code_object_version 2,1
	.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"
	.globl	Encrypt                 ; -- Begin function Encrypt
	.p2align	8
	.type	Encrypt,@function
	.amdgpu_hsa_kernel Encrypt
Encrypt:                                ; @Encrypt
	.amd_kernel_code_t
		amd_code_version_major = 1
		amd_code_version_minor = 1
		amd_machine_kind = 1
		amd_machine_version_major = 8
		amd_machine_version_minor = 0
		amd_machine_version_stepping = 3
		kernel_code_entry_byte_offset = 256
		kernel_code_prefetch_byte_size = 0
		max_scratch_backing_memory_byte_size = 0
		granulated_workitem_vgpr_count = 10
		granulated_wavefront_sgpr_count = 3
		priority = 0
		float_mode = 192
		priv = 0
		enable_dx10_clamp = 1
		debug_mode = 0
		enable_ieee_mode = 1
		enable_sgpr_private_segment_wave_byte_offset = 0
		user_sgpr_count = 8
		enable_trap_handler = 1
		enable_sgpr_workgroup_id_x = 1
		enable_sgpr_workgroup_id_y = 0
		enable_sgpr_workgroup_id_z = 0
		enable_sgpr_workgroup_info = 0
		enable_vgpr_workitem_id = 0
		enable_exception_msb = 0
		granulated_lds_size = 0
		enable_exception = 0
		enable_sgpr_private_segment_buffer = 1
		enable_sgpr_dispatch_ptr = 1
		enable_sgpr_queue_ptr = 0
		enable_sgpr_kernarg_segment_ptr = 1
		enable_sgpr_dispatch_id = 0
		enable_sgpr_flat_scratch_init = 0
		enable_sgpr_private_segment_size = 0
		enable_sgpr_grid_workgroup_count_x = 0
		enable_sgpr_grid_workgroup_count_y = 0
		enable_sgpr_grid_workgroup_count_z = 0
		enable_ordered_append_gds = 0
		private_element_size = 1
		is_ptr64 = 1
		is_dynamic_callstack = 0
		is_debug_enabled = 0
		is_xnack_enabled = 0
		workitem_private_segment_byte_size = 0
		workgroup_group_segment_byte_size = 0
		gds_segment_byte_size = 0
		kernarg_segment_byte_size = 48
		workgroup_fbarrier_count = 0
		wavefront_sgpr_count = 27
		workitem_vgpr_count = 44
		reserved_vgpr_first = 0
		reserved_vgpr_count = 0
		reserved_sgpr_first = 0
		reserved_sgpr_count = 0
		debug_wavefront_private_segment_offset_sgpr = 0
		debug_private_segment_buffer_sgpr = 0
		kernarg_segment_alignment = 4
		group_segment_alignment = 4
		private_segment_alignment = 4
		wavefront_size = 6
		call_convention = -1
		runtime_loader_kernel_symbol = 0
	.end_amd_kernel_code_t
; BB#0:
	s_load_dword s4, s[4:5], 0x4
	s_load_dwordx2 s[2:3], s[6:7], 0x0
	s_load_dwordx2 s[0:1], s[6:7], 0x8
	s_load_dword s5, s[6:7], 0x10
	s_mov_b32 s12, 0xffff
	s_waitcnt lgkmcnt(0)
	s_and_b32 s4, s4, s12
	s_mul_i32 s8, s8, s4
	v_mov_b32_e32 v1, s3
	v_add_u32_e32 v0, vcc, s5, v0
	v_add_u32_e32 v0, vcc, s8, v0
	v_lshlrev_b32_e32 v0, 4, v0
	v_ashrrev_i32_e32 v2, 31, v0
	v_add_u32_e32 v0, vcc, s2, v0
	v_addc_u32_e32 v1, vcc, v1, v2, vcc
	flat_load_dwordx4 v[8:11], v[0:1]
	s_load_dwordx4 s[8:11], s[0:1], 0x0
	s_getpc_b64 s[2:3]
	s_add_u32 s2, s2, s@gotpcrel32@lo+4
	s_addc_u32 s3, s3, s@gotpcrel32@hi+4
	s_load_dwordx2 s[2:3], s[2:3], 0x0
	s_movk_i32 s4, 0xff
	s_mov_b64 s[6:7], 0
	s_waitcnt lgkmcnt(0)
	s_lshr_b32 s13, s8, 24
	s_lshr_b32 s14, s8, 16
	s_lshr_b32 s15, s8, 8
	s_lshr_b32 s16, s9, 24
	s_lshr_b32 s17, s9, 16
	s_lshr_b32 s18, s9, 8
	s_lshr_b32 s19, s10, 24
	s_lshr_b32 s20, s10, 16
	s_lshr_b32 s21, s10, 8
	s_lshr_b32 s22, s11, 24
	s_lshr_b32 s23, s11, 16
	s_lshr_b32 s24, s11, 8
	v_mov_b32_e32 v2, s8
	v_mov_b32_e32 v5, s9
	v_mov_b32_e32 v3, s13
	v_mov_b32_e32 v4, s14
	v_mov_b32_e32 v13, s16
	v_mov_b32_e32 v14, s17
	v_mov_b32_e32 v16, s19
	v_mov_b32_e32 v7, s10
	v_mov_b32_e32 v12, s11
	v_mov_b32_e32 v6, s15
	v_mov_b32_e32 v15, s18
	v_mov_b32_e32 v18, s20
	v_mov_b32_e32 v25, s21
	v_mov_b32_e32 v26, s22
	v_mov_b32_e32 v27, s23
	v_mov_b32_e32 v28, s24
	s_movk_i32 s5, 0x80
	s_waitcnt vmcnt(0)
	v_xor_b32_sdwa v24, v3, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v23, v4, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_1
	v_xor_b32_sdwa v4, v6, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_2
	v_xor_b32_sdwa v19, v2, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_3
	v_xor_b32_sdwa v20, v14, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_1
	v_xor_b32_sdwa v17, v5, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_3
	v_xor_b32_sdwa v6, v18, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_1
	v_xor_b32_sdwa v5, v7, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_3
	v_xor_b32_sdwa v22, v13, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v3, v15, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_2
	v_xor_b32_sdwa v21, v16, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v9, v25, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_2
	v_xor_b32_sdwa v18, v26, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v8, v27, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_1
	v_xor_b32_sdwa v7, v28, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_2
	v_xor_b32_sdwa v14, v12, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_3
	v_mov_b32_e32 v2, s12
	s_branch BB0_2
BB0_1:                                  ;   in Loop: Header=BB0_2 Depth=1
	s_waitcnt vmcnt(5) lgkmcnt(5)
	v_and_b32_sdwa v8, v2, v13 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v20, s5, v8
	v_lshlrev_b32_e32 v8, 1, v8
	s_waitcnt vmcnt(2) lgkmcnt(2)
	v_and_b32_sdwa v19, v2, v15 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cmp_eq_u32_e32 vcc, 0, v20
	v_xor_b32_e32 v21, 27, v8
	v_and_b32_e32 v20, s5, v19
	v_lshlrev_b32_e32 v19, 1, v19
	v_cndmask_b32_e32 v8, v21, v8, vcc
	v_cmp_eq_u32_e32 vcc, 0, v20
	v_xor_b32_e32 v21, 27, v19
	v_and_b32_sdwa v20, v2, v18 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v19, v21, v19, vcc
	v_and_b32_e32 v21, s5, v20
	v_lshlrev_b32_e32 v20, 1, v20
	v_xor_b32_e32 v23, 27, v20
	v_cmp_eq_u32_e32 vcc, 0, v21
	v_cndmask_b32_e32 v29, v23, v20, vcc
	v_and_b32_sdwa v20, v2, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v21, s5, v20
	v_lshlrev_b32_e32 v20, 1, v20
	v_xor_b32_e32 v23, 27, v20
	v_cmp_eq_u32_e32 vcc, 0, v21
	v_cndmask_b32_e32 v30, v23, v20, vcc
	v_xor_b32_e32 v20, v15, v8
	v_xor_b32_e32 v8, v13, v8
	v_xor_b32_e32 v20, v20, v19
	v_xor_b32_e32 v19, v13, v19
	v_xor_b32_e32 v8, v8, v15
	v_xor_b32_e32 v8, v8, v18
	v_xor_b32_e32 v20, v20, v18
	v_xor_b32_e32 v19, v19, v18
	v_and_b32_sdwa v18, v2, v22 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v21, s5, v18
	v_lshlrev_b32_e32 v18, 1, v18
	v_cmp_eq_u32_e32 vcc, 0, v21
	v_xor_b32_e32 v23, 27, v18
	s_waitcnt vmcnt(1) lgkmcnt(1)
	v_and_b32_sdwa v21, v2, v12 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v18, v23, v18, vcc
	v_and_b32_e32 v23, s5, v21
	v_lshlrev_b32_e32 v21, 1, v21
	v_cmp_eq_u32_e32 vcc, 0, v23
	v_xor_b32_e32 v24, 27, v21
	v_and_b32_sdwa v23, v2, v14 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v21, v24, v21, vcc
	v_and_b32_e32 v24, s5, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_xor_b32_e32 v25, 27, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_cndmask_b32_e32 v31, v25, v23, vcc
	v_and_b32_sdwa v23, v2, v6 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v24, s5, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_xor_b32_e32 v25, 27, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_xor_b32_e32 v33, v22, v6
	v_xor_b32_e32 v6, v6, v18
	v_cndmask_b32_e32 v32, v25, v23, vcc
	v_xor_b32_e32 v6, v6, v12
	v_xor_b32_e32 v22, v22, v32
	v_xor_b32_e32 v18, v22, v18
	v_xor_b32_e32 v6, v6, v21
	v_xor_b32_e32 v22, v6, v14
	v_xor_b32_e32 v6, v33, v21
	v_xor_b32_e32 v18, v18, v12
	v_xor_b32_e32 v6, v6, v14
	v_xor_b32_e32 v14, v18, v14
	v_and_b32_sdwa v18, v2, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v21, s5, v18
	v_lshlrev_b32_e32 v18, 1, v18
	v_cmp_eq_u32_e32 vcc, 0, v21
	v_xor_b32_e32 v23, 27, v18
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_and_b32_sdwa v21, v2, v16 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v18, v23, v18, vcc
	v_and_b32_e32 v23, s5, v21
	v_lshlrev_b32_e32 v21, 1, v21
	v_cmp_eq_u32_e32 vcc, 0, v23
	v_xor_b32_e32 v24, 27, v21
	v_and_b32_sdwa v23, v2, v17 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v21, v24, v21, vcc
	v_and_b32_e32 v24, s5, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_xor_b32_e32 v25, 27, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_cndmask_b32_e32 v34, v25, v23, vcc
	v_and_b32_sdwa v23, v2, v4 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v24, s5, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_xor_b32_e32 v25, 27, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_cndmask_b32_e32 v35, v25, v23, vcc
	v_xor_b32_e32 v23, v4, v17
	v_xor_b32_e32 v23, v23, v18
	v_xor_b32_e32 v23, v23, v16
	v_xor_b32_e32 v36, v23, v21
	v_xor_b32_e32 v23, v17, v34
	v_xor_b32_e32 v17, v10, v17
	v_xor_b32_e32 v23, v23, v4
	v_xor_b32_e32 v17, v17, v35
	v_xor_b32_e32 v17, v17, v18
	v_xor_b32_e32 v23, v23, v10
	v_and_b32_sdwa v18, v2, v7 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_e32 v37, v23, v21
	v_and_b32_e32 v21, s5, v18
	v_lshlrev_b32_e32 v18, 1, v18
	v_cmp_eq_u32_e32 vcc, 0, v21
	v_xor_b32_e32 v23, 27, v18
	v_and_b32_sdwa v21, v2, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v18, v23, v18, vcc
	v_and_b32_e32 v23, s5, v21
	v_lshlrev_b32_e32 v21, 1, v21
	v_cmp_eq_u32_e32 vcc, 0, v23
	v_xor_b32_e32 v24, 27, v21
	v_and_b32_sdwa v23, v2, v5 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v21, v24, v21, vcc
	v_and_b32_e32 v24, s5, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_xor_b32_e32 v25, 27, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_cndmask_b32_e32 v38, v25, v23, vcc
	v_and_b32_sdwa v23, v2, v3 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v24, s5, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_xor_b32_e32 v25, 27, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_cndmask_b32_e32 v39, v25, v23, vcc
	v_xor_b32_e32 v23, v9, v21
	v_xor_b32_e32 v23, v23, v5
	v_xor_b32_e32 v21, v5, v21
	v_xor_b32_e32 v5, v5, v9
	v_xor_b32_e32 v5, v5, v7
	s_add_u32 s8, s0, s6
	v_xor_b32_e32 v5, v5, v39
	s_addc_u32 s9, s1, s7
	v_xor_b32_e32 v41, v5, v18
	v_xor_b32_e32 v5, v19, v29
	s_add_u32 s8, s8, 16
	v_xor_b32_e32 v19, v5, v11
	v_xor_b32_e32 v5, v21, v38
	s_addc_u32 s9, s9, 0
	v_xor_b32_e32 v23, v23, v3
	v_xor_b32_e32 v5, v5, v3
	v_xor_b32_e32 v40, v23, v18
	v_xor_b32_e32 v18, v20, v11
	v_xor_b32_e32 v20, v6, v31
	v_xor_b32_e32 v43, v5, v7
	v_mov_b32_e32 v5, s8
	v_mov_b32_e32 v6, s9
	flat_load_dwordx4 v[25:28], v[5:6]
	v_xor_b32_e32 v4, v4, v34
	v_xor_b32_e32 v4, v4, v10
	v_xor_b32_e32 v13, v15, v13
	v_xor_b32_e32 v4, v4, v35
	v_xor_b32_e32 v13, v13, v29
	v_xor_b32_e32 v10, v4, v16
	v_xor_b32_e32 v4, v9, v38
	v_xor_b32_e32 v11, v13, v11
	v_xor_b32_e32 v3, v4, v3
	v_xor_b32_e32 v13, v33, v32
	v_xor_b32_e32 v42, v17, v16
	v_xor_b32_e32 v12, v13, v12
	v_xor_b32_e32 v3, v3, v7
	v_xor_b32_e32 v8, v8, v30
	s_add_u32 s6, s6, 16
	v_xor_b32_e32 v11, v11, v30
	v_xor_b32_e32 v12, v12, v31
	v_xor_b32_e32 v7, v3, v39
	s_addc_u32 s7, s7, 0
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_xor_b32_sdwa v24, v25, v18 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:BYTE_0
	v_xor_b32_sdwa v23, v25, v19 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:BYTE_0
	v_xor_b32_sdwa v19, v25, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v17, v26, v14 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_lshrrev_b32_e32 v25, 8, v25
	v_xor_b32_sdwa v22, v26, v22 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:BYTE_0
	v_xor_b32_sdwa v20, v26, v20 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:BYTE_0
	v_lshrrev_b32_e32 v26, 8, v26
	v_xor_b32_sdwa v21, v27, v36 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:BYTE_0
	v_xor_b32_sdwa v6, v27, v37 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:BYTE_0
	v_xor_b32_sdwa v5, v27, v42 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_lshrrev_b32_e32 v27, 8, v27
	v_xor_b32_sdwa v18, v28, v40 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:BYTE_0
	v_xor_b32_sdwa v8, v28, v43 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:BYTE_0
	v_xor_b32_sdwa v14, v28, v41 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_lshrrev_b32_e32 v28, 8, v28
	v_xor_b32_sdwa v4, v25, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v3, v26, v12 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v9, v27, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v7, v28, v7 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
BB0_2:                                  ; =>This Inner Loop Header: Depth=1
	v_and_b32_e32 v10, s4, v24
	v_add_u32_e32 v12, vcc, s2, v10
	v_mov_b32_e32 v11, s3
	v_and_b32_e32 v10, s4, v23
	v_addc_u32_e32 v13, vcc, 0, v11, vcc
	v_add_u32_e32 v15, vcc, s2, v10
	v_addc_u32_e32 v16, vcc, 0, v11, vcc
	v_and_b32_e32 v4, s4, v4
	v_add_u32_e32 v10, vcc, s2, v4
	v_addc_u32_e32 v11, vcc, 0, v11, vcc
	v_and_b32_e32 v4, s4, v22
	v_add_u32_e32 v22, vcc, s2, v4
	v_mov_b32_e32 v23, s3
	v_addc_u32_e32 v23, vcc, 0, v23, vcc
	v_and_b32_e32 v3, s4, v3
	v_add_u32_e32 v24, vcc, s2, v3
	v_mov_b32_e32 v4, s3
	v_addc_u32_e32 v25, vcc, 0, v4, vcc
	v_and_b32_e32 v3, s4, v17
	v_add_u32_e32 v26, vcc, s2, v3
	v_addc_u32_e32 v27, vcc, 0, v4, vcc
	v_and_b32_e32 v3, s4, v5
	v_add_u32_e32 v3, vcc, s2, v3
	v_addc_u32_e32 v4, vcc, 0, v4, vcc
	flat_load_ubyte v3, v[3:4]
	flat_load_ubyte v5, v[24:25]
	flat_load_ubyte v22, v[22:23]
	flat_load_ubyte v4, v[26:27]
	flat_load_ubyte v17, v[10:11]
	v_and_b32_e32 v10, s4, v19
	v_add_u32_e32 v23, vcc, s2, v10
	v_mov_b32_e32 v11, s3
	v_addc_u32_e32 v24, vcc, 0, v11, vcc
	v_and_b32_e32 v10, s4, v20
	v_add_u32_e32 v19, vcc, s2, v10
	v_addc_u32_e32 v20, vcc, 0, v11, vcc
	v_and_b32_e32 v10, s4, v21
	v_add_u32_e32 v25, vcc, s2, v10
	v_addc_u32_e32 v26, vcc, 0, v11, vcc
	v_and_b32_e32 v9, s4, v9
	v_mov_b32_e32 v10, s3
	v_add_u32_e32 v9, vcc, s2, v9
	v_and_b32_e32 v11, s4, v18
	v_addc_u32_e32 v10, vcc, 0, v10, vcc
	v_add_u32_e32 v27, vcc, s2, v11
	v_mov_b32_e32 v18, s3
	v_addc_u32_e32 v28, vcc, 0, v18, vcc
	v_and_b32_e32 v7, s4, v7
	v_add_u32_e32 v29, vcc, s2, v7
	v_mov_b32_e32 v11, s3
	v_addc_u32_e32 v30, vcc, 0, v11, vcc
	v_and_b32_e32 v7, s4, v14
	v_add_u32_e32 v31, vcc, s2, v7
	v_addc_u32_e32 v32, vcc, 0, v11, vcc
	v_and_b32_e32 v6, s4, v6
	flat_load_ubyte v14, v[29:30]
	flat_load_ubyte v7, v[27:28]
	flat_load_ubyte v11, v[31:32]
	flat_load_ubyte v18, v[9:10]
	flat_load_ubyte v10, v[25:26]
	v_add_u32_e32 v25, vcc, s2, v6
	v_mov_b32_e32 v9, s3
	v_addc_u32_e32 v26, vcc, 0, v9, vcc
	v_and_b32_e32 v8, s4, v8
	flat_load_ubyte v13, v[12:13]
	flat_load_ubyte v6, v[23:24]
	flat_load_ubyte v9, v[15:16]
	flat_load_ubyte v15, v[19:20]
	flat_load_ubyte v12, v[25:26]
	v_mov_b32_e32 v16, s3
	v_add_u32_e32 v19, vcc, s2, v8
	v_addc_u32_e32 v20, vcc, 0, v16, vcc
	flat_load_ubyte v16, v[19:20]
	s_cmpk_lg_i32 s6, 0xd0
	s_cbranch_scc1 BB0_1
; BB#3:
	s_add_u32 s0, s0, 0xe0
	s_addc_u32 s1, s1, 0
	v_mov_b32_e32 v20, s1
	v_mov_b32_e32 v19, s0
	flat_load_dwordx4 v[23:26], v[19:20]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_xor_b32_sdwa v2, v23, v13 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:DWORD
	v_xor_b32_sdwa v8, v23, v15 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:DWORD
	v_xor_b32_sdwa v15, v25, v16 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:DWORD
	v_lshrrev_b32_e32 v21, 8, v26
	v_xor_b32_sdwa v13, v24, v22 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:DWORD
	v_xor_b32_sdwa v12, v24, v12 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:DWORD
	v_xor_b32_sdwa v7, v26, v7 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:DWORD
	v_xor_b32_sdwa v9, v26, v9 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:DWORD
	v_lshrrev_b32_e32 v16, 8, v23
	v_lshrrev_b32_e32 v19, 8, v24
	v_lshrrev_b32_e32 v20, 8, v25
	v_xor_b32_sdwa v10, v25, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:DWORD
	v_or_b32_sdwa v2, v2, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v8, v13, v12 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v7, v7, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_xor_b32_sdwa v11, v23, v11 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:DWORD
	v_xor_b32_e32 v9, v16, v18
	v_xor_b32_sdwa v6, v24, v6 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:DWORD
	v_xor_b32_e32 v12, v19, v14
	v_xor_b32_sdwa v4, v25, v4 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:DWORD
	v_xor_b32_e32 v13, v20, v17
	v_xor_b32_sdwa v3, v26, v3 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:DWORD
	v_xor_b32_e32 v5, v21, v5
	v_or_b32_sdwa v3, v5, v3 dst_sel:WORD_1 dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v9, v9, v11 dst_sel:WORD_1 dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v10, v10, v15 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v4, v13, v4 dst_sel:WORD_1 dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v6, v12, v6 dst_sel:WORD_1 dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v5, v7, v3 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_0 src1_sel:DWORD
	v_or_b32_sdwa v4, v10, v4 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_0 src1_sel:DWORD
	v_or_b32_sdwa v3, v8, v6 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_0 src1_sel:DWORD
	v_or_b32_sdwa v2, v2, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_0 src1_sel:DWORD
	flat_store_dwordx4 v[0:1], v[2:5]
	s_endpgm
.Lfunc_end0:
	.size	Encrypt, .Lfunc_end0-Encrypt
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 1928
; NumSgprs: 27
; NumVgprs: 44
; ScratchSize: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 3
; VGPRBlocks: 10
; NumSGPRsForWavesPerEU: 27
; NumVGPRsForWavesPerEU: 44
; ReservedVGPRFirst: 0
; ReservedVGPRCount: 0
; COMPUTE_PGM_RSRC2:USER_SGPR: 8
; COMPUTE_PGM_RSRC2:TRAP_HANDLER: 1
; COMPUTE_PGM_RSRC2:TGID_X_EN: 1
; COMPUTE_PGM_RSRC2:TGID_Y_EN: 0
; COMPUTE_PGM_RSRC2:TGID_Z_EN: 0
; COMPUTE_PGM_RSRC2:TIDIG_COMP_CNT: 0
	.type	s,@object               ; @s
	.section	.rodata,#alloc
	.globl	s
s:
	.ascii	"c|w{\362ko\3050\001g+\376\327\253v\312\202\311}\372YG\360\255\324\242\257\234\244r\300\267\375\223&6?\367\3144\245\345\361q\3301\025\004\307#\303\030\226\005\232\007\022\200\342\353'\262u\t\203,\032\033nZ\240R;\326\263)\343/\204S\321\000\355 \374\261[j\313\2769JLX\317\320\357\252\373CM3\205E\371\002\177P<\237\250Q\243@\217\222\2358\365\274\266\332!\020\377\363\322\315\f\023\354_\227D\027\304\247~=d]\031s`\201O\334\"*\220\210F\356\270\024\336^\013\333\3402:\nI\006$\\\302\323\254b\221\225\344y\347\3107m\215\325N\251lV\364\352ez\256\b\272x%.\034\246\264\306\350\335t\037K\275\213\212p>\265fH\003\366\016a5W\271\206\301\035\236\341\370\230\021i\331\216\224\233\036\207\351\316U(\337\214\241\211\r\277\346BhA\231-\017\260T\273\026"
	.size	s, 256


	.ident	"clang version 4.0 "
	.section	".note.GNU-stack"
	.amd_amdgpu_isa "amdgcn-amd-amdhsa-opencl-gfx803"
	.amd_amdgpu_hsa_metadata
---
Version:         [ 1, 0 ]
Kernels:         
  - Name:            Encrypt
    SymbolName:      'Encrypt@kd'
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Name:            input
        TypeName:        'uchar*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       U8
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            expanded_key
        TypeName:        'uint*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       U32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Size:            8
        Align:           8
        ValueKind:       HiddenGlobalOffsetX
        ValueType:       I64
      - Size:            8
        Align:           8
        ValueKind:       HiddenGlobalOffsetY
        ValueType:       I64
      - Size:            8
        Align:           8
        ValueKind:       HiddenGlobalOffsetZ
        ValueType:       I64
    CodeProps:       
      KernargSegmentSize: 48
      GroupSegmentFixedSize: 0
      PrivateSegmentFixedSize: 0
      KernargSegmentAlign: 8
      WavefrontSize:   64
      NumSGPRs:        27
      NumVGPRs:        44
      MaxFlatWorkGroupSize: 256
...

	.end_amd_amdgpu_hsa_metadata
