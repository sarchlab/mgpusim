# Docker-based HIP compilation for gputensor kernels
# Uses ROCm Docker image - no local ROCm installation required

DOCKER_IMAGE = rocm/dev-ubuntu-24.04:7.1.1
GPU_TARGET = gfx942

# Run hipcc inside Docker container
# --platform linux/amd64 required for Apple Silicon Macs
DOCKER_RUN = docker run --rm --platform linux/amd64 -v $(PWD):/work -w /work $(DOCKER_IMAGE)

# Source files
SRCS = operator_hip.cpp repeat_hip.cpp im2col_hip.cpp maxpooling_hip.cpp avgpooling_hip.cpp gemm_hip.cpp cross_entropy_hip.cpp

# HSACO output files (in parent directory)
HSACO_DIR = ..
OPERATOR_HSACO = $(HSACO_DIR)/operator_gfx942.hsaco
REPEAT_HSACO = $(HSACO_DIR)/repeat_gfx942.hsaco
IM2COL_HSACO = $(HSACO_DIR)/im2col_gfx942.hsaco
MAXPOOLING_HSACO = $(HSACO_DIR)/maxpooling_gfx942.hsaco
AVGPOOLING_HSACO = $(HSACO_DIR)/avgpooling_gfx942.hsaco
GEMM_HSACO = $(HSACO_DIR)/gemm_gfx942.hsaco
CROSS_ENTROPY_HSACO = $(HSACO_DIR)/cross_entropy_gfx942.hsaco

ALL_HSACO = $(OPERATOR_HSACO) $(REPEAT_HSACO) $(IM2COL_HSACO) $(MAXPOOLING_HSACO) $(AVGPOOLING_HSACO) $(GEMM_HSACO) $(CROSS_ENTROPY_HSACO)

.PHONY: all clean clean-all

all: $(ALL_HSACO)

$(OPERATOR_HSACO): operator_hip.cpp
	$(DOCKER_RUN) hipcc --save-temps -c --offload-arch=$(GPU_TARGET) $<
	cp operator_hip-hip-amdgcn-amd-amdhsa-$(GPU_TARGET).o $@

$(REPEAT_HSACO): repeat_hip.cpp
	$(DOCKER_RUN) hipcc --save-temps -c --offload-arch=$(GPU_TARGET) $<
	cp repeat_hip-hip-amdgcn-amd-amdhsa-$(GPU_TARGET).o $@

$(IM2COL_HSACO): im2col_hip.cpp
	$(DOCKER_RUN) hipcc --save-temps -c --offload-arch=$(GPU_TARGET) $<
	cp im2col_hip-hip-amdgcn-amd-amdhsa-$(GPU_TARGET).o $@

$(MAXPOOLING_HSACO): maxpooling_hip.cpp
	$(DOCKER_RUN) hipcc --save-temps -c --offload-arch=$(GPU_TARGET) $<
	cp maxpooling_hip-hip-amdgcn-amd-amdhsa-$(GPU_TARGET).o $@

$(AVGPOOLING_HSACO): avgpooling_hip.cpp
	$(DOCKER_RUN) hipcc --save-temps -c --offload-arch=$(GPU_TARGET) $<
	cp avgpooling_hip-hip-amdgcn-amd-amdhsa-$(GPU_TARGET).o $@

$(GEMM_HSACO): gemm_hip.cpp
	$(DOCKER_RUN) hipcc --save-temps -c --offload-arch=$(GPU_TARGET) $<
	cp gemm_hip-hip-amdgcn-amd-amdhsa-$(GPU_TARGET).o $@

$(CROSS_ENTROPY_HSACO): cross_entropy_hip.cpp
	$(DOCKER_RUN) hipcc --save-temps -c --offload-arch=$(GPU_TARGET) $<
	cp cross_entropy_hip-hip-amdgcn-amd-amdhsa-$(GPU_TARGET).o $@

clean:
	rm -f *.ll *.bc *.s *.o *.cui *.fatbin *.hipfb *.out *.txt *.hipi

clean-all: clean
	rm -f $(ALL_HSACO)
