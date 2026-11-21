# 0016 — A lean package that ships only what the app runs

**State:** Ready (defined 2026-11-20)
**Prerequisite:** 0013 — installable personal desktop app

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

Windows and macOS are the first-tier platforms; Linux is second-tier (owner decision 2026-11-20, Q-0000-07). The installed experience on the first-tier platforms should be at least as good as comparable unsigned desktop apps.

## Intended outcome

Today every package carries far more than the app runs. The v0.15.0 Windows build stages 12,774 files and 196 MB uncompressed, and its installer is 36.5 MB. Almost all of it is the agent hosts' `node_modules`: 88 packages, 12,768 files, 89 MB. Most of that is provider SDKs and other libraries that no host ever loads (`openai`, `@google/genai`, `@anthropic-ai/sdk`, AWS Bedrock, an image library), which `@earendil-works/pi-ai` pulls in. Only `agent/durable-host.mjs` imports npm packages at all, and of `pi-ai` it uses only the OpenAI-compatible completions API. A quick experiment bundled that host into a single 1.48 MB file, and the file started and answered the host protocol. That experiment is a lead, not a decision. The bundled Node 22 runtime (84 MB, Q-0013-03) is not in question here.

After this goal, each package ships only the agent code its hosts actually execute. The installer and disk image are smaller, they install fewer files, and the agent features work exactly as before. Each claim is proven on the installed package rather than only in the source tree.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the existing gates must stay green on Windows, macOS and Linux.

1. **Only code that runs is shipped.** The installed agent tree contains no third-party package that none of the hosts load. In particular, the provider SDKs listed above and their dependency trees are absent. Evidence: the package gate's contents check fails if such a package reappears, and a self-test case proves that check fails.
2. **The smaller package is measured, not asserted.** For each OS's CI package job, a before/after table gives the staged file count, the uncompressed size and the installer, disk image or AppImage size. "Before" is v0.15.0's package or main's CI run before the change. Evidence: the table in the evidence record, citing the CI runs.
3. **The study brief works in the installed app.** Today the package smoke drives the durable study brief only on its unavailable path. After this goal, the installed app on each OS completes a study brief against a loopback provider stub, using the package's own Node runtime and hosts. Evidence: the package smoke's new success case on all three OSes.
4. **Nothing else changes for the learner.** The coach, the note refinement, the search host, the Anki push, the durable brief's pause/resume/cancel behaviour and the host protocol behave exactly as before. Evidence: the existing agent, tutor, note, find, provider, desktop and package smokes pass unchanged, and no golden changes.
5. **Third-party notices travel with the code.** Any licence notice that a shipped third-party library's licence requires to accompany redistribution is still present in the package. Evidence: the contents check, plus a listing in the evidence record.
6. **Host start-up is reported.** The cold start of the durable host (process start to first protocol answer) on the package's own runtime is measured before and after, with sample count and spread. This is reported, not gated. Evidence: the numbers in the evidence record.

Constraints:

- The pinned Node 22 runtime stays as decided in Q-0013-03.
- The internal-only material scan and the secrets scan stay at least as strict as they are now.
- Development from the source tree keeps working for every host.
- Lock files keep resolving through the configured registry: do not swap the registry and do not downgrade the lock.
- Linux keeps being built and smoke-tested, but no Linux-only polish is in scope.
- No change to scoring, consent, budgets, the update check or the local-only exception.

Stop and report at once if, at the start, something only the owner can supply is missing. Otherwise take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md`. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits.

## Out of scope

- Replacing or trimming the Node runtime.
- New platforms or architectures.
- Signing, notarization and install-guide changes (goals 0017 and 0018).
