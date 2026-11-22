#!/usr/bin/env bash
set -euo pipefail

if (( $# < 2 )); then
    echo "usage: $0 SOURCE OUTPUT [CLANG_FLAGS...]" >&2
    exit 2
fi

source_file=$1
output=$2
shift 2

rocm=${ROCM_PATH:-/opt/rocm}
version_file="$rocm/.info/version"
if [[ ! -r "$version_file" || $(<"$version_file") != 10.2.0 ]]; then
    echo "ROCM_PATH must point to a ROCm 10.2.0 installation" >&2
    exit 1
fi

llvm="$rocm/lib/llvm/bin"
# Create the final ELF beside the requested output so replacement is atomic.
scratch=$(mktemp -d "$(dirname "$output")/.gfx942.XXXXXX")
trap 'rm -rf "$scratch"' EXIT

if [[ "$source_file" == *.cl ]]; then
    # The ROCclr device copy kernel is OpenCL and uses no library functions.
    "$llvm/clang" --target=amdgcn-amd-amdhsa -mcpu=gfx942 -x cl \
        -cl-std=CL2.0 -O3 -nogpulib "$@" "$source_file" -o "$scratch/kernel.hsaco"
else
    # There is no host object to pair with this single device translation unit.
    # Disable the host/device identifier that hashes the temporary output path.
    "$llvm/clang++" --rocm-path="$rocm" --offload-arch=gfx942 --offload-device-only \
        -O3 "$@" -fuse-cuid=none -x hip "$source_file" -o "$scratch/bundle"
    "$llvm/clang-offload-bundler" --type=o --unbundle \
        --targets=hipv4-amdgcn-amd-amdhsa--gfx942 \
        --input="$scratch/bundle" --output="$scratch/kernel.hsaco"
fi
header=$("$llvm/llvm-readelf" --file-header "$scratch/kernel.hsaco")
if ! grep -q 'Machine:.*EM_AMDGPU' <<<"$header" ||
    ! grep -q 'Type:.*DYN' <<<"$header" ||
    ! grep -q 'Flags:.*gfx942' <<<"$header"; then
    echo "expected a linked gfx942 device ELF" >&2
    exit 1
fi
mv "$scratch/kernel.hsaco" "$output"
