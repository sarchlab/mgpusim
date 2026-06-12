#!/bin/bash
# collect_sysinfo.sh -- Gather CPU/GPU/OS/driver info as JSON
#
# Usage:
#   ./collect_sysinfo.sh --run-id <id> --output <file.json>
#   ./collect_sysinfo.sh   # prints JSON to stdout

set -euo pipefail

RUN_ID=""
OUTPUT=""

while [ $# -gt 0 ]; do
    case "$1" in
        --run-id) RUN_ID="$2"; shift 2 ;;
        --output) OUTPUT="$2"; shift 2 ;;
        *)        shift ;;
    esac
done

RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)-$(hostname -s 2>/dev/null || hostname)}"

# --- OS info ---
OS_NAME=$(lsb_release -ds 2>/dev/null \
    || (grep PRETTY_NAME /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"') \
    || echo "unknown")
KERNEL_VER=$(uname -r 2>/dev/null || echo "unknown")

# --- CPU info ---
CPU_MODEL=$(lscpu 2>/dev/null | grep "Model name" | sed 's/.*:\s*//' \
    || sysctl -n machdep.cpu.brand_string 2>/dev/null \
    || echo "unknown")
CPU_CORES=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 0)

# --- RAM ---
RAM_GB=$(free -g 2>/dev/null | awk '/Mem:/{print $2}' \
    || vm_stat 2>/dev/null | awk '/Pages free/{print int($3*4096/1024/1024/1024)}' \
    || echo 0)

# --- GPU/toolchain detection ---
COMPILER="unknown"
COMPILER_VER="unknown"
RUNTIME_VER="unknown"
PLATFORM="unknown"
GPU_MODEL="unknown"
GPU_COUNT=0
GPU_VRAM=0
GPU_DRIVER="unknown"
GPU_ARCH="unknown"
SYSINFO_ERROR=""

if command -v rocm-smi &>/dev/null; then
    PLATFORM="hip"
    COMPILER="hipcc"
    GPU_MODEL=$(rocm-smi --showproductname 2>/dev/null \
        | grep -m1 "Card series" | sed 's/.*:\s*//' || echo "unknown")
    GPU_COUNT=$(rocm-smi -l 2>/dev/null | grep -c "GPU\[" || echo 1)
    GPU_VRAM=$(rocm-smi --showmeminfo vram 2>/dev/null \
        | grep "Total" | head -1 \
        | awk '{printf "%.1f", $3/1024/1024/1024}' || echo 0)
    GPU_DRIVER=$(cat /sys/module/amdgpu/version 2>/dev/null || echo "unknown")
    GPU_ARCH=$(rocminfo 2>/dev/null \
        | grep -m1 'Name:.*gfx' | awk '{print $2}' || echo "unknown")
    COMPILER_VER=$(hipcc --version 2>/dev/null || echo "unknown")
    RUNTIME_VER=$(cat /opt/rocm/.info/version 2>/dev/null \
        || echo "unknown")
elif command -v nvidia-smi &>/dev/null; then
    PLATFORM="cuda"
    COMPILER="nvcc"
    GPU_MODEL=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null \
        | head -1 || echo "unknown")
    GPU_COUNT=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null \
        | wc -l | tr -d ' ' || echo 1)
    GPU_VRAM=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits \
        2>/dev/null | head -1 | awk '{printf "%.1f", $1/1024}' || echo 0)
    GPU_DRIVER=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader \
        2>/dev/null | head -1 || echo "unknown")
    GPU_ARCH=$(nvidia-smi --query-gpu=compute_cap --format=csv,noheader \
        2>/dev/null | head -1 | tr -d '.' | sed 's/^/sm_/' || echo "unknown")
    COMPILER_VER=$(nvcc --version 2>/dev/null || echo "unknown")
    RUNTIME_VER="CUDA"
else
    SYSINFO_ERROR="No GPU runtime detected (rocm-smi or nvidia-smi)"
fi

# --- Generate JSON with python for robust escaping/newline handling ---
export SYSINFO_RUN_ID="$RUN_ID"
export SYSINFO_TIMESTAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
export SYSINFO_HOSTNAME="$(hostname 2>/dev/null || echo unknown)"
export SYSINFO_OS="$OS_NAME"
export SYSINFO_KERNEL="$KERNEL_VER"
export SYSINFO_CPU_MODEL="$CPU_MODEL"
export SYSINFO_CPU_CORES="$CPU_CORES"
export SYSINFO_RAM_GB="$RAM_GB"
export SYSINFO_GPU_MODEL="$GPU_MODEL"
export SYSINFO_GPU_COUNT="$GPU_COUNT"
export SYSINFO_GPU_VRAM="$GPU_VRAM"
export SYSINFO_GPU_DRIVER="$GPU_DRIVER"
export SYSINFO_GPU_ARCH="$GPU_ARCH"
export SYSINFO_COMPILER="$COMPILER"
export SYSINFO_COMPILER_VER="$COMPILER_VER"
export SYSINFO_RUNTIME_VER="$RUNTIME_VER"
export SYSINFO_PLATFORM="$PLATFORM"
export SYSINFO_ERROR="$SYSINFO_ERROR"

if [ -n "$OUTPUT" ]; then
    mkdir -p "$(dirname "$OUTPUT")"
fi

python3 - "$OUTPUT" <<'PYEOF'
import json
import os
import sys

output_path = sys.argv[1]


def safe_int(val, default=0):
    try:
        return int(float(val))
    except (ValueError, TypeError):
        return default


def safe_float(val, default=0.0):
    try:
        return float(val)
    except (ValueError, TypeError):
        return default


data = {
    "run_id": os.environ.get("SYSINFO_RUN_ID", ""),
    "timestamp": os.environ.get("SYSINFO_TIMESTAMP", ""),
    "hostname": os.environ.get("SYSINFO_HOSTNAME", ""),
    "os": os.environ.get("SYSINFO_OS", ""),
    "kernel": os.environ.get("SYSINFO_KERNEL", ""),
    "cpu": {
        "model": os.environ.get("SYSINFO_CPU_MODEL", ""),
        "cores": safe_int(os.environ.get("SYSINFO_CPU_CORES", "0")),
    },
    "ram_gb": safe_float(os.environ.get("SYSINFO_RAM_GB", "0")),
    "gpu": {
        "model": os.environ.get("SYSINFO_GPU_MODEL", ""),
        "count": safe_int(os.environ.get("SYSINFO_GPU_COUNT", "0")),
        "vram_gb": safe_float(os.environ.get("SYSINFO_GPU_VRAM", "0")),
        "driver": os.environ.get("SYSINFO_GPU_DRIVER", ""),
        "arch": os.environ.get("SYSINFO_GPU_ARCH", ""),
    },
    "compiler": os.environ.get("SYSINFO_COMPILER", ""),
    "compiler_version": os.environ.get("SYSINFO_COMPILER_VER", ""),
    "runtime": os.environ.get("SYSINFO_RUNTIME_VER", ""),
    "platform": os.environ.get("SYSINFO_PLATFORM", ""),
}

error_message = os.environ.get("SYSINFO_ERROR", "").strip()
if error_message:
    data["error"] = error_message

payload = json.dumps(data, indent=2, ensure_ascii=False) + "\n"

if output_path:
    with open(output_path, "w", encoding="utf-8") as out:
        out.write(payload)
else:
    sys.stdout.write(payload)
PYEOF

if [ -n "$OUTPUT" ]; then
    echo "System info written to $OUTPUT" >&2
fi
