	.text
	.hsa_code_object_version 2,1
	.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"
	.globl	matrixTranspose         ; -- Begin function matrixTranspose
	.p2align	8
	.type	matrixTranspose,@function
	.amdgpu_hsa_kernel matrixTranspose
matrixTranspose:                        ; @matrixTranspose
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
		granulated_workitem_vgpr_count = 6
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
		enable_sgpr_workgroup_id_y = 1
		enable_sgpr_workgroup_id_z = 0
		enable_sgpr_workgroup_info = 0
		enable_vgpr_workitem_id = 1
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
		kernarg_segment_byte_size = 56
		workgroup_fbarrier_count = 0
		wavefront_sgpr_count = 17
		workitem_vgpr_count = 25
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
; %bb.0:
	s_load_dword s0, s[4:5], 0x4
	s_load_dword s14, s[4:5], 0xc
	s_load_dwordx2 s[10:11], s[6:7], 0x0
	s_load_dwordx2 s[12:13], s[6:7], 0x8
	s_load_dword s4, s[6:7], 0x10
	s_waitcnt lgkmcnt(0)
	s_and_b32 s2, s0, 0xffff
	v_cvt_f32_u32_e32 v2, s2
	s_mul_i32 s0, s8, s2
	s_sub_i32 s0, s14, s0
	s_min_u32 s6, s0, s2
	v_rcp_iflag_f32_e32 v2, v2
	v_mul_u32_u24_e32 v7, s6, v1
	v_lshlrev_b32_e32 v7, 2, v7
	s_mul_i32 s5, s6, s8
	v_mul_f32_e32 v2, 0x4f800000, v2
	v_cvt_u32_f32_e32 v2, v2
	v_add_u32_e32 v3, vcc, s5, v1
	s_add_i32 s9, s9, s8
	v_mul_u32_u24_e32 v14, s6, v0
	v_mul_lo_i32 v5, v2, s2
	v_mul_hi_u32 v6, v2, s2
	v_lshlrev_b32_e32 v14, 2, v14
	s_lshl_b32 s8, s6, 4
	v_sub_u32_e32 v8, vcc, 0, v5
	v_cmp_eq_u32_e64 s[0:1], 0, v6
	v_cndmask_b32_e64 v5, v5, v8, s[0:1]
	v_mul_hi_u32 v5, v5, v2
	v_add_u32_e32 v6, vcc, v0, v7
	v_lshlrev_b32_e32 v6, 4, v6
	s_lshl_b32 s7, s14, 2
	v_add_u32_e32 v7, vcc, v5, v2
	v_subrev_u32_e32 v2, vcc, v5, v2
	v_cndmask_b32_e64 v2, v2, v7, s[0:1]
	v_mul_hi_u32 v2, v2, s14
	v_add_u32_e32 v5, vcc, s4, v6
	v_mul_lo_i32 v3, v3, s7
	v_mov_b32_e32 v4, s13
	v_mul_lo_i32 v6, v2, s2
	v_add_u32_e32 v7, vcc, 1, v2
	v_add_u32_e32 v8, vcc, -1, v2
	v_mov_b32_e32 v20, 0
	v_sub_u32_e32 v9, vcc, s14, v6
	v_cmp_ge_u32_e32 vcc, s14, v6
	v_cndmask_b32_e64 v6, 0, -1, vcc
	v_cmp_le_u32_e32 vcc, s2, v9
	v_cndmask_b32_e64 v9, 0, -1, vcc
	v_and_b32_e32 v9, v9, v6
	v_cmp_eq_u32_e32 vcc, 0, v9
	v_cndmask_b32_e32 v2, v7, v2, vcc
	v_cmp_eq_u32_e32 vcc, 0, v6
	v_cndmask_b32_e32 v2, v2, v8, vcc
	v_mul_lo_i32 v6, v2, s2
	v_add_u32_e32 v14, vcc, v1, v14
	v_lshlrev_b32_e32 v14, 4, v14
	v_add_u32_e32 v9, vcc, s8, v5
	v_cmp_gt_u32_e64 s[0:1], s14, v6
	v_addc_u32_e64 v6, vcc, 0, v2, s[0:1]
	v_cvt_f32_u32_e32 v8, v6
	v_add_u32_e32 v14, vcc, s4, v14
	v_add_u32_e32 v11, vcc, s8, v9
	v_add_u32_e32 v13, vcc, s8, v11
	v_rcp_iflag_f32_e32 v8, v8
	s_mov_b32 m0, -1
	v_mov_b32_e32 v7, s13
	v_mov_b32_e32 v10, s13
	v_mul_f32_e32 v8, 0x4f800000, v8
	v_cvt_u32_f32_e32 v8, v8
	v_mov_b32_e32 v12, s13
	v_mov_b32_e32 v23, s11
	v_mov_b32_e32 v24, s11
	v_mul_lo_i32 v15, v8, v6
	v_mul_hi_u32 v16, v8, v6
	v_sub_u32_e32 v17, vcc, 0, v15
	v_cmp_eq_u32_e64 s[2:3], 0, v16
	v_cndmask_b32_e64 v15, v15, v17, s[2:3]
	v_mul_hi_u32 v15, v15, v8
	v_add_u32_e32 v17, vcc, s5, v0
	v_add_u32_e32 v16, vcc, v15, v8
	v_subrev_u32_e32 v8, vcc, v15, v8
	v_cndmask_b32_e64 v8, v8, v16, s[2:3]
	v_mul_hi_u32 v8, v8, s9
	v_add_u32_e32 v15, vcc, s8, v14
	v_add_u32_e32 v16, vcc, s8, v15
	v_mul_lo_i32 v8, v8, v6
	v_cmp_ge_u32_e64 s[2:3], s9, v8
	v_sub_u32_e32 v8, vcc, s9, v8
	v_cmp_ge_u32_e64 s[4:5], v8, v6
	v_cndmask_b32_e64 v18, 0, -1, s[4:5]
	v_cndmask_b32_e64 v19, 0, -1, s[2:3]
	v_subrev_u32_e32 v6, vcc, v6, v8
	v_addc_u32_e64 v2, vcc, v8, v2, s[0:1]
	v_and_b32_e32 v18, v18, v19
	v_cmp_eq_u32_e32 vcc, 0, v18
	v_cndmask_b32_e32 v6, v6, v8, vcc
	v_cmp_eq_u32_e32 vcc, 0, v19
	v_cndmask_b32_e32 v2, v6, v2, vcc
	v_mul_lo_i32 v6, s6, v2
	v_add_u32_e32 v18, vcc, s8, v16
	v_add_u32_e32 v0, vcc, v0, v6
	v_add_u32_e32 v2, vcc, v3, v0
	v_add_u32_e32 v0, vcc, v1, v6
	v_mul_lo_i32 v0, v0, s7
	v_ashrrev_i32_e32 v3, 31, v2
	v_add_u32_e32 v19, vcc, s14, v2
	v_lshlrev_b64 v[2:3], 4, v[2:3]
	v_add_u32_e32 v21, vcc, v17, v0
	v_add_u32_e32 v0, vcc, s12, v2
	v_addc_u32_e32 v1, vcc, v4, v3, vcc
	flat_load_dwordx4 v[0:3], v[0:1]
	v_ashrrev_i32_e32 v22, 31, v21
	s_waitcnt vmcnt(0) lgkmcnt(0)
	ds_write2_b64 v5, v[0:1], v[2:3] offset1:1
	v_lshlrev_b64 v[0:1], 4, v[19:20]
	v_add_u32_e32 v0, vcc, s12, v0
	v_addc_u32_e32 v1, vcc, v7, v1, vcc
	flat_load_dwordx4 v[0:3], v[0:1]
	v_add_u32_e32 v19, vcc, s14, v19
	s_waitcnt vmcnt(0) lgkmcnt(0)
	ds_write2_b64 v9, v[0:1], v[2:3] offset1:1
	v_lshlrev_b64 v[0:1], 4, v[19:20]
	v_add_u32_e32 v0, vcc, s12, v0
	v_addc_u32_e32 v1, vcc, v10, v1, vcc
	flat_load_dwordx4 v[0:3], v[0:1]
	v_add_u32_e32 v19, vcc, s14, v19
	v_lshlrev_b64 v[4:5], 4, v[19:20]
	v_lshlrev_b64 v[8:9], 4, v[21:22]
	s_waitcnt vmcnt(0) lgkmcnt(0)
	ds_write2_b64 v11, v[0:1], v[2:3] offset1:1
	v_add_u32_e32 v0, vcc, s12, v4
	v_addc_u32_e32 v1, vcc, v12, v5, vcc
	flat_load_dwordx4 v[0:3], v[0:1]
	v_add_u32_e32 v22, vcc, s10, v8
	v_addc_u32_e32 v23, vcc, v23, v9, vcc
	v_add_u32_e32 v19, vcc, s14, v21
	s_waitcnt vmcnt(0) lgkmcnt(0)
	ds_write2_b64 v13, v[0:1], v[2:3] offset1:1
	s_waitcnt lgkmcnt(0)
	s_barrier
	ds_read2_b64 v[0:3], v14 offset1:1
	ds_read2_b64 v[4:7], v15 offset1:1
	ds_read2_b64 v[11:14], v16 offset1:1
	ds_read2_b64 v[15:18], v18 offset1:1
	s_waitcnt lgkmcnt(3)
	v_mov_b32_e32 v8, v0
	s_waitcnt lgkmcnt(2)
	v_mov_b32_e32 v9, v4
	s_waitcnt lgkmcnt(1)
	v_mov_b32_e32 v10, v11
	s_waitcnt lgkmcnt(0)
	v_mov_b32_e32 v11, v15
	flat_store_dwordx4 v[22:23], v[8:11]
	v_mov_b32_e32 v4, s11
	v_lshlrev_b64 v[8:9], 4, v[19:20]
	v_add_u32_e32 v21, vcc, s10, v8
	v_addc_u32_e32 v22, vcc, v24, v9, vcc
	v_add_u32_e32 v19, vcc, s14, v19
	v_mov_b32_e32 v8, v1
	v_lshlrev_b64 v[0:1], 4, v[19:20]
	v_add_u32_e32 v0, vcc, s10, v0
	v_mov_b32_e32 v11, v16
	v_mov_b32_e32 v9, v5
	v_mov_b32_e32 v10, v12
	flat_store_dwordx4 v[21:22], v[8:11]
	v_addc_u32_e32 v1, vcc, v4, v1, vcc
	v_mov_b32_e32 v8, v2
	v_mov_b32_e32 v11, v17
	v_mov_b32_e32 v9, v6
	v_mov_b32_e32 v10, v13
	v_add_u32_e32 v19, vcc, s14, v19
	flat_store_dwordx4 v[0:1], v[8:11]
	v_lshlrev_b64 v[0:1], 4, v[19:20]
	v_mov_b32_e32 v2, s11
	v_add_u32_e32 v0, vcc, s10, v0
	v_addc_u32_e32 v1, vcc, v2, v1, vcc
	v_mov_b32_e32 v15, v3
	v_mov_b32_e32 v16, v7
	v_mov_b32_e32 v17, v14
	flat_store_dwordx4 v[0:1], v[15:18]
	s_endpgm
.Lfunc_end0:
	.size	matrixTranspose, .Lfunc_end0-matrixTranspose
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 964
; NumSgprs: 17
; NumVgprs: 25
; ScratchSize: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 2
; VGPRBlocks: 6
; NumSGPRsForWavesPerEU: 17
; NumVGPRsForWavesPerEU: 25
; ReservedVGPRFirst: 0
; ReservedVGPRCount: 0
; COMPUTE_PGM_RSRC2:USER_SGPR: 8
; COMPUTE_PGM_RSRC2:TRAP_HANDLER: 1
; COMPUTE_PGM_RSRC2:TGID_X_EN: 1
; COMPUTE_PGM_RSRC2:TGID_Y_EN: 1
; COMPUTE_PGM_RSRC2:TGID_Z_EN: 0
; COMPUTE_PGM_RSRC2:TIDIG_COMP_CNT: 1

	.ident	"clang version 4.0 "
	.section	".note.GNU-stack"
	.amd_amdgpu_isa "amdgcn-amd-amdhsa-amdgizcl-gfx803"
	.amd_amdgpu_hsa_metadata
---
Version:         [ 1, 0 ]
Kernels:         
  - Name:            matrixTranspose
    SymbolName:      'matrixTranspose@kd'
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Name:            output
        TypeName:        'float4*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            input
        TypeName:        'float4*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            block
        TypeName:        'float4*'
        Size:            4
        Align:           4
        ValueKind:       DynamicSharedPointer
        ValueType:       F32
        PointeeAlign:    16
        AddrSpaceQual:   Local
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
      KernargSegmentSize: 56
      GroupSegmentFixedSize: 0
      PrivateSegmentFixedSize: 0
      KernargSegmentAlign: 8
      WavefrontSize:   64
      NumSGPRs:        17
      NumVGPRs:        25
      MaxFlatWorkGroupSize: 256
...

	.end_amd_amdgpu_hsa_metadata
