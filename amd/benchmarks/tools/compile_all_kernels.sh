#!/usr/bin/env bash
#
# compile_all_kernels.sh — compile every OpenCL kernel source under
# amd/benchmarks to a gfx942 (MI300A / CDNA3) code object with the pinned clang
# toolchain, then decode-verify each one with MGPUSim's own disassembler.
#
# This is the CI entry point. It does NOT overwrite the checked-in
# kernels_gfx942.hsaco files; it builds into an output directory so the toolchain
# and the sources can be validated without changing binary provenance.
#
# Usage:
#   compile_all_kernels.sh [output_dir]
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
OUT_DIR="${1:-$REPO_ROOT/.kernel-build}"
COMPILE="$SCRIPT_DIR/clang_compile_kernel.sh"
MCPU="${MCPU:-gfx942}"

mkdir -p "$OUT_DIR"

# Build MGPUSim's disassembler once; we use it as the decode-verify oracle.
DISASM_BIN="$OUT_DIR/gcn3disassembler"
echo "==> building MGPUSim disassembler"
( cd "$REPO_ROOT/amd/insts/gcn3disassembler" && go build -o "$DISASM_BIN" . )

n_ok=0
n_fail=0
failed=()

while IFS= read -r -d '' src; do
	rel="${src#"$REPO_ROOT"/}"
	# Flatten the path into a unique output name.
	name="$(echo "${rel%.cl}" | tr '/' '_')_${MCPU}.hsaco"
	out="$OUT_DIR/$name"

	echo "==> $rel"
	# Optional per-source compile flags (e.g. SHOC templated kernels that need
	# -DT2=float2). Put them in a sidecar file "<source>.clflags".
	export EXTRA_CFLAGS=""
	if [[ -f "$src.clflags" ]]; then
		EXTRA_CFLAGS="$(cat "$src.clflags")"
	fi
	if ! "$COMPILE" "$src" "$out" "$MCPU" >/dev/null 2>"$OUT_DIR/$name.compile.log"; then
		echo "    COMPILE FAILED (see $name.compile.log)"
		n_fail=$((n_fail + 1)); failed+=("$rel (compile)"); continue
	fi

	# Decode-verify: MGPUSim must be able to parse + disassemble every opcode.
	if ! "$DISASM_BIN" "$out" >"$OUT_DIR/$name.disasm" 2>"$OUT_DIR/$name.disasm.log"; then
		echo "    DECODE FAILED (see $name.disasm.log)"
		n_fail=$((n_fail + 1)); failed+=("$rel (decode)"); continue
	fi

	echo "    ok"
	n_ok=$((n_ok + 1))
done < <(find "$REPO_ROOT/amd/benchmarks" -name '*.cl' -print0 | sort -z)

echo ""
echo "==================== summary ===================="
echo "compiled + decoded ok : $n_ok"
echo "failed                : $n_fail"
if [[ $n_fail -gt 0 ]]; then
	printf '  - %s\n' "${failed[@]}"
	exit 1
fi
