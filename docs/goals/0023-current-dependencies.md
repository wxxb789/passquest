# 0023 — Every dependency is on its newest served release

**State:** Ready (defined 2026-11-26)
**Prerequisite:** None. It takes over item 5 of [0021](./archive/0021-responses-api.md), which the owner moved here on 2026-11-26. Do not run it at the same time as another goal that changes the package build or the lockfile on one tree.

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

The app is built from four dependency sets:

- **Go modules** in `go.mod`: MyGo (the desktop shell), the pure-Go SQLite driver, `golang.org/x/*` and their indirect modules.
- **npm packages** in `package.json`, `agent/package.json` and `bun.lock`. Among them are the four `@earendil-works` Pi packages the agent hosts bundle (`chord`, `pi-ai`, `pi-durable`, `pi-schema`, all at 1.5.2). Upstream released 1.6.0 on 2026-11-20 and 1.6.1 on 2026-11-25. 1.6.0 updates `undici` for six security advisories, and it changes how system-role messages are sent to non-OpenAI providers that use the OpenAI-compatible format.
- **Toolchains** pinned in `mise.toml`, `.github/workflows/*.yml` and `harness/lib/package.ts`: Go, Bun, and the Node runtime the package ships (Q-0013-03; 26.13.1 since goal 0020).
- **GitHub Actions** the workflows use. Every recent run warns that `actions/checkout@v4`, `actions/setup-go@v5`, `actions/setup-node@v4`, `actions/upload-artifact@v4` and `actions/download-artifact@v4` target the deprecated Node.js 20 and are forced onto Node 24.

The development machine reaches npm only through a corporate feed proxy that lags the public registry by about seven days. The owner's standing rule: never switch the registry, never downgrade a locked version.

## Intended outcome

Every dependency the repository pins is on the newest release its source serves, the app behaves exactly as before for the learner, and the security fixes those releases carry are in the shipped package. A learner sees no change except what an upgrade's own release notes require, and each such change is listed.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and every existing gate stays green on all three OSes.

1. **An inventory says what moved.** One table in the evidence record lists every direct dependency in each of the four sets: old version, new version, where the new version was read from (the corporate npm feed's listing, the Go module proxy, the toolchain's release page, the action's releases), and the upstream notes that matter to this app. A dependency left where it is names its reason (already newest, or a named blocker). Evidence: the table and the listings it quotes.
2. **The Pi packages are current.** All four direct `@earendil-works` dependencies move together to the newest version the corporate feed serves, at least 1.6.0. The lockfile is resolved from that feed, including the transitive `pi-telemetry` and `pi-codemode` entries. The run shows what the system-role change does to the durable host's requests, as a recorded request-body comparison before and after for both endpoint formats. Evidence: the lockfile diff, the feed's version listing, and the agent and package smokes on three OSes.
3. **Go modules, npm packages, toolchains and actions are current.** Each moves to its newest served release, majors included, with any code or config change it needs. A major upgrade that changes behaviour the app relies on is either adapted, with tests, or held back with the reason recorded in a Q-0023-NN entry. The deprecation warning is gone from the workflow runs. Evidence: the diffs, the full gate on three OSes, and one workflow log without the warning.
4. **Behaviour is unchanged.** CLI output and exit codes, goldens, host request bodies (apart from the listed Pi change), the database schema, the window's study flows and the title area are as before. The 0019 frontend bench reads no regression, and the package size is read before and after. Evidence: the unchanged gates and goldens, the bench record, and the per-OS package sizes.
5. **The pins agree.** Every place that names a toolchain version (`mise.toml`, the workflows, `harness/lib/package.ts`, the install guides) names the same one, checked by the existing pin-agreement check, which is extended if a new pin appears. Evidence: the package smoke's pin line.

Constraints:

- Use only the configured feeds. Never switch a registry or proxy, and never move a package to an older version than the lock holds.
- No telemetry. Pi's telemetry package stays a transitive dependency and is not turned on.
- No change to scoring, consent, the database schema or the internal-only source rules.
- MyGo moves only together with goal 0022's title-bar checks passing unchanged on three OSes. If a MyGo release changes the hidden-title-bar contract, adapting to it is part of this goal, and the per-OS room readings are recorded again.

Stop and report at once if, at the start, something only the owner can supply is missing. If the corporate feed still does not serve `@earendil-works` 1.6.0 or newer when the run starts, do not hand-build or vendor the packages (the owner declined that on 2026-11-26). Finish items 1 and 3–5 for everything else, then report item 2 as blocked by the feed, with the listing as evidence, so the item can be re-run when the feed catches up. Take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md` (Q-0023-NN). Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits.

## Out of scope

- New features, refactors and dependency replacements (swapping one library for another).
- Telemetry of any kind.
- Signing and in-app update (goal 0018 and later).
