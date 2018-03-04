	.text
	.hsa_code_object_version 2,1
	.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"
	.globl	simpleNonSeparableConvolution ; -- Begin function simpleNonSeparableConvolution
	.p2align	8
	.type	simpleNonSeparableConvolution,@function
	.amdgpu_hsa_kernel simpleNonSeparableConvolution
simpleNonSeparableConvolution:          ; @simpleNonSeparableConvolution
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
		granulated_workitem_vgpr_count = 3
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
		kernarg_segment_byte_size = 80
		workgroup_fbarrier_count = 0
		wavefront_sgpr_count = 20
		workitem_vgpr_count = 15
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
	s_load_dword s2, s[4:5], 0x4
	s_load_dwordx2 s[14:15], s[6:7], 0x18
	s_load_dwordx2 s[0:1], s[6:7], 0x30
	s_waitcnt lgkmcnt(0)
	s_and_b32 s1, s2, 0xffff
	v_cvt_f32_u32_e32 v1, s14
	s_mul_i32 s8, s8, s1
	v_add_i32_e32 v0, vcc, s8, v0
	v_add_i32_e32 v0, vcc, s0, v0
	v_rcp_iflag_f32_e32 v1, v1
	v_mul_f32_e32 v1, 0x4f800000, v1
	v_cvt_u32_f32_e32 v1, v1
	v_mul_lo_i32 v2, v1, s14
	v_mul_hi_u32 v3, v1, s14
	v_sub_i32_e32 v4, vcc, 0, v2
	v_cmp_eq_u32_e64 s[0:1], 0, v3
	v_cndmask_b32_e64 v2, v2, v4, s[0:1]
	v_mul_hi_u32 v2, v2, v1
	v_add_i32_e32 v3, vcc, v2, v1
	v_subrev_i32_e32 v1, vcc, v2, v1
	v_cndmask_b32_e64 v1, v1, v3, s[0:1]
	v_mul_hi_u32 v1, v1, v0
	v_mul_lo_i32 v2, v1, s14
	v_add_i32_e32 v3, vcc, 1, v1
	v_cmp_ge_u32_e32 vcc, v0, v2
	v_cndmask_b32_e64 v4, 0, -1, vcc
	v_subrev_i32_e32 v2, vcc, v2, v0
	v_cmp_le_u32_e32 vcc, s14, v2
	v_cndmask_b32_e64 v2, 0, -1, vcc
	v_and_b32_e32 v2, v2, v4
	v_cmp_eq_u32_e32 vcc, 0, v2
	v_cndmask_b32_e32 v2, v3, v1, vcc
	v_add_i32_e32 v1, vcc, -1, v1
	v_cmp_eq_u32_e32 vcc, 0, v4
	v_cndmask_b32_e32 v1, v2, v1, vcc
	v_cmp_gt_u32_e32 vcc, s15, v1
	s_and_saveexec_b64 s[0:1], vcc
	; mask branch BB0_9
	s_cbranch_execz BB0_9
BB0_1:
	s_load_dwordx2 s[2:3], s[6:7], 0x10
	s_load_dwordx2 s[4:5], s[6:7], 0x20
	s_waitcnt lgkmcnt(0)
	v_add_i32_e32 v2, vcc, s5, v1
	v_cmp_lt_u32_e32 vcc, v1, v2
	v_mov_b32_e32 v2, 0
	s_and_saveexec_b64 s[8:9], vcc
	; mask branch BB0_8
	s_cbranch_execz BB0_8
BB0_2:
	v_mul_lo_i32 v2, v1, s14
	s_load_dwordx2 s[10:11], s[6:7], 0x0
	s_load_dwordx2 s[12:13], s[6:7], 0x8
	s_load_dword s6, s[6:7], 0x28
	v_subrev_i32_e32 v3, vcc, v2, v0
	v_add_i32_e32 v2, vcc, s4, v3
	v_cmp_lt_u32_e64 s[0:1], v3, v2
	v_mov_b32_e32 v2, 0
	v_mov_b32_e32 v4, 0
	v_mov_b32_e32 v5, 0
BB0_3:                                  ; =>This Loop Header: Depth=1
                                        ;     Child Loop BB0_5 Depth 2
	s_and_saveexec_b64 s[14:15], s[0:1]
	; mask branch BB0_6
	s_cbranch_execz BB0_6
BB0_4:                                  ;   in Loop: Header=BB0_3 Depth=1
	s_waitcnt lgkmcnt(0)
	v_mul_lo_i32 v6, v1, s6
	v_mov_b32_e32 v7, s4
	v_mov_b32_e32 v8, v4
	v_mov_b32_e32 v10, v3
BB0_5:                                  ;   Parent Loop BB0_3 Depth=1
                                        ; =>  This Inner Loop Header: Depth=2
	v_add_i32_e32 v11, vcc, v6, v10
	v_mov_b32_e32 v12, 0
	v_lshlrev_b64 v[13:14], 2, v[11:12]
	v_mov_b32_e32 v9, s11
	v_add_i32_e32 v13, vcc, s10, v13
	v_addc_u32_e32 v14, vcc, v9, v14, vcc
	flat_load_dword v13, v[13:14]
	v_mov_b32_e32 v9, v12
	v_lshlrev_b64 v[11:12], 2, v[8:9]
	v_add_i32_e32 v9, vcc, s12, v11
	v_mov_b32_e32 v14, s13
	v_addc_u32_e32 v11, vcc, v14, v12, vcc
	v_readfirstlane_b32 s16, v9
	v_readfirstlane_b32 s17, v11
	s_load_dword s7, s[16:17], 0x0
	v_add_i32_e32 v10, vcc, 1, v10
	v_add_i32_e32 v8, vcc, 1, v8
	v_add_i32_e32 v7, vcc, -1, v7
	v_cmp_eq_u32_e32 vcc, 0, v7
	s_and_b64 vcc, exec, vcc
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_cvt_f32_u32_e32 v9, v13
	v_mac_f32_e32 v2, s7, v9
	s_cbranch_vccz BB0_5
BB0_6:                                  ; %Flow
                                        ;   in Loop: Header=BB0_3 Depth=1
	s_or_b64 exec, exec, s[14:15]
; BB#7:                                 ; %.loopexit
                                        ;   in Loop: Header=BB0_3 Depth=1
	v_add_i32_e32 v1, vcc, 1, v1
	v_add_i32_e32 v5, vcc, 1, v5
	v_add_i32_e32 v4, vcc, s4, v4
	v_cmp_eq_u32_e32 vcc, s5, v5
	s_and_b64 vcc, exec, vcc
	s_cbranch_vccz BB0_3
BB0_8:                                  ; %Flow13
	s_or_b64 exec, exec, s[8:9]
	v_add_f32_e32 v1, 0.5, v2
	v_cvt_i32_f32_e32 v2, v1
	v_mov_b32_e32 v1, 0
	v_lshlrev_b64 v[0:1], 2, v[0:1]
	v_add_i32_e32 v0, vcc, s2, v0
	v_mov_b32_e32 v3, s3
	v_addc_u32_e32 v1, vcc, v3, v1, vcc
	flat_store_dword v[0:1], v2
BB0_9:
	s_endpgm
.Lfunc_end0:
	.size	simpleNonSeparableConvolution, .Lfunc_end0-simpleNonSeparableConvolution
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 532
; NumSgprs: 20
; NumVgprs: 15
; ScratchSize: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 2
; VGPRBlocks: 3
; NumSGPRsForWavesPerEU: 20
; NumVGPRsForWavesPerEU: 15
; ReservedVGPRFirst: 0
; ReservedVGPRCount: 0
; COMPUTE_PGM_RSRC2:USER_SGPR: 8
; COMPUTE_PGM_RSRC2:TRAP_HANDLER: 1
; COMPUTE_PGM_RSRC2:TGID_X_EN: 1
; COMPUTE_PGM_RSRC2:TGID_Y_EN: 0
; COMPUTE_PGM_RSRC2:TGID_Z_EN: 0
; COMPUTE_PGM_RSRC2:TIDIG_COMP_CNT: 0

	.ident	"clang version 4.0 "
	.section	".note.GNU-stack"
	.amdgpu_code_object_metadata
---
Version:         [ 1, 0 ]
Kernels:         
  - Name:            simpleNonSeparableConvolution
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       U32
        AccQual:         Default
        AddrSpaceQual:   Global
        Name:            input
        TypeName:        'uint*'
      - Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AccQual:         Default
        AddrSpaceQual:   Global
        Name:            mask
        TypeName:        'float*'
      - Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       I32
        AccQual:         Default
        AddrSpaceQual:   Global
        Name:            output
        TypeName:        'int*'
      - Size:            8
        Align:           8
        ValueKind:       ByValue
        ValueType:       U32
        AccQual:         Default
        Name:            inputDimensions
        TypeName:        uint2
      - Size:            8
        Align:           8
        ValueKind:       ByValue
        ValueType:       U32
        AccQual:         Default
        Name:            maskDimensions
        TypeName:        uint2
      - Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       U32
        AccQual:         Default
        Name:            nExWidth
        TypeName:        uint
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
      KernargSegmentSize: 80
      WavefrontNumSGPRs: 20
      WorkitemNumVGPRs: 15
      KernargSegmentAlign: 4
      GroupSegmentAlign: 4
      PrivateSegmentAlign: 4
      WavefrontSize:   6
...
	.end_amdgpu_code_object_metadata
