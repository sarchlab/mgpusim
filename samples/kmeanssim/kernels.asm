	.text
	.hsa_code_object_version 2,1
	.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"
	.globl	kmeans_kernel_compute   ; -- Begin function kmeans_kernel_compute
	.p2align	8
	.type	kmeans_kernel_compute,@function
	.amdgpu_hsa_kernel kmeans_kernel_compute
kmeans_kernel_compute:                  ; @kmeans_kernel_compute
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
		granulated_workitem_vgpr_count = 1
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
		wavefront_sgpr_count = 18
		workitem_vgpr_count = 8
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
	s_load_dword s2, s[4:5], 0x4
	s_load_dword s3, s[6:7], 0x18
	s_load_dwordx2 s[0:1], s[6:7], 0x30
	s_waitcnt lgkmcnt(0)
	s_and_b32 s1, s2, 0xffff
	s_mul_i32 s8, s8, s1
	v_add_u32_e32 v0, vcc, s8, v0
	v_add_u32_e32 v0, vcc, s0, v0
	v_cmp_gt_u32_e32 vcc, s3, v0
	s_and_saveexec_b64 s[0:1], vcc
	; mask branch BB0_10
	s_cbranch_execz BB0_10
BB0_1:
	s_load_dwordx2 s[0:1], s[6:7], 0x10
	s_load_dword s2, s[6:7], 0x1c
	v_mov_b32_e32 v2, 0
	s_waitcnt lgkmcnt(0)
	s_cmp_lt_i32 s2, 1
	s_cbranch_scc1 BB0_9
; %bb.2:
	s_load_dword s4, s[6:7], 0x20
	s_mov_b32 s5, 0
	s_waitcnt lgkmcnt(0)
	s_cmp_gt_i32 s4, 0
	s_cbranch_scc0 BB0_7
; %bb.3:                                ; %.preheader
	s_load_dwordx2 s[8:9], s[6:7], 0x0
	s_load_dwordx2 s[6:7], s[6:7], 0x8
	s_ashr_i32 s5, s4, 31
	s_lshl_b64 s[10:11], s[4:5], 2
	v_mov_b32_e32 v1, 0x7f7fffff
	s_mov_b32 s5, 0
	v_mov_b32_e32 v2, 0
BB0_4:                                  ; =>This Loop Header: Depth=1
                                        ;     Child Loop BB0_5 Depth 2
	v_mov_b32_e32 v3, 0
	s_mov_b32 s12, s4
	v_mov_b32_e32 v4, v0
	s_waitcnt lgkmcnt(0)
	s_mov_b64 s[14:15], s[6:7]
BB0_5:                                  ;   Parent Loop BB0_4 Depth=1
                                        ; =>  This Inner Loop Header: Depth=2
	v_mov_b32_e32 v5, 0
	v_lshlrev_b64 v[5:6], 2, v[4:5]
	v_add_u32_e32 v4, vcc, s3, v4
	v_mov_b32_e32 v7, s9
	v_add_u32_e32 v5, vcc, s8, v5
	v_addc_u32_e32 v6, vcc, v7, v6, vcc
	flat_load_dword v5, v[5:6]
	s_load_dword s13, s[14:15], 0x0
	s_add_u32 s14, s14, 4
	s_addc_u32 s15, s15, 0
	s_add_i32 s12, s12, -1
	s_cmp_lg_u32 s12, 0
	s_waitcnt vmcnt(0) lgkmcnt(0)
	v_subrev_f32_e32 v5, s13, v5
	v_mac_f32_e32 v3, v5, v5
	s_cbranch_scc1 BB0_5
; %bb.6:                                ;   in Loop: Header=BB0_4 Depth=1
	v_mov_b32_e32 v4, s5
	s_add_i32 s5, s5, 1
	s_add_u32 s6, s6, s10
	v_cmp_lt_f32_e32 vcc, v3, v1
	s_addc_u32 s7, s7, s11
	v_cndmask_b32_e32 v2, v2, v4, vcc
	s_cmp_eq_u32 s5, s2
	v_cndmask_b32_e32 v1, v1, v3, vcc
	s_cbranch_scc0 BB0_4
	s_branch BB0_9
BB0_7:                                  ; %.preheader13
	v_mov_b32_e32 v1, 0x7f7fffff
	v_mov_b32_e32 v2, 0
BB0_8:                                  ; =>This Inner Loop Header: Depth=1
	v_cmp_lt_f32_e32 vcc, 0, v1
	v_mov_b32_e32 v3, s5
	s_add_i32 s5, s5, 1
	v_cndmask_b32_e32 v2, v2, v3, vcc
	s_cmp_eq_u32 s2, s5
	v_cndmask_b32_e64 v1, v1, 0, vcc
	s_cbranch_scc0 BB0_8
BB0_9:                                  ; %.loopexit
	v_mov_b32_e32 v1, 0
	v_lshlrev_b64 v[0:1], 2, v[0:1]
	v_mov_b32_e32 v3, s1
	v_add_u32_e32 v0, vcc, s0, v0
	v_addc_u32_e32 v1, vcc, v3, v1, vcc
	flat_store_dword v[0:1], v2
BB0_10:
	s_endpgm
.Lfunc_end0:
	.size	kmeans_kernel_compute, .Lfunc_end0-kmeans_kernel_compute
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 372
; NumSgprs: 18
; NumVgprs: 8
; ScratchSize: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 2
; VGPRBlocks: 1
; NumSGPRsForWavesPerEU: 18
; NumVGPRsForWavesPerEU: 8
; ReservedVGPRFirst: 0
; ReservedVGPRCount: 0
; COMPUTE_PGM_RSRC2:USER_SGPR: 8
; COMPUTE_PGM_RSRC2:TRAP_HANDLER: 1
; COMPUTE_PGM_RSRC2:TGID_X_EN: 1
; COMPUTE_PGM_RSRC2:TGID_Y_EN: 0
; COMPUTE_PGM_RSRC2:TGID_Z_EN: 0
; COMPUTE_PGM_RSRC2:TIDIG_COMP_CNT: 0
	.text
	.globl	kmeans_kernel_swap      ; -- Begin function kmeans_kernel_swap
	.p2align	8
	.type	kmeans_kernel_swap,@function
	.amdgpu_hsa_kernel kmeans_kernel_swap
kmeans_kernel_swap:                     ; @kmeans_kernel_swap
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
		granulated_workitem_vgpr_count = 1
		granulated_wavefront_sgpr_count = 1
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
		kernarg_segment_byte_size = 56
		workgroup_fbarrier_count = 0
		wavefront_sgpr_count = 11
		workitem_vgpr_count = 8
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
	s_load_dword s2, s[4:5], 0x4
	s_load_dword s0, s[6:7], 0x10
	s_load_dword s1, s[6:7], 0x14
	s_load_dword s3, s[6:7], 0x18
	s_waitcnt lgkmcnt(0)
	s_and_b32 s2, s2, 0xffff
	s_mul_i32 s8, s8, s2
	v_add_u32_e32 v0, vcc, s8, v0
	v_add_u32_e32 v0, vcc, s3, v0
	v_cmp_gt_u32_e32 vcc, s0, v0
	v_cmp_gt_i32_e64 s[2:3], s1, 0
	s_and_b64 s[2:3], s[2:3], vcc
	s_and_saveexec_b64 s[4:5], s[2:3]
	; mask branch BB1_3
	s_cbranch_execz BB1_3
BB1_1:
	s_load_dwordx2 s[2:3], s[6:7], 0x0
	s_load_dwordx2 s[4:5], s[6:7], 0x8
	v_mul_lo_i32 v2, v0, s1
BB1_2:                                  ; =>This Inner Loop Header: Depth=1
	v_mov_b32_e32 v3, 0
	v_lshlrev_b64 v[4:5], 2, v[2:3]
	s_waitcnt lgkmcnt(0)
	v_mov_b32_e32 v1, s3
	v_add_u32_e32 v4, vcc, s2, v4
	v_addc_u32_e32 v5, vcc, v1, v5, vcc
	v_mov_b32_e32 v1, v3
	v_lshlrev_b64 v[6:7], 2, v[0:1]
	flat_load_dword v1, v[4:5]
	v_add_u32_e32 v0, vcc, s0, v0
	s_add_i32 s1, s1, -1
	v_mov_b32_e32 v4, s5
	v_add_u32_e32 v3, vcc, s4, v6
	v_addc_u32_e32 v4, vcc, v4, v7, vcc
	s_cmp_eq_u32 s1, 0
	v_add_u32_e32 v2, vcc, 1, v2
	s_waitcnt vmcnt(0) lgkmcnt(0)
	flat_store_dword v[3:4], v1
	s_cbranch_scc0 BB1_2
BB1_3:                                  ; %.loopexit
	s_endpgm
.Lfunc_end1:
	.size	kmeans_kernel_swap, .Lfunc_end1-kmeans_kernel_swap
                                        ; -- End function
	.section	.AMDGPU.csdata
; Kernel info:
; codeLenInByte = 200
; NumSgprs: 11
; NumVgprs: 8
; ScratchSize: 0
; FloatMode: 192
; IeeeMode: 1
; LDSByteSize: 0 bytes/workgroup (compile time only)
; SGPRBlocks: 1
; VGPRBlocks: 1
; NumSGPRsForWavesPerEU: 11
; NumVGPRsForWavesPerEU: 8
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
	.amd_amdgpu_isa "amdgcn-amd-amdhsa-amdgizcl-gfx803"
	.amd_amdgpu_hsa_metadata
---
Version:         [ 1, 0 ]
Kernels:         
  - Name:            kmeans_kernel_compute
    SymbolName:      'kmeans_kernel_compute@kd'
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Name:            feature
        TypeName:        'float*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            clusters
        TypeName:        'float*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            membership
        TypeName:        'int*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       I32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            npoints
        TypeName:        int
        Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
      - Name:            nclusters
        TypeName:        int
        Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
      - Name:            nfeatures
        TypeName:        int
        Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
      - Name:            offset
        TypeName:        int
        Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
      - Name:            size
        TypeName:        int
        Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
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
      KernargSegmentSize: 80
      GroupSegmentFixedSize: 0
      PrivateSegmentFixedSize: 0
      KernargSegmentAlign: 8
      WavefrontSize:   64
      NumSGPRs:        18
      NumVGPRs:        8
      MaxFlatWorkGroupSize: 256
  - Name:            kmeans_kernel_swap
    SymbolName:      'kmeans_kernel_swap@kd'
    Language:        OpenCL C
    LanguageVersion: [ 1, 2 ]
    Args:            
      - Name:            feature
        TypeName:        'float*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            feature_swap
        TypeName:        'float*'
        Size:            8
        Align:           8
        ValueKind:       GlobalBuffer
        ValueType:       F32
        AddrSpaceQual:   Global
        AccQual:         Default
      - Name:            npoints
        TypeName:        int
        Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
        AccQual:         Default
      - Name:            nfeatures
        TypeName:        int
        Size:            4
        Align:           4
        ValueKind:       ByValue
        ValueType:       I32
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
      NumSGPRs:        11
      NumVGPRs:        8
      MaxFlatWorkGroupSize: 256
...

	.end_amd_amdgpu_hsa_metadata
