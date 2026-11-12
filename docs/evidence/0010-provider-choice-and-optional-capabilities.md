# 0010 — Provider choice and optional capabilities: measured evidence

Contract: [`../goals/archive/0010-provider-choice-and-optional-capabilities.md`](../goals/archive/0010-provider-choice-and-optional-capabilities.md)
Plan: [`../plans/2026-11-12-2245-0010-provider-choice-and-optional-capabilities.md`](../plans/2026-11-12-2245-0010-provider-choice-and-optional-capabilities.md)

Environment: Linux, Go 1.27, Bun harness, Node host, no external network and no real provider
credential. Every provider call in this record is a loopback stub. A real owner `model test` remains
owner evidence (Q-0010-01).

## Gate runs

| Command | Result |
|---|---|
| `go test ./...` | PASS, 18 packages, 759 tests, 0 skipped |
| `bash scripts/gate.sh` | PASS, 21/21 gates, 0 failed, 0 skipped |
| `provider-smoke` alone | PASS, 92 assertions |
| `harness self-test` | PASS, 32 cases, 20/20 gates |

## Acceptance → evidence

| # | Acceptance | Evidence in this run |
|---|---|---|
| 1 | The registry drives behaviour | `provider-smoke` writes a temp registry, runs `model list --json`, then adds a `gamma` entry and reruns; the new provider appears. `internal/provider` tests load the embedded registry, add an entry and reject duplicate/unknown/invalid data. |
| 2 | The four axes stay separate | With no provider key, `model list` prints the provider and model rows with `auth: no_credential`; `text.chat` is `declared` and `search` is `unsupported`. `internal/cli/model_test.go` pins the same document. |
| 3 | Switching is local and reversible | `model use alpha/alpha-text` then `model list` shows the active pair. `provider-smoke` clones one seeded database, runs `next`/`report` on the clone and again after `model use`; after masking the temporary database path, both documents are identical. |
| 4 | Verification is real | `model test alpha/alpha-text` against a loopback chat-completions stub records `result: verified` with a timestamp; `model test dead/dead-text` against a closed loopback port records `failed:endpoint_unreachable` with a timestamp. Both come from the real CLI + Node host. |
| 5 | Failures are typed and actionable | One loopback transcript per type: `no_credential`, `unauthorized`, `rate_limited`, `model_missing`, `invalid_response`, `provider_error`; each document carries `failure.action`. A dead endpoint supplies `endpoint_unreachable`. The attempt report is byte-identical after all failures. |
| 6 | A run keeps its model | `provider-smoke` starts a paused agent run on `alpha/alpha-text`, switches to `beta/beta-text`, resumes, and checks `run.model == alpha-text` and `run.provider == alpha` in the resume and status documents. |
| 7 | Credentials never leak | The loopback stub receives the generated credential; `model list`, `tutor ask`, `practice` and the collected command output are checked for the value and do not contain it. The secrets gate scans the working tree. |
| 8 | Optional features cannot block text | `alpha-opt` declares `search` and `speech.synthesis` but only `text.chat` is verified; `tutor ask` and `practice` both succeed, and the optional capabilities remain `declared,unverified`. |
| 9 | Budget is per provider and the total is visible | With `--daily-calls 1` and two registry providers, each provider's first `model test` succeeds and its second refuses at that provider's own limit. `model budget` prints `total_calls: 2`, `total_limit: 2`, `ceiling: "2 providers × 1 = 2"`, and both counter rows. |
| 10 | Failover respects consent | `tutor ask` on provider `alpha` with a 429 retries once on verified `beta` with equal consent, prints `failover: alpha → beta (reason: rate_limited)`, and records both calls (`provider`, `failover_from`). With a private source granted for `alpha` only, the same failure refuses: `failover.applied == false`, `refusal == "consent_scope_differs"`, the detail names the source, and the stub receives no `beta` call. |

## What the loopback proves and what it does not

- It proves the registry, the four-axis listing, the transport, the typed failure mapping, the
  verification ledger, the per-provider reservation/count SQL, the bounded failover branch and the
  consent comparison.
- It does not prove a real provider's model id, capability or quota. Those stay `declared,unverified`
  until the owner exports a key and runs `model test`; Q-0010-01 records the default.
- Only `text.chat` is probed (Q-0010-07). `tool.calls`, `vision`, `search` and `speech.synthesis`
  remain discoverable and unverified, so no feature offers them as working.
- The budget is per local database. Two databases have two budgets.
