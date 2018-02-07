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
		granulated_workitem_vgpr_count = 11
		granulated_wavefront_sgpr_count = 2
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
		wavefront_sgpr_count = 22
		workitem_vgpr_count = 45
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
	v_add_i32_e32 v0, vcc, s8, v0
	s_load_dwordx4 s[8:11], s[0:1], 0x0
	v_add_i32_e32 v0, vcc, s5, v0
	v_lshlrev_b32_e32 v0, 4, v0
	v_ashrrev_i32_e32 v1, 31, v0
	v_add_i32_e32 v0, vcc, s2, v0
	v_mov_b32_e32 v2, s3
	s_waitcnt lgkmcnt(0)
	s_lshr_b32 s4, s8, 16
	s_lshr_b32 s5, s8, 8
	s_lshr_b32 s6, s9, 24
	s_lshr_b32 s7, s9, 16
	s_lshr_b32 s13, s9, 8
	s_lshr_b32 s14, s10, 24
	s_lshr_b32 s15, s10, 16
	s_lshr_b32 s16, s10, 8
	s_lshr_b32 s17, s11, 24
	s_lshr_b32 s18, s11, 16
	s_lshr_b32 s19, s11, 8
	v_addc_u32_e32 v1, vcc, v2, v1, vcc
	v_mov_b32_e32 v6, s6
	v_mov_b32_e32 v12, s7
	s_getpc_b64 s[6:7]
	s_add_u32 s6, s6, s@gotpcrel32@lo+4
	s_addc_u32 s7, s7, s@gotpcrel32@hi+4
	flat_load_dwordx4 v[8:11], v[0:1]
	s_load_dwordx2 s[6:7], s[6:7], 0x0
	v_mov_b32_e32 v4, s5
	s_lshr_b32 s5, s8, 24
	v_mov_b32_e32 v3, s4
	v_mov_b32_e32 v5, s8
	v_mov_b32_e32 v14, s9
	v_mov_b32_e32 v15, s14
	v_mov_b32_e32 v7, s5
	v_mov_b32_e32 v13, s13
	v_mov_b32_e32 v16, s15
	v_mov_b32_e32 v17, s16
	v_mov_b32_e32 v18, s10
	v_mov_b32_e32 v26, s17
	v_mov_b32_e32 v27, s18
	v_mov_b32_e32 v28, s19
	v_mov_b32_e32 v29, s11
	s_mov_b64 s[2:3], 0
	s_movk_i32 s4, 0x80
	s_waitcnt lgkmcnt(0)
	v_mov_b32_e32 v2, s6
	s_waitcnt vmcnt(0)
	v_xor_b32_sdwa v25, v7, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v24, v3, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_1
	v_xor_b32_sdwa v7, v4, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_2
	v_xor_b32_sdwa v23, v5, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_3
	v_xor_b32_sdwa v22, v6, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v21, v12, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_1
	v_xor_b32_sdwa v19, v15, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v15, v16, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_1
	v_xor_b32_sdwa v4, v17, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_2
	v_xor_b32_sdwa v12, v18, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_3
	v_xor_b32_sdwa v5, v13, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_2
	v_xor_b32_sdwa v20, v14, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_3
	v_xor_b32_sdwa v18, v26, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v17, v27, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_1
	v_xor_b32_sdwa v6, v28, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_2
	v_xor_b32_sdwa v16, v29, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_3
	v_mov_b32_e32 v3, s12
	s_branch BB0_2
BB0_1:                                  ;   in Loop: Header=BB0_2 Depth=1
	s_waitcnt vmcnt(5) lgkmcnt(5)
	v_and_b32_sdwa v20, v3, v13 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v22, s4, v20
	v_lshlrev_b32_e32 v20, 1, v20
	s_waitcnt vmcnt(2) lgkmcnt(2)
	v_and_b32_sdwa v21, v3, v14 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cmp_eq_u32_e32 vcc, 0, v22
	v_xor_b32_e32 v23, 27, v20
	v_and_b32_e32 v22, s4, v21
	v_lshlrev_b32_e32 v21, 1, v21
	v_cndmask_b32_e32 v20, v23, v20, vcc
	v_cmp_eq_u32_e32 vcc, 0, v22
	v_xor_b32_e32 v23, 27, v21
	v_and_b32_sdwa v22, v3, v18 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v21, v23, v21, vcc
	v_and_b32_e32 v23, s4, v22
	v_lshlrev_b32_e32 v22, 1, v22
	v_xor_b32_e32 v24, 27, v22
	v_cmp_eq_u32_e32 vcc, 0, v23
	v_cndmask_b32_e32 v30, v24, v22, vcc
	v_and_b32_sdwa v22, v3, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v23, s4, v22
	v_lshlrev_b32_e32 v22, 1, v22
	v_xor_b32_e32 v24, 27, v22
	v_cmp_eq_u32_e32 vcc, 0, v23
	v_cndmask_b32_e32 v31, v24, v22, vcc
	v_xor_b32_e32 v22, v14, v20
	v_xor_b32_e32 v20, v13, v20
	v_xor_b32_e32 v22, v22, v21
	v_xor_b32_e32 v21, v13, v21
	v_xor_b32_e32 v20, v20, v14
	v_xor_b32_e32 v22, v22, v18
	v_xor_b32_e32 v21, v21, v18
	v_xor_b32_e32 v18, v20, v18
	v_and_b32_sdwa v20, v3, v19 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v23, s4, v20
	v_lshlrev_b32_e32 v20, 1, v20
	v_cmp_eq_u32_e32 vcc, 0, v23
	v_xor_b32_e32 v24, 27, v20
	s_waitcnt vmcnt(1) lgkmcnt(1)
	v_and_b32_sdwa v23, v3, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v20, v24, v20, vcc
	v_and_b32_e32 v24, s4, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_xor_b32_e32 v25, 27, v23
	v_and_b32_sdwa v24, v3, v16 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v23, v25, v23, vcc
	v_and_b32_e32 v25, s4, v24
	v_lshlrev_b32_e32 v24, 1, v24
	v_xor_b32_e32 v26, 27, v24
	v_cmp_eq_u32_e32 vcc, 0, v25
	v_cndmask_b32_e32 v32, v26, v24, vcc
	v_and_b32_sdwa v24, v3, v17 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v25, s4, v24
	v_lshlrev_b32_e32 v24, 1, v24
	v_xor_b32_e32 v26, 27, v24
	v_cmp_eq_u32_e32 vcc, 0, v25
	v_cndmask_b32_e32 v33, v26, v24, vcc
	v_xor_b32_e32 v34, v19, v17
	v_xor_b32_e32 v17, v17, v20
	v_xor_b32_e32 v19, v19, v33
	v_xor_b32_e32 v19, v19, v20
	v_xor_b32_e32 v17, v17, v10
	v_xor_b32_e32 v17, v17, v23
	v_xor_b32_e32 v20, v34, v23
	v_xor_b32_e32 v19, v19, v10
	v_xor_b32_e32 v20, v20, v16
	v_xor_b32_e32 v17, v17, v16
	v_xor_b32_e32 v19, v19, v16
	v_and_b32_sdwa v16, v3, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v23, s4, v16
	v_lshlrev_b32_e32 v16, 1, v16
	v_cmp_eq_u32_e32 vcc, 0, v23
	v_xor_b32_e32 v24, 27, v16
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_and_b32_sdwa v23, v3, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v16, v24, v16, vcc
	v_and_b32_e32 v24, s4, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_xor_b32_e32 v25, 27, v23
	v_and_b32_sdwa v24, v3, v15 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v23, v25, v23, vcc
	v_and_b32_e32 v25, s4, v24
	v_lshlrev_b32_e32 v24, 1, v24
	v_xor_b32_e32 v26, 27, v24
	v_cmp_eq_u32_e32 vcc, 0, v25
	v_cndmask_b32_e32 v35, v26, v24, vcc
	v_and_b32_sdwa v24, v3, v5 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v25, s4, v24
	v_lshlrev_b32_e32 v24, 1, v24
	v_xor_b32_e32 v26, 27, v24
	v_cmp_eq_u32_e32 vcc, 0, v25
	v_cndmask_b32_e32 v36, v26, v24, vcc
	v_xor_b32_e32 v24, v5, v15
	v_xor_b32_e32 v24, v24, v16
	v_xor_b32_e32 v24, v24, v9
	v_xor_b32_e32 v37, v24, v23
	v_xor_b32_e32 v24, v15, v35
	v_xor_b32_e32 v15, v8, v15
	v_xor_b32_e32 v24, v24, v5
	v_xor_b32_e32 v15, v15, v36
	v_xor_b32_e32 v15, v15, v16
	v_xor_b32_e32 v24, v24, v8
	v_and_b32_sdwa v16, v3, v6 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_e32 v38, v24, v23
	v_and_b32_e32 v23, s4, v16
	v_lshlrev_b32_e32 v16, 1, v16
	v_cmp_eq_u32_e32 vcc, 0, v23
	v_xor_b32_e32 v24, 27, v16
	v_and_b32_sdwa v23, v3, v7 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v16, v24, v16, vcc
	v_and_b32_e32 v24, s4, v23
	v_lshlrev_b32_e32 v23, 1, v23
	v_cmp_eq_u32_e32 vcc, 0, v24
	v_xor_b32_e32 v25, 27, v23
	v_and_b32_sdwa v24, v3, v12 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_cndmask_b32_e32 v23, v25, v23, vcc
	v_and_b32_e32 v25, s4, v24
	v_lshlrev_b32_e32 v24, 1, v24
	v_xor_b32_e32 v26, 27, v24
	v_cmp_eq_u32_e32 vcc, 0, v25
	v_cndmask_b32_e32 v39, v26, v24, vcc
	v_and_b32_sdwa v24, v3, v4 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_and_b32_e32 v25, s4, v24
	v_lshlrev_b32_e32 v24, 1, v24
	v_xor_b32_e32 v26, 27, v24
	v_cmp_eq_u32_e32 vcc, 0, v25
	v_cndmask_b32_e32 v40, v26, v24, vcc
	v_xor_b32_e32 v24, v7, v23
	v_xor_b32_e32 v24, v24, v12
	s_add_u32 s5, s0, s2
	v_xor_b32_e32 v23, v12, v23
	v_xor_b32_e32 v12, v12, v7
	v_xor_b32_e32 v12, v12, v6
	s_addc_u32 s6, s1, s3
	s_add_u32 s5, s5, 16
	v_xor_b32_e32 v24, v24, v4
	v_xor_b32_e32 v12, v12, v40
	v_xor_b32_e32 v43, v15, v9
	v_xor_b32_e32 v15, v23, v39
	s_addc_u32 s6, s6, 0
	v_xor_b32_e32 v42, v12, v16
	v_xor_b32_e32 v41, v24, v16
	v_xor_b32_e32 v16, v21, v30
	v_xor_b32_e32 v15, v15, v4
	v_xor_b32_e32 v21, v16, v11
	v_xor_b32_e32 v44, v15, v6
	v_mov_b32_e32 v15, s5
	v_mov_b32_e32 v16, s6
	flat_load_dwordx4 v[26:29], v[15:16]
	v_xor_b32_e32 v5, v5, v35
	v_xor_b32_e32 v5, v5, v8
	v_xor_b32_e32 v13, v14, v13
	v_xor_b32_e32 v5, v5, v36
	v_xor_b32_e32 v8, v5, v9
	v_xor_b32_e32 v5, v7, v39
	v_xor_b32_e32 v13, v13, v30
	v_xor_b32_e32 v12, v22, v11
	v_xor_b32_e32 v11, v13, v11
	v_xor_b32_e32 v4, v5, v4
	v_xor_b32_e32 v13, v34, v33
	v_xor_b32_e32 v10, v13, v10
	v_xor_b32_e32 v4, v4, v6
	v_xor_b32_e32 v18, v18, v31
	v_xor_b32_e32 v20, v20, v32
	s_add_u32 s2, s2, 16
	v_xor_b32_e32 v11, v11, v31
	v_xor_b32_e32 v10, v10, v32
	v_xor_b32_e32 v6, v4, v40
	s_addc_u32 s3, s3, 0
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_xor_b32_sdwa v25, v26, v12 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:BYTE_0
	v_xor_b32_sdwa v24, v26, v21 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:BYTE_0
	v_xor_b32_sdwa v23, v26, v18 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v22, v27, v17 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:BYTE_0
	v_xor_b32_sdwa v21, v27, v20 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:BYTE_0
	v_xor_b32_sdwa v20, v27, v19 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_lshrrev_b32_e32 v26, 8, v26
	v_lshrrev_b32_e32 v27, 8, v27
	v_xor_b32_sdwa v19, v28, v37 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:BYTE_0
	v_xor_b32_sdwa v15, v28, v38 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:BYTE_0
	v_xor_b32_sdwa v12, v28, v43 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_lshrrev_b32_e32 v28, 8, v28
	v_xor_b32_sdwa v18, v29, v41 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:BYTE_0
	v_xor_b32_sdwa v17, v29, v44 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:BYTE_0
	v_xor_b32_sdwa v16, v29, v42 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_lshrrev_b32_e32 v29, 8, v29
	v_xor_b32_sdwa v7, v26, v11 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v5, v27, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v4, v28, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_xor_b32_sdwa v6, v29, v6 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
BB0_2:                                  ; =>This Inner Loop Header: Depth=1
	v_add_i32_sdwa v9, vcc, v2, v25 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_mov_b32_e32 v11, s7
	v_addc_u32_e32 v10, vcc, 0, v11, vcc
	v_add_i32_sdwa v24, vcc, v2, v24 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v25, vcc, 0, v11, vcc
	v_add_i32_sdwa v7, vcc, v2, v7 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v8, vcc, 0, v11, vcc
	v_add_i32_sdwa v26, vcc, v2, v23 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v27, vcc, 0, v11, vcc
	v_add_i32_sdwa v13, vcc, v2, v22 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v14, vcc, 0, v11, vcc
	v_add_i32_sdwa v21, vcc, v2, v21 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v22, vcc, 0, v11, vcc
	v_add_i32_sdwa v28, vcc, v2, v5 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v29, vcc, 0, v11, vcc
	v_add_i32_sdwa v30, vcc, v2, v20 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v31, vcc, 0, v11, vcc
	v_add_i32_sdwa v32, vcc, v2, v19 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v33, vcc, 0, v11, vcc
	v_add_i32_sdwa v34, vcc, v2, v15 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v35, vcc, 0, v11, vcc
	v_add_i32_sdwa v36, vcc, v2, v4 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v37, vcc, 0, v11, vcc
	v_add_i32_sdwa v4, vcc, v2, v12 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v5, vcc, 0, v11, vcc
	flat_load_ubyte v4, v[4:5]
	flat_load_ubyte v12, v[28:29]
	flat_load_ubyte v19, v[13:14]
	flat_load_ubyte v5, v[30:31]
	flat_load_ubyte v15, v[7:8]
	v_add_i32_sdwa v7, vcc, v2, v18 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v8, vcc, 0, v11, vcc
	v_add_i32_sdwa v28, vcc, v2, v17 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v29, vcc, 0, v11, vcc
	v_add_i32_sdwa v13, vcc, v2, v6 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v14, vcc, 0, v11, vcc
	v_add_i32_sdwa v17, vcc, v2, v16 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:BYTE_0
	v_addc_u32_e32 v18, vcc, 0, v11, vcc
	flat_load_ubyte v16, v[13:14]
	flat_load_ubyte v6, v[7:8]
	flat_load_ubyte v11, v[17:18]
	flat_load_ubyte v18, v[36:37]
	flat_load_ubyte v8, v[32:33]
	flat_load_ubyte v13, v[9:10]
	flat_load_ubyte v17, v[26:27]
	flat_load_ubyte v7, v[24:25]
	flat_load_ubyte v14, v[21:22]
	flat_load_ubyte v10, v[34:35]
	flat_load_ubyte v9, v[28:29]
	s_cmpk_lg_i32 s2, 0xd0
	s_cbranch_scc1 BB0_1
; BB#3:
	s_add_u32 s0, s0, 0xe0
	s_addc_u32 s1, s1, 0
	v_mov_b32_e32 v2, s0
	v_mov_b32_e32 v3, s1
	flat_load_dwordx4 v[20:23], v[2:3]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_xor_b32_sdwa v2, v20, v13 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:DWORD
	v_xor_b32_sdwa v3, v20, v14 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:DWORD
	v_xor_b32_sdwa v13, v21, v19 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:DWORD
	v_xor_b32_sdwa v14, v21, v17 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:DWORD
	v_lshrrev_b32_e32 v17, 8, v20
	v_xor_b32_sdwa v11, v20, v11 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:DWORD
	v_lshrrev_b32_e32 v19, 8, v21
	v_xor_b32_sdwa v10, v21, v10 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:DWORD
	v_lshrrev_b32_e32 v20, 8, v22
	v_lshrrev_b32_e32 v21, 8, v23
	v_xor_b32_sdwa v8, v22, v8 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:DWORD
	v_xor_b32_sdwa v9, v22, v9 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:DWORD
	v_xor_b32_sdwa v5, v22, v5 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:DWORD
	v_xor_b32_sdwa v6, v23, v6 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_3 src1_sel:DWORD
	v_xor_b32_sdwa v7, v23, v7 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:WORD_1 src1_sel:DWORD
	v_xor_b32_sdwa v4, v23, v4 dst_sel:BYTE_1 dst_unused:UNUSED_PAD src0_sel:DWORD src1_sel:DWORD
	v_xor_b32_e32 v17, v17, v18
	v_xor_b32_e32 v16, v19, v16
	v_xor_b32_e32 v15, v20, v15
	v_xor_b32_e32 v12, v21, v12
	v_or_b32_sdwa v6, v6, v7 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v7, v8, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v8, v13, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v2, v2, v3 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v3, v12, v4 dst_sel:WORD_1 dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v4, v15, v5 dst_sel:WORD_1 dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v10, v17, v11 dst_sel:WORD_1 dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v9, v16, v14 dst_sel:WORD_1 dst_unused:UNUSED_PAD src0_sel:BYTE_0 src1_sel:DWORD
	v_or_b32_sdwa v5, v6, v3 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_0 src1_sel:DWORD
	v_or_b32_sdwa v4, v7, v4 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_0 src1_sel:DWORD
	v_or_b32_sdwa v3, v8, v9 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_0 src1_sel:DWORD
	v_or_b32_sdwa v2, v2, v10 dst_sel:DWORD dst_unused:UNUSED_PAD src0_sel:WORD_0 src1_sel:DWORD
	flat_store_dwordx4 v[0:1], v[2:5]
	s_endpgm
.Lfunc_end0:
	.size	Encrypt, .Lfunc_end0-Encrypt
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 1880
; NumSgprs: 22
; NumVgprs: 45
; ScratchSize: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 2
; VGPRBlocks: 11
; NumSGPRsForWavesPerEU: 22
; NumVGPRsForWavesPerEU: 45
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
	.amdgpu_code_object_metadata
---
Version:         [ 1, 0 ]
Kernels:         
  - Name:            Encrypt
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       U8
        AccQual:         Default
        AddrSpaceQual:   Global
        Name:            input
        TypeName:        'uchar*'
      - Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       U32
        AccQual:         Default
        AddrSpaceQual:   Global
        Name:            expanded_key
        TypeName:        'uint*'
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
      WavefrontNumSGPRs: 22
      WorkitemNumVGPRs: 45
      KernargSegmentAlign: 4
      GroupSegmentAlign: 4
      PrivateSegmentAlign: 4
      WavefrontSize:   6
...
	.end_amdgpu_code_object_metadata
