#!/bin/bash
# Test MGPUSim disassembler against LLVM reference output
#
# Usage:
#   ./test-disasm.sh         # Show diff, exit 0
#   ./test-disasm.sh --check # Fail if outputs differ (for CI)

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
"$DISASSEMBLER" "$HSACO" > "$MGPUSIM_DISASM"

# Compare outputs
echo ""
echo "=== Comparing disassembler outputs ==="
echo "LLVM reference: $LLVM_DISASM"
echo "MGPUSim output: $MGPUSIM_DISASM"
echo ""

if [ "$1" = "--check" ]; then
    # Strict mode for CI - fail if different
    if diff -q "$LLVM_DISASM" "$MGPUSIM_DISASM" > /dev/null; then
        echo "OK: Disassembler outputs match"
        exit 0
    else
        echo "FAIL: Disassembler outputs differ"
        echo ""
        diff -u "$LLVM_DISASM" "$MGPUSIM_DISASM" || true
        exit 1
    fi
else
    # Show diff but don't fail
    diff -u "$LLVM_DISASM" "$MGPUSIM_DISASM" || echo -e "\nDifferences found (see above)"
fi
