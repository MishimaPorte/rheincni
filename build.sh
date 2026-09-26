#!/usr/bin/env bash
set -euo pipefail

# Usage: ./build.sh [additional clang flags]
# Set CLANG to select a different clang executable.
repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
compiler="${CLANG:-clang}"

if ! command -v "$compiler" >/dev/null 2>&1; then
  printf 'Required compiler not found: %s\n' "$compiler" >&2
  exit 1
fi

flags=(-target bpf -O2 -g -Wall -I "$repo_dir")

# Debian-derived systems keep asm headers in a multiarch include directory.
if command -v gcc >/dev/null 2>&1; then
  multiarch="$(gcc -print-multiarch)"
  if [[ -n "$multiarch" && -d "/usr/include/$multiarch" ]]; then
    flags+=(-isystem "/usr/include/$multiarch")
  fi
fi

shopt -s nullglob globstar
sources=("$repo_dir"/ebpf/**/*.c)
if (( ${#sources[@]} == 0 )); then
  printf 'No eBPF C sources found in %s/ebpf.\n' "$repo_dir" >&2
  exit 1
fi

for source in "${sources[@]}"; do
  object="${source%.c}.o"
  printf 'Building %s\n' "${object#"$repo_dir"/}"
  "$compiler" "${flags[@]}" "$@" -c "$source" -o "$object"
done

for item in ipam router_service; do
    echo "Generating $item..."
    ./$item/generate.sh
done
