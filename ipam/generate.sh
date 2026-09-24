#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
output_dir="$script_dir/gen/ipamv1"

for command in protoc protoc-gen-go protoc-gen-go-grpc; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$command" >&2
    exit 1
  fi
done

mkdir -p "$output_dir"

protoc \
  --proto_path="$script_dir" \
  --go_out="$output_dir" \
  --go_opt=paths=source_relative \
  --go-grpc_out="$output_dir" \
  --go-grpc_opt=paths=source_relative \
  "$script_dir/ipam.proto"
