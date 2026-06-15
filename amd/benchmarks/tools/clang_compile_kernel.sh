#!/usr/bin/env bash
#
# clang_compile_kernel.sh — compile an OpenCL kernel to an AMD GPU code object
# (HSACO) using a *pinned* clang/LLVM toolchain + libclc. No ROCm SDK and no
# Docker required.
#
# This replaces the old `hipcc`-inside-Docker recipe (see native/Makefile in the
# benchmark folders) for environments where the ROCm container cannot be pulled.
# The toolchain version is intentionally locked (see CLANG_VERSION below) so that
# kernel binaries are reproducible across machines and CI.
#
# Usage:
#   clang_compile_kernel.sh <input.cl> <output.hsaco> [mcpu]
#
# Environment overrides:
#   CLANG   - clang binary to use            (default: clang-18)
#   LIBCLC  - libclc amdhsa bitcode to link  (default: /usr/lib/clc/amdgcn--amdhsa.bc)
#   MCPU    - target GPU                      (default: gfx942, i.e. MI300A / CDNA3)
#
set -euo pipefail

# --- Pinned toolchain --------------------------------------------------------
# We lock to LLVM 18.x. On Ubuntu 24.04 this is package set
# clang-18 / libclc-18 / lld-18 == 1:18.1.3-1ubuntu1.
CLANG_VERSION_PREFIX="18.1"
CLANG="${CLANG:-clang-18}"
LIBCLC="${LIBCLC:-/usr/lib/clc/amdgcn--amdhsa.bc}"

# --- Args --------------------------------------------------------------------
if [[ $# -lt 2 ]]; then
	echo "usage: $0 <input.cl> <output.hsaco> [mcpu]" >&2
	exit 2
fi
SRC="$1"
OUT="$2"
MCPU="${3:-${MCPU:-gfx942}}"

# --- Preconditions -----------------------------------------------------------
if ! command -v "$CLANG" >/dev/null 2>&1; then
	echo "error: $CLANG not found. Install with: apt-get install clang-18 libclc-18 lld-18" >&2
	exit 1
fi

CLANG_VERSION="$("$CLANG" --version | head -1)"
if [[ "$CLANG_VERSION" != *"$CLANG_VERSION_PREFIX"* ]]; then
	echo "error: pinned toolchain mismatch." >&2
	echo "       expected clang $CLANG_VERSION_PREFIX.x, got: $CLANG_VERSION" >&2
	exit 1
fi

if [[ ! -f "$LIBCLC" ]]; then
	echo "error: libclc bitcode not found at $LIBCLC" >&2
	echo "       install with: apt-get install libclc-18" >&2
	exit 1
fi

# --- Compile -----------------------------------------------------------------
# Flag notes:
#   -x cl -cl-std=CL2.0           : treat input as OpenCL C
#   --target=amdgcn-amd-amdhsa    : emit an amdhsa code object (loadable HSACO)
#   -mcpu=$MCPU                   : gfx942 == MI300A / CDNA3
#   -nogpulib                    : do not search for the ROCm device library...
#   -mlink-builtin-bitcode <bc>  : ...link libclc instead (provides OpenCL builtins)
#   -finclude-default-header     : provide OpenCL builtin declarations
#   -fno-*vectorize / load-store-vectorizer=false :
#                                  keep memory accesses scalar. mgpusim's
#                                  functional emulator reads memory per-access and
#                                  does not benefit from widened (dwordx2/x4)
#                                  vector loads; keeping loads scalar avoids
#                                  page-straddling wide accesses.
"$CLANG" \
	-x cl -cl-std=CL2.0 \
	--target=amdgcn-amd-amdhsa -mcpu="$MCPU" \
	-nogpulib \
	-Xclang -finclude-default-header \
	-Xclang -mlink-builtin-bitcode -Xclang "$LIBCLC" \
	-O2 -fno-slp-vectorize -fno-vectorize \
	-mllvm -amdgpu-load-store-vectorizer=false \
	${EXTRA_CFLAGS:-} \
	"$SRC" -o "$OUT"

echo "compiled: $SRC -> $OUT ($MCPU)"
