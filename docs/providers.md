# Provider and model choice

Goal 0010 turns one BYOK endpoint into a registry-driven choice of OpenAI, Qwen, Doubao, DeepSeek,
Gemini or a custom OpenAI-compatible endpoint. Provider, model, capability and authorization are four
**separate** facts and are never collapsed into one word.

The registry is data, not a vendor SDK: `internal/provider/registry.json` is embedded at build time,
versioned (`version: 1`) and validated on load. Adding a provider or a model is a data edit plus a
registry test. `PASSQUEST_PROVIDER_REGISTRY` may point at another local file for a test or a deliberate
operator edit; it is not a learner setting.

## Commands

```bash
passquest model list                     # providers, models, capabilities, authorization
passquest model use openai/gpt-4o-mini   # one local setting; practice state is untouched
passquest model test openai/gpt-4o-mini  # one minimal call; records verified or failed:<type>
passquest model budget                   # per-provider counters, their sum and the ceiling
passquest model use custom/MODEL --endpoint https://host/v1/chat/completions
```

`model use` writes the `tutor_config` row (`provider`, `model`, `endpoint`). A hand-edited
`tutor config --endpoint --model` clears the registry provider and stays the goal-0005 legacy path.
`model use` does not verify, change an attempt or alter `report`/`next`.

`model test` sends one minimal non-streaming chat-completions request through the same Node host as
`tutor ask` and appends a timestamped verification event. It spends one slot in that provider's daily
budget because it is a real provider call. A missing credential is recorded as
`failed:no_credential` without a call or a budget slot.

## Four axes

| Axis | Where it comes from | What it never means |
|---|---|---|
| Provider | registry entry (`openai`, `qwen`, `doubao`, `deepseek`, `gemini`, `custom`) | Not a capability claim |
| Model | registry model id, or the custom model id | Not an authorization claim |
| Capability | `text.chat`, `tool.calls`, `vision`, `search`, `speech.synthesis` | `declared` is not `verified` |
| Authorization | presence of the provider's credential variable | Missing key is `no_credential`, never `unsupported` |

A capability is rendered in exactly one of these states:

- `verified@timestamp` — a `model test` call succeeded.
- `failed:<type>@timestamp` — a `model test` call failed; the list also carries the next action in JSON.
- `declared,unverified` — the registry declares it but nothing has tested it.
- `unsupported` — the model does not declare it.

Only `text.chat` has a probe in this increment. `tool.calls`, `vision`, `search` and
`speech.synthesis` stay discoverable but unverified, so no feature offers them as working. Search
(goal 0008) remains a separate bounded adapter, not a provider model capability.

## Credentials

The credential variable is per provider:

| Provider | Variable |
|---|---|
| `openai` | `PASSQUEST_OPENAI_API_KEY` |
| `qwen` | `PASSQUEST_QWEN_API_KEY` |
| `doubao` | `PASSQUEST_DOUBAO_API_KEY` |
| `deepseek` | `PASSQUEST_DEEPSEEK_API_KEY` |
| `gemini` | `PASSQUEST_GEMINI_API_KEY` |
| `custom` | `PASSQUEST_CUSTOM_API_KEY`, or `PASSQUEST_TUTOR_API_KEY` as a fallback |
| legacy manual config | `PASSQUEST_TUTOR_API_KEY` |

The value is read only by the Node host. It is never stored in SQLite, argv, output, a call record,
a generated command or the repository. The Go transport copies the selected provider variable into
the host's credential slot at spawn time, so it never appears in the request document.

## Verification and the text path

When a registry provider is active, `tutor ask` and `agent start` require `text.chat = verified` for
that exact provider/model. An unverified model is refused with `capability_unverified` and the one
action `passquest model test <provider>/<model>`. `tutor preview` still works and prints the state.
The goal-0005 hand-configured path (no registry provider) remains the documented legacy exception.

A provider failure is typed and names one action:

| Type | Next action |
|---|---|
| `no_credential` | export the provider's credential variable |
| `unauthorized` | replace the provider key |
| `rate_limited` | wait for the provider quota window |
| `model_missing` | choose a model from `model list`, or update the registry |
| `endpoint_unreachable` | check the endpoint URL and this machine's network |
| `invalid_response` | point the endpoint at an OpenAI-compatible `/chat/completions` API |
| `provider_error` | retry later; the provider reported a server error |
| `agent_unavailable` | install Node and the agent dependencies |
| `config_error` | check the endpoint and the Pass Quest build |

A failed call writes no attempt, grade, answer key or practice state.

## Budget

The goal-0005 daily value is now **one counter per provider**, in UTC, counted from recorded calls,
in-flight reservations, note refinements and model probes that actually reached a provider.
`model list`, `model budget` and `tutor history` print every provider counter and the sum. The sum is
the real ceiling:

```text
ceiling = providers × per-provider limit
```

A refusal at one provider's own limit leaves every other provider's budget usable. Legacy rows with
no provider are shown as `unattributed`; they count in the sum but are not charged to a named
provider. A reservation that is never finalized keeps its slot for the rest of the UTC day, which can
under-spend and never over-spend.

## Bounded failover

`tutor ask` may retry **once** on another provider after a retryable failure (`endpoint_unreachable`,
`rate_limited`, `provider_error`). It does so only when all of these hold:

1. the target's default model has `text.chat = verified`;
2. the target's consent scope is exactly equal to the failing request's (same source, same grant);
3. the target has budget left;
4. the target is not the provider that failed;
5. the target's credential is present (a retry without a key cannot succeed).

Output is exactly one of:

```text
failover: alpha → beta (reason: rate_limited)
failover refused: alpha → beta (reason: consent_scope_differs; source src-x is authorized for alpha but withheld for beta)
```

The failed call, the retry and the decision are appended to the provider ledger
(`tutor_calls.failover_from`, `provider_failover_events`). A consent difference never widens what
private text can reach. There is no second retry. Agent runs do not fail over: goal 0006 records the
model and provider at start, and `resume` reports and uses that recorded pair.

## Registry defaults

The shipped registry lists one default text model per built-in provider and a small number of other
documented text models. Every entry is `declared,unverified` until the owner runs `model test` with a
real key. No price, quality ranking or account entitlement is claimed. Model ids and endpoint shapes
are hand-maintained; a provider that changes its API fails as `invalid_response` until the data file
is updated.

## Limits

- No real provider is contacted by CI. Capability claims are registry declarations until a real
  `model test`.
- Only `text.chat` is probed. Optional capabilities are discoverable but have no execution path in
  this goal.
- Budget accounting is per local database, not per account. Two databases have two budgets.
- Failover is as precise as the recorded consent scope. Consent is per source and optionally per
  provider; a global grant applies to every provider.
- Speech/image features, account login and subscription-token extraction are out of scope.

Measured evidence: [`evidence/0010-provider-choice-and-optional-capabilities.md`](evidence/0010-provider-choice-and-optional-capabilities.md).
