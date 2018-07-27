.hsa_code_object_version 2,1
.hsa_code_object_isa 8,0,3,"AMD","AMDGPU"

.text
.p2align        8
.amdgpu_hsa_kernel microbench

	
microbench:                             ; @microbench
	.amd_kernel_code_t
	wavefront_sgpr_count = 8
	workitem_vgpr_count = 5
	.end_amd_kernel_code_t

