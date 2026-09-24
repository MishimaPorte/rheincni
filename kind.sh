#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
config_dir="$repo_dir/.kind"
kind_config="$config_dir/kind.yaml"
cni_config="$config_dir/10-rheincni.conf"
kubeconfig="$config_dir/kubeconfig"
plugin_binary="$config_dir/bin/rheincni"
ipam_binary="$config_dir/bin/rheincni-ipam"
ipam_image="rheincni-ipam:dev"
ipam_dockerfile="$repo_dir/deploy/ipam-agent.Dockerfile"
ipam_manifest="$repo_dir/deploy/ipam-agent.yaml"
cluster_name="${KIND_CLUSTER_NAME:-rheincni}"
action="${1:-up}"

usage() {
  printf 'Usage: %s [up|delete]\n' "$0" >&2
  printf 'Set KIND_CLUSTER_NAME to use a different cluster name.\n' >&2
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$1" >&2
    exit 1
  fi
}

case "$action" in
  delete)
    require_command kind
    kind delete cluster --name "$cluster_name" --kubeconfig "$kubeconfig"
    exit
    ;;
  up) ;;
  *) usage; exit 2 ;;
esac

require_command kind
require_command docker
require_command go
require_command kubectl

if ! docker info >/dev/null 2>&1; then
  printf 'Docker is not available. Start the Docker daemon and try again.\n' >&2
  exit 1
fi

case "$(docker info --format '{{.Architecture}}')" in
  x86_64|amd64) node_arch=amd64 ;;
  aarch64|arm64) node_arch=arm64 ;;
  *) printf 'Unsupported Docker node architecture.\n' >&2; exit 1 ;;
esac

mkdir -p "$config_dir/bin"

# The current loader uses cgo. A static Linux binary can run in the kind nodes
# even when the host and node images have different libc versions.
printf 'Building rheincni for linux/%s...\n' "$node_arch"
(
  cd "$repo_dir"
  GOOS=linux GOARCH="$node_arch" CGO_ENABLED=1 \
    go build -ldflags='-linkmode external -extldflags -static' \
    -o "$plugin_binary" ./cmd/cni
)

printf 'Building rheincni IPAM agent for linux/%s...\n' "$node_arch"
(
  cd "$repo_dir"
  GOOS=linux GOARCH="$node_arch" CGO_ENABLED=1 \
    go build -tags netgo,osusergo -trimpath -ldflags='-linkmode external -extldflags -static' \
    -o "$ipam_binary" ./cmd/ipam
)

printf 'Building %s...\n' "$ipam_image"
docker build \
  --platform "linux/$node_arch" \
  --file "$ipam_dockerfile" \
  --tag "$ipam_image" \
  "$config_dir/bin"

if [[ ! -e "$kind_config" ]]; then
  cat >"$kind_config" <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
  podSubnet: 10.244.0.0/16
nodes:
  - role: control-plane
  - role: worker
EOF
fi

if [[ ! -e "$cni_config" ]]; then
  cat >"$cni_config" <<'EOF'
{
  "cniVersion": "1.0.0",
  "name": "rheincni",
  "type": "rheincni"
}
EOF
fi

clusters="$(kind get clusters 2>/dev/null)"
if ! printf '%s\n' "$clusters" | grep -Fxq "$cluster_name"; then
  printf 'Creating kind cluster %s...\n' "$cluster_name"
  kind create cluster --name "$cluster_name" --config "$kind_config" --kubeconfig "$kubeconfig"
else
  printf 'Updating existing kind cluster %s...\n' "$cluster_name"
  kind export kubeconfig --name "$cluster_name" --kubeconfig "$kubeconfig"
fi

nodes="$(kind get nodes --name "$cluster_name")"
if [[ -z "$nodes" ]]; then
  printf 'No nodes found in kind cluster %s.\n' "$cluster_name" >&2
  exit 1
fi

# Refuse to take over a cluster that already has a different CNI configured.
for node in $nodes; do
  docker exec "$node" sh -c '
    for file in /etc/cni/net.d/*.conf /etc/cni/net.d/*.conflist /etc/cni/net.d/*.json; do
      if [ -e "$file" ] && [ "$file" != /etc/cni/net.d/10-rheincni.conf ]; then
        printf "Another CNI config is present on %s: %s\n" "$(hostname)" "$file" >&2
        exit 1
      fi
    done
  '
done

printf 'Loading %s into kind cluster %s...\n' "$ipam_image" "$cluster_name"
kind load docker-image --name "$cluster_name" "$ipam_image"

printf 'Applying rheincni IPAM agent manifests...\n'
kubectl --kubeconfig "$kubeconfig" --context "kind-$cluster_name" \
  apply -f "$ipam_manifest"
kubectl --kubeconfig "$kubeconfig" --context "kind-$cluster_name" \
  rollout restart daemonset/rheincni-ipam --namespace kube-system
kubectl --kubeconfig "$kubeconfig" --context "kind-$cluster_name" \
  rollout status daemonset/rheincni-ipam --namespace kube-system --timeout=120s

for node in $nodes; do
  printf 'Installing rheincni on %s...\n' "$node"
  docker exec "$node" mkdir -p /opt/cni/bin /etc/cni/net.d
  docker cp "$plugin_binary" "$node:/opt/cni/bin/rheincni"
  docker exec "$node" chmod 0755 /opt/cni/bin/rheincni
  docker cp "$cni_config" "$node:/etc/cni/net.d/10-rheincni.conf"
done

printf 'Cluster %s is configured to use rheincni.\n' "$cluster_name"
printf 'Kubeconfig: %s (context kind-%s)\n' "$kubeconfig" "$cluster_name"
first_node="${nodes%%$'\n'*}"
if ! docker exec -e CNI_COMMAND=VERSION "$first_node" /opt/cni/bin/rheincni \
    >"$config_dir/version-output" 2>&1; then
  printf 'Warning: rheincni failed the CNI VERSION command; pod networking will fail until the plugin implements CNI.\n' >&2
fi
kubectl --kubeconfig "$kubeconfig" --context "kind-$cluster_name" get nodes
