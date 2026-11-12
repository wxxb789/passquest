# 0010 — Provider choice and optional capabilities

Contract: [`../goals/archive/0010-provider-choice-and-optional-capabilities.md`](../goals/archive/0010-provider-choice-and-optional-capabilities.md)
Prerequisite: 0005 (BYOK tutor). Owner decisions: Q-0010-01 … Q-0010-05.

## Outcome

A registry-driven provider/model surface (`passquest model`) that separates provider, model,
capability and authorization; switches the local provider without touching practice state; verifies
one real text call per model; reports typed failures with one next action; counts provider calls per
provider and prints the sum; and allows one bounded, consent-equal failover retry.

## Public shape

### Registry

`internal/provider/registry.json`, embedded with `go:embed`, version `1`:

```json
{
  "version": 1,
  "providers": [
    {
      "id": "openai",
      "name": "OpenAI",
      "endpoint": "https://api.openai.com/v1/chat/completions",
      "endpoint_shape": "openai-chat-completions",
      "credential_env": "PASSQUEST_OPENAI_API_KEY",
      "models": [
        {"id": "gpt-4o-mini", "label": "GPT-4o mini", "default": true,
         "capabilities": ["text.chat", "tool.calls", "vision"]}
      ]
    }
  ]
}
```

- Providers: `openai`, `qwen`, `doubao`, `deepseek`, `gemini`, `custom`.
- Built-ins list one default (cheapest documented text model) plus a small number of other
  documented text models. Every entry is `declared` until `model test` verifies `text.chat`.
- `custom` has no fixed endpoint and no built-in model. `model use custom/<model> --endpoint URL`
  supplies both. A custom endpoint must be an absolute `http(s)` `/chat/completions` URL with no
  userinfo, query or fragment; `http` is allowed only on loopback, matching 0005.
- Credential env is per provider (`PASSQUEST_OPENAI_API_KEY`, …). `custom` additionally accepts
  `PASSQUEST_TUTOR_API_KEY`. The legacy configuration (no provider selected) keeps
  `PASSQUEST_TUTOR_API_KEY`.
- `PASSQUEST_PROVIDER_REGISTRY` overrides the embedded file for tests and operators. It is not a
  learner setting and never points at a network URL.
- Adding a provider or model is a data edit plus a registry test.

Capabilities: `text.chat`, `tool.calls`, `vision`, `search`, `speech.synthesis`. The contract names the
first four; clause 7 requires speech to be discoverable, so `speech.synthesis` is the fifth
discoverable axis. A capability not declared by a model renders `unsupported`; a declared one without
a verification event renders `declared,unverified`; a verified one renders `verified@timestamp`; a
failed one renders `failed:<type>@timestamp`.

### Commands

```bash
passquest model list   [--db PATH] [--json]
passquest model use    <provider>/<model> [--endpoint URL] [--db PATH] [--json]
passquest model test   <provider>/<model> [--db PATH] [--json]
passquest model budget [--db PATH] [--json]
```

- `model list`: one row per model with `provider`, `model`, `auth` (`present` / `no_credential`) and
  one capability state per capability. The active row is marked. The daily-budget block prints every
  provider counter and the sum (`providers × per-provider limit`).
- `model use`: writes one `tutor_config` row (`provider`, `model`, `endpoint`); leaves attempts,
  presentations, tutor calls and agent runs untouched. `--endpoint` is accepted only for `custom`.
- `model test`: one minimal non-streaming `chat/completions` call through the existing Node tutor
  host; appends a provider-verification event with `verified` or `failed:<type>` and a UTC timestamp.
  It reserves one slot in that provider's daily budget (a real provider call). No key means
  `failed:no_credential` and no reservation. Only `text.chat` has a probe in this increment;
  `tool.calls`, `vision`, `search` and `speech.synthesis` remain unverified, so no feature offers
  them as working.
- `model budget`: per-provider `calls/limit` for the current UTC day and the sum; the ceiling line is
  printed as `N providers × M = N*M`. Providers come from the registry plus `unattributed` legacy
  calls.

Failure vocabulary and the one action each names:

| type | next action |
|---|---|
| `no_credential` | export the provider's credential env var |
| `unauthorized` | replace the provider key |
| `rate_limited` | wait for the provider quota window |
| `model_missing` | choose a model from `model list` or edit the registry |
| `endpoint_unreachable` | check the endpoint URL and network |
| `invalid_response` | check the endpoint is an OpenAI-compatible chat/completions API |
| `provider_error` (extra, 5xx) | retry later |
| `agent_unavailable` (local host) | install Node and the agent dependencies |

### Integration with existing surfaces

- `tutor ask` and `agent start` require `text.chat = verified` **only when a registry provider is
  active**. The goal-0005 legacy configuration (no `model use`) keeps its existing unverified path
  for backward compatibility; it is labelled `legacy` in `model list`.
- `tutor ask` resolves the credential from the active provider's env var, reserves that provider's
  budget, and records `provider` on the call.
- `agent start` records the provider with the run; `resume` keeps the recorded model and reports it.
  Agent runs never fail over: 0006's recorded-model rule is stronger than a silent mid-run switch.
- `tutor consent SOURCE --reviewer NAME [--provider ID]` adds an optional provider scope. Without
  `--provider` the event is global (0005 behavior). A provider-specific event overrides a global one
  for that provider.
- `note propose --refine` uses the active provider's credential/budget and records the provider.

### Failover (tutor ask only)

After a retryable failure (`endpoint_unreachable`, `rate_limited`, `provider_error`) on provider A,
`tutor ask` may make **one** retry on provider B if and only if:

1. B's default model has `text.chat = verified`;
2. B's consent scope for the request is exactly equal to A's (same source id, same grant);
3. B has budget left;
4. B ≠ A;
5. B's credential is present (otherwise the retry cannot run).

Output always contains `failover: <A> → <B> (reason: <type>)` on success and
`failover refused: <A> → <B> (reason: <reason>)` on refusal. The failed call, the failover event and
the retried call are appended to the provider call ledger (`tutor_calls` + `provider_failover_events`).
Any consent difference refuses with `consent_scope_differs` and names the source. There is no second
retry.

## Edge-case semantics

- A capability that is unverified is never described as working. Declared-but-untested rendering is
  always `declared,unverified`; an absent capability is `unsupported`, never `no_credential`.
- Auth state and capability state are independent columns. A row with no key still prints the
  provider, model and declared capabilities.
- `model use` and `model test` never write `attempts`, `presentations`, `agent_runs` or answer keys.
  All provider failures leave `report` and `next` byte-identical.
- `model test` records a failure after the host is invoked, including a transport failure. A failure
  before the host (no key, no budget, unknown model, invalid custom endpoint) records only the
  no-credential case; the others are input/budget refusals and write no verification event.
- Verification is append-only and keyed by `(provider, model, capability)`; the latest event wins.
- A stale verification is never deleted by a later failure; it is superseded by the newer event.
- Failover never widens consent: a target with more grants than the source request is also refused.
- The budget is local-DB and UTC-day scoped. A crash after a reservation can under-spend, never
  over-spend. Legacy `provider = ''` rows count as `unattributed`, not against a named provider.
- Registry validation fails loudly on an unknown version, duplicate provider/model, unknown
  capability, zero or multiple defaults, an invalid endpoint, or an invalid env name.

## Evidence plan

New gate `harness/gates/provider-smoke.ts` (21st gate) drives the real CLI and the real Node tutor
host against loopback stubs:

1. Registry drives `model list`; a temp registry entry appears in the next listing.
2. With no keys, every row prints provider/model and `no_credential`.
3. `model use` then `model list` shows the active model; `report`/`next` are byte-identical before
   and after (seed one practice attempt first).
4. `model test` against a loopback stub records `verified` with a timestamp; a dead port records
   `endpoint_unreachable` with a timestamp.
5. One transcript per failure type (`no_credential`, `unauthorized`, `rate_limited`,
   `model_missing`, `endpoint_unreachable`, `invalid_response`), each with its next action, and
   `report`/`next` unchanged after every failure.
6. Agent run retention: start a paused run on provider A, `model use` B, resume; status/show/resume
   report A's recorded model.
7. No key material in `model list` output or an export; the secrets gate stays green.
8. `tutor ask` succeeds with `text.chat` verified while `search`/`speech.synthesis` are
   `declared,unverified`; `practice` succeeds independently.
9. Two providers, `--daily-calls 1`: one call each, then a refusal at each provider's own limit;
   `model budget` prints both counters and the sum.
10. Failover: A returns 429 and B (same consent, verified) completes, printing the line and recording
    both calls; with A granted and B withheld for a private source, B is not called and the refusal
    names the consent difference.

Go tests: registry load/validate/add; failure classification; provider-scoped reservations/counts;
verification append/latest; consent precedence; failover event append; schema migration v15; CLI
list/use/test/budget with a fake transport.

`bash scripts/gate.sh` runs on the final tree. The evidence record
`docs/evidence/0010-provider-choice-and-optional-capabilities.md` carries the real commands, counts,
timestamps and exit codes.

## Trade-offs and ambiguous points

- **Speech capability added to the matrix.** The contract's parenthetical lists four capabilities but
  clause 7 says speech is discoverable. Recommendation taken: add `speech.synthesis` as a fifth,
  always `declared`/`unsupported` for the shipped text models, never offered. Removing it is a data
  edit plus a registry test.
- **Only `text.chat` is probed.** The existing tutor host validates an exact five-key chat body and
  has no tool/image path. Adding probes for `tool.calls`/`vision` would widen the host protocol and
  the goal's surface without a contract acceptance row. Recommendation taken: verify only the
  capability the text path uses; the optional capabilities stay discoverable and unverified, which
  is exactly the state clause 7 requires before they may be offered.
- **Legacy tutor configuration stays permissive.** A learner who configured the 0005 endpoint by
  hand is not forced to re-verify; the registry path is strict. This preserves 0005 tests and
  documented behavior. Reverse by requiring verification whenever an endpoint is configured.
- **Failover is tutor-only.** Agent runs keep their recorded model; durable resumption is their
  retry mechanism. Reverse by adding a failover branch inside the agent host, which would need a
  recorded-model hand-off decision first.
- **Failover requires a credential on the target** even though clause 8 does not list it. A retry
  without a key cannot succeed; the refusal names `no_credential`.
- **`model test` consumes budget.** It makes a real provider call. This keeps accounting honest and
  is the surface used to prove per-provider refusals. Reverse by exempting probes from the ledger.
- **Default models are declarations, not verified facts.** No provider is contacted in CI. Model
  ids, endpoint shapes and capability declarations come from provider documentation at the time the
  registry was authored; the ledger says `declared,unverified` until the owner runs `model test`.

## Out of scope

Unofficial subscription-token extraction, account login (Q-0010-02), cross-consent failover,
per-provider price claims or quality rankings, speech/vision feature execution, provider-hosted
search (0008 remains a separate adapter), and desktop rendering (0011). No third-party provider SDK
is added; every provider uses the one OpenAI-compatible transport.
