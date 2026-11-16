#!/usr/bin/env bash
# Purpose: collect a Linux floor record inside a digest-pinned container.
#
# The evidence model records the environment each proof was produced under. A
# Node 22, Python 3.12 or Linux Go integration proof therefore cannot be
# produced on the macOS default host, and the collector refuses a profile the
# live host does not satisfy. This helper runs the collector *inside* the
# container that does satisfy the profile, so the recorded environment is the
# real probe: no profile is asserted by hand.
#
# Images are pinned by digest for the linux/arm64 platform this host's colima VM
# runs. The previous CI legs ran on linux/x64; emulating x64 here crashes the
# bundled esbuild service, so an emulated record would be an artefact of the
# emulator rather than a floor proof. The profile therefore declares arm64, and
# the platform is part of what the record claims. The container publishes no port; a suite that needs DynamoDB
# Local is given the endpoint of a backend this run started on the host, reached
# through the host gateway.
#
# Usage:
#   scripts/evidence/floor-run.sh --profile floor-py3.12 --gate py
#   scripts/evidence/floor-run.sh --profile floor-node22 --gate ts
#   scripts/evidence/floor-run.sh --profile linux-arm64 --gate go-integration \
#     --dynamodb-endpoint http://host.docker.internal:55321
#
# Options:
#   --profile <id>            required: floor-node22, floor-py3.12 or linux-arm64
#   --gate <id>               passed to the collector; repeatable
#   --all                     passed to the collector
#   --force                   passed to the collector
#   --dry-run                 passed to the collector
#   --host-network            run the container on the host network, so a
#                             loopback-only local backend stays loopback-only and
#                             is still reachable (no port is published)
#   --dynamodb-endpoint URL   exported to the suite as DYNAMODB_ENDPOINT
#   --clean-host-state        remove host-built dependency trees and outputs
#                             first, so a foreign-architecture tree is never
#                             reused inside the container
#   -- <args...>              any remaining collector arguments
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_root}"

profile=""
dynamodb_endpoint=""
clean_host_state="false"
host_network="false"
collector_args=()

usage() {
  sed -n '2,40p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      profile="${2:-}"
      shift 2
      ;;
    --gate|--review-ref)
      collector_args+=("$1" "${2:-}")
      shift 2
      ;;
    --all|--force|--dry-run)
      collector_args+=("$1")
      shift
      ;;
    --dynamodb-endpoint)
      dynamodb_endpoint="${2:-}"
      shift 2
      ;;
    --clean-host-state)
      clean_host_state="true"
      shift
      ;;
    --host-network)
      host_network="true"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --)
      shift
      collector_args+=("$@")
      break
      ;;
    *)
      echo "floor-run: FAIL (unknown argument: $1)" >&2
      exit 2
      ;;
  esac
done

if [[ -z "${profile}" ]]; then
  echo "floor-run: FAIL (--profile is required)" >&2
  exit 2
fi

# Digests resolved from Docker Hub on 2026-11-15 for the linux/arm64 manifests
# this host runs. A moved tag cannot change what a record proves.
case "${profile}" in
  floor-node22)
    image="node:22-bookworm@sha256:f796bda8a91392fe7b5c974ff42abaa5cc801eb6987dca967f1db8be493e0ee3"
    setup=""
    ;;
  floor-py3.12)
    image="python:3.12-bookworm@sha256:3dc01f342e31ad8de39338d14ab12de0090c9102b34f75ca9774ff1f4f5e904e"
    # The Python package suite builds generated-import fixtures with the Go CLI,
    # so this floor carries the same Go pin the repository tracks. The pin is
    # read from go.mod at run time instead of restated here.
    setup="$(cat <<'FLOOR_SETUP'
if [ ! -x /usr/local/go/bin/go ]; then
  go_pin="$(awk '/^toolchain /{print $2}' go.mod)"
  go_arch="$(uname -m)"
  case "${go_arch}" in
    aarch64) go_arch="arm64" ;;
    x86_64) go_arch="amd64" ;;
  esac
  curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/${go_pin}.linux-${go_arch}.tar.gz"
  tar -C /usr/local -xzf /tmp/go.tgz
fi
export PATH="/usr/local/go/bin:${PATH}"
python3 -m pip install --no-cache-dir 'uv==0.9.26'
FLOOR_SETUP
)"
    ;;
  linux-arm64)
    image="golang:1.27-bookworm@sha256:8d18482ddade3123e2288535ea760ac7027541b54b3e655802327607306f5da9"
    setup=""
    ;;
  *)
    echo "floor-run: FAIL (profile ${profile} has no digest-pinned Linux producer)" >&2
    exit 2
    ;;
esac

command -v docker >/dev/null 2>&1 || {
  echo "floor-run: FAIL (docker is required to run a floor proof)" >&2
  exit 1
}

if [[ "${clean_host_state}" == "true" ]]; then
  # Dependency trees and build outputs are architecture-bound and git-ignored.
  # A Darwin tree cannot answer a Linux run, and a rebuild is the only honest
  # way to prove the floor.
  rm -rf ts/node_modules ts/dist ts/coverage py/.venv py/build py/dist
fi

host_uid="$(id -u)"
host_gid="$(id -g)"

# Mount the checkout at its own absolute path. This repository is a submodule,
# so its `.git` is a file pointing at the parent work tree's git directory: that
# path has to exist inside the container too, and it is mounted read-only so a
# gate inside the container can read history but cannot write outside the
# checkout it was given.
mount_args=()
if [[ -f "${repo_root}/.git" ]]; then
  gitdir_rel="$(sed -n 's/^gitdir: //p' "${repo_root}/.git")"
  if [[ -n "${gitdir_rel}" ]]; then
    gitdir_abs="$(cd "${repo_root}" && realpath "${gitdir_rel}")"
    host_root="${gitdir_abs%%/.git/*}"
    if [[ -n "${host_root}" && -d "${host_root}" ]]; then
      mount_args+=(--volume "${host_root}:${host_root}:ro")
    fi
  fi
fi
mount_args+=(--volume "${repo_root}:${repo_root}")

# The Go module cache is source, not build output: reuse the production host's
# cache read-only so a floor run does not depend on a fresh module download.
go_cache="${GOMODCACHE:-$(go env GOMODCACHE 2>/dev/null || true)}"
cache_args=()
if [[ -n "${go_cache}" && -d "${go_cache}" ]]; then
  cache_args=(--volume "${go_cache}:/go/pkg/mod:ro" -e "GOMODCACHE=/go/pkg/mod" -e "GOPATH=/go")
fi

network_args=()
if [[ "${host_network}" == "true" ]]; then
  # The suite's DynamoDB Local is a loopback-only local backend by policy, so
  # the container reaches it on the host's loopback namespace instead of a
  # published port.
  network_args=(--network host)
fi

docker_env=(
  -e "DYNAMODB_ENDPOINT=${dynamodb_endpoint:-http://127.0.0.1:8000}"
  -e "AWS_REGION=${AWS_REGION:-us-east-1}"
  -e "AWS_DEFAULT_REGION=${AWS_DEFAULT_REGION:-us-east-1}"
  -e "AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-dummy}"
  -e "AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-dummy}"
  # The checkout is host-owned; git inside the container is not. Pin the trust
  # explicitly instead of disabling the check.
  -e "GIT_CONFIG_COUNT=1"
  -e "GIT_CONFIG_KEY_0=safe.directory"
  -e "GIT_CONFIG_VALUE_0=*"
)

cleanup_ownership="for path in gov-infra/evidence/validation gov-infra/evidence/validation/local ts/node_modules ts/dist ts/coverage py/.venv py/build py/dist; do
    if [ -e \"\${path}\" ]; then chown -R ${host_uid}:${host_gid} \"\${path}\" || true; fi
  done"

# The profile is the point of this helper: always ask the collector for it, so a
# gate whose first profile is the default host cannot be recorded by accident
# inside a floor container.
collector="python3 scripts/evidence/validation_evidence.py collect --profile ${profile} ${collector_args[*]:-}"

echo "floor-run: profile=${profile} image=${image} platform=linux/arm64" >&2
post_cleanup() {
  if [[ "${clean_host_state}" == "true" ]]; then
    # Do not leave a foreign-architecture dependency tree behind: the next run
    # on the default host must build its own, or it would answer a Darwin gate
    # with a Linux tree.
    rm -rf ts/node_modules ts/dist ts/coverage py/.venv py/build py/dist
  fi
}
trap post_cleanup EXIT
docker run --rm \
  --platform linux/arm64 \
  "${network_args[@]}" \
  "${mount_args[@]}" \
  "${cache_args[@]}" \
  --workdir "${repo_root}" \
  "${docker_env[@]}" \
  --entrypoint bash \
  "${image}" -c "set -euo pipefail
# A login shell would reset PATH from /etc/profile and lose the image's own
# toolchain directory, so run a plain shell and name the toolchain explicitly.
export PATH=\"/usr/local/go/bin:${PATH}\"
${setup:+${setup}
}${collector}
${cleanup_ownership}"
