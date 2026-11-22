#!/bin/bash
# Test MGPUSim disassembler against LLVM reference output
#
# Usage:
#   ./test-disasm.sh         # Show diff, exit 0
#   ./test-disasm.sh --check # Compare decoded instructions (for CI)

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MGPUSIM_ROOT="$SCRIPT_DIR/../../../../.."

HSACO="$SCRIPT_DIR/vectoradd.hsaco"
LLVM_DISASM="$SCRIPT_DIR/vectoradd.disasm"
MGPUSIM_DISASM="$SCRIPT_DIR/vectoradd.mgpusim.disasm"
DISASSEMBLER="$MGPUSIM_ROOT/amd/insts/gcn3disassembler/gcn3disassembler"

# Check required files exist
if [ ! -f "$HSACO" ]; then
    echo "Error: $HSACO not found"
    exit 1
fi

if [ ! -f "$LLVM_DISASM" ]; then
    echo "Error: $LLVM_DISASM not found"
    echo "This file should be checked into git as the reference output."
    exit 1
fi

# Build disassembler if needed
if [ ! -f "$DISASSEMBLER" ]; then
    echo "Building MGPUSim disassembler..."
    (cd "$MGPUSIM_ROOT/amd/insts/gcn3disassembler" && go build -o gcn3disassembler .)
fi

# Run MGPUSim disassembler
echo "Disassembling $HSACO..."
"$DISASSEMBLER" -arch=cdna3 "$HSACO" > "$MGPUSIM_DISASM"

# Compare outputs
echo ""
echo "=== Comparing disassembler outputs ==="
echo "LLVM reference: $LLVM_DISASM"
echo "MGPUSim output: $MGPUSIM_DISASM"
echo ""

if [ "${1:-}" = "--check" ]; then
    # LLVM reports linked virtual addresses, symbols and branch labels.
    # Compare kernel-relative offsets, decoded operands and machine words.
    python3 - "$LLVM_DISASM" "$MGPUSIM_DISASM" <<'PYTHON'
import difflib
import re
import sys
from pathlib import Path

instruction = re.compile(r"^\s*(.*?)\s*//\s*([0-9a-fA-F]+):\s*(.*?)(?:\s+<[^>]+>)?\s*$")

def instructions(path):
    result = []
    base = None
    for line in Path(path).read_text().splitlines():
        match = instruction.fullmatch(line)
        if match is None:
            continue
        decoded, address, words = match.groups()
        offset = int(address, 16)
        if base is None:
            base = offset
        result.append(f"{offset - base:04x}: {' '.join(decoded.split())} // {' '.join(words.upper().split())}\n")
    if not result:
        raise SystemExit(f"No decoded instructions in {path}")
    return result

expected, actual = (instructions(path) for path in sys.argv[1:])
if expected != actual:
    sys.stderr.writelines(difflib.unified_diff(expected, actual, *sys.argv[1:]))
    raise SystemExit(1)
print("OK: Disassembler instructions match")
PYTHON
else
    # Show the full output, including format differences.
    diff -u "$LLVM_DISASM" "$MGPUSIM_DISASM" || echo -e "\nDifferences found (see above)"
fi
