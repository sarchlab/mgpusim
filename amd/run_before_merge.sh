#!/usr/bin/env bash

set -Eeuo pipefail

MOCKGEN_VERSION="v0.6.0"
GINKGO_VERSION="v2.32.1"
GOLANGCI_LINT_VERSION="v2.13.2"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temp_parent_dir="${MGPUSIM_RUN_BEFORE_MERGE_TMPDIR:-${TMPDIR:-/tmp}}"
temp_dir="$(mktemp -d "${temp_parent_dir}/mgpusim-run-before-merge.XXXXXX")"
bin_dir="${temp_dir}/bin"
go_path_dir="${temp_dir}/gopath"
go_mod_cache_dir="${go_path_dir}/pkg/mod"
go_build_cache_dir="${temp_dir}/go-build-cache"
lint_cache_dir="${temp_dir}/golangci-lint-cache"
go_env=(
  "GOPATH=${go_path_dir}"
  "GOMODCACHE=${go_mod_cache_dir}"
  "GOCACHE=${go_build_cache_dir}"
  "GOWORK=off"
)

cleanup() {
  if [[ -d "${temp_dir}" ]]; then
    chmod -R u+w -- "${temp_dir}" 2>/dev/null || true
    rm -rf -- "${temp_dir}"
  fi
}
trap cleanup EXIT

cd "${repo_root}"
mkdir -p -- "${bin_dir}" "${go_mod_cache_dir}" "${go_build_cache_dir}" "${lint_cache_dir}"

print_command() {
  printf '+'
  printf ' %q' "$@"
  printf '\n'
}

run() {
  print_command "$@"
  "$@"
}

verify_clean_tree() {
  local phase="$1"
  local current_worktree_status

  current_worktree_status="$(git status --porcelain=v1 --untracked-files=all)"

  if [[ -n "${current_worktree_status}" ]]; then
    printf 'run_before_merge.sh failed: the worktree is not clean during %s.\n' "${phase}" >&2
    git status --short >&2
    return 1
  fi
}

verify_clean_tree "startup"

printf 'Running local MGPUSim Go build/lint/test gate.\n'

run env "${go_env[@]}" go version
run env "${go_env[@]}" go env GOVERSION GOTOOLCHAIN GOMOD
run env "${go_env[@]}" go list -mod=readonly -m all
run env "${go_env[@]}" go mod tidy -diff

run env "${go_env[@]}" GOBIN="${bin_dir}" go install "go.uber.org/mock/mockgen@${MOCKGEN_VERSION}"
run env "${go_env[@]}" GOBIN="${bin_dir}" go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
run env "${go_env[@]}" GOBIN="${bin_dir}" go install "github.com/onsi/ginkgo/v2/ginkgo@${GINKGO_VERSION}"

run env "${go_env[@]}" PATH="${bin_dir}:${PATH}" go generate -mod=readonly ./...
run env "${go_env[@]}" go build -mod=readonly ./...
run env "${go_env[@]}" GOLANGCI_LINT_CACHE="${lint_cache_dir}" \
  "${bin_dir}/golangci-lint" run --timeout=10m --modules-download-mode=readonly ./...
run env "${go_env[@]}" "${bin_dir}/ginkgo" -r --skip-package=mccl --mod=readonly

verify_clean_tree "validation"

printf 'Local MGPUSim Go build/lint/test gate completed successfully.\n'
