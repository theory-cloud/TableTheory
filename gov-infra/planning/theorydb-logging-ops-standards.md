# theorydb Logging & Operational Standards (Rubric v0.1)

This document defines what “acceptable logging” means for Theorydb as a library and how we avoid accidental data
exposure or crashy behavior in production environments.

## Scope

In scope for COM-6 (logging-ops):
- First-party Go library code tracked in git under the repo root, excluding:
  - `examples/`
  - `tests/`
  - `scripts/`
  - `contract-tests/`
  - `*_test.go`

Out of scope for COM-6:
- Example apps and verification harnesses (they may print to stdout by design), but they should still avoid real secrets.

## Allowed patterns (library code)

Allowed:
- `log.Printf(...)` for **rare** operational warnings/errors where returning an error is not possible (e.g., background
  refresh failures) or for **security warnings** when explicitly enabling unsafe features.

Required:
- Never log secrets/PII/CHD-like payloads (raw structs, attribute maps, request/response bodies).
- Sanitize identifiers before logging when they may contain ARNs/account IDs/tokens.

## Prohibited patterns (library code)

Disallowed in in-scope library code:
- `fmt.Print*` and `println(...)` (stdout printing is not an operational strategy for a library).
- `log.Fatal*` / `log.Panic*` (libraries must not terminate the host process).

## Examples vs library code

- Examples may use stdout for demonstration purposes, but should only print synthetic/non-sensitive data.
- Any logging/printing that could accidentally include real payloads must be avoided or redacted.

