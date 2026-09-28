#!/usr/bin/env bash
# Shared bounded-retry helper for network-dependent verifier steps.
#
# Upstream registries (pypi.org, registry.npmjs.org, vuln.go.dev) intermittently
# return 5xx or partial responses. A single attempt turns that transient
# upstream state into a red gate that this repository cannot fix by changing its
# own code (observed 2026-09-28: a pypi "503 Backend is unhealthy" response
# failed a gating SEC-2 run). These helpers retry with backoff and still fail
# closed: the final attempt's exit status is returned, so a genuine finding or a
# sustained outage stays red.
#
# Never use these to soften a threshold or to swallow a failure. They only
# absorb a bounded transient upstream window. Tooling that already retries
# internally (uv: UV_HTTP_RETRIES default 3; pip: --retries default 5) does not
# need this helper.

# theorydb_retry_bounded <attempts> <base_delay_seconds> <label> -- <command...>
#
# Runs <command...> up to <attempts> times. Between attempts it sleeps
# <base_delay_seconds>, then 3x that, and so on. Returns the exit status of the
# last attempt (0 only when an attempt succeeded).
theorydb_retry_bounded() {
  local attempts="$1"
  shift
  local base_delay="$1"
  shift
  local label="$1"
  shift
  if [[ "${1:-}" == "--" ]]; then
    shift
  fi

  if (( attempts < 1 )); then
    echo "${label}: invalid retry bound (${attempts}); refusing to run" >&2
    return 2
  fi

  local attempt=1
  local delay="${base_delay}"
  local status=1

  while :; do
    # `if` suspends errexit for the guarded command, so a failing attempt is
    # captured instead of aborting the retry loop.
    if "$@"; then
      return 0
    else
      status=$?
    fi

    if (( attempt >= attempts )); then
      echo "${label}: FAIL after ${attempts} attempt(s) (exit ${status}); failing closed" >&2
      return "${status}"
    fi

    echo "${label}: attempt ${attempt}/${attempts} failed (exit ${status}); retrying in ${delay}s" >&2
    sleep "${delay}"
    delay=$(( delay * 3 ))
    attempt=$(( attempt + 1 ))
  done
}
