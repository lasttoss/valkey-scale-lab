#!/usr/bin/env bash
# Lint, render both branches, and validate the manifests. The chart's value surface is small and
# every value in it is a mode: a chart whose only tested path is the default one has an untested
# path for every value a user sets.
set -euo pipefail
cd "$(dirname "$0")/.."

HELM=${HELM:-helm}
KUBECONFORM=${KUBECONFORM:-kubeconform}
if ! command -v "$KUBECONFORM" >/dev/null 2>&1; then
  echo "  kubeconform not found; installing to .cache/bin"
  mkdir -p .cache/bin
  ver=v0.6.7
  curl -sSfL "https://github.com/yannh/kubeconform/releases/download/${ver}/kubeconform-linux-amd64.tar.gz" \
    | tar xz -C .cache/bin kubeconform
  KUBECONFORM="$PWD/.cache/bin/kubeconform"
fi

"$HELM" lint charts/valkey-scale-lab --strict

n=0
for mode in leak fixed; do
  out=$("$HELM" template valkey-scale-lab charts/valkey-scale-lab --set mode="$mode")
  echo "$out" | "$KUBECONFORM" -strict -summary -kubernetes-version 1.29.0 - >/dev/null
  case "$mode" in
    # A leak-mode gate that succeeds is a gate that is not running the leak.
    leak) echo "$out" | grep -q -- '-mode=leak' || { echo "  mode=leak did not reach the Job args"; exit 1; } ;;
    fixed) echo "$out" | grep -q -- '-mode=fixed' || { echo "  mode=fixed did not reach the Job args"; exit 1; } ;;
  esac
  echo "  mode=$mode ✓"
  n=$((n + 1))
done
echo "  $n/$n branches render and validate"
