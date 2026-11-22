# 0020 — The command line keeps the learner's data where the window does, and the package runs on Node 26

**State:** Ready (defined 2026-11-22)
**Prerequisite:** None. It can follow 0019; the two touch different parts of the tree. Do not run it at the same time as another goal that changes the package build on one tree.

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; recoverable, idempotent local state; no central account or cloud service. Follow [Core Design](../core-design.md).

Pass Quest has two entry points over one Go core. The desktop window is what a learner uses. The command line (`passquest`) is used by developers and CI. The installed app's binary is also a command line when it is started with arguments; the durable host relies on that for its `agent record-step` callback. The AI features run Node host scripts (`agent/*.mjs`), and every package carries those scripts plus a pinned Node runtime (Q-0013-03), so the features work without a Node installation.

## Intended outcome

Today the command line and the window disagree in two places.

- **Where the learner's data lives.** The window keeps its database in the per-user data directory (`%APPDATA%\Pass Quest` on Windows, `~/Library/Application Support/Pass Quest` on macOS, `$XDG_CONFIG_HOME/Pass Quest` on Linux; see `docs/install.md`). The command line defaults to `./.passquest/passquest.sqlite` in whatever directory it was started from (`internal/cli/cli.go`). The owner's decision (2026-11-22): the learner's database is personal, private data. It belongs in the user's own per-user directory, never in the working directory or the install location.
- **Where the AI runtime comes from.** The window passes the package's resource directory to the host resolution (`internal/hostpaths`). The command line does not, so the installed binary started with arguments falls back to `node` on `PATH`, and on macOS it cannot find the host scripts at all, because they live in `Contents/Resources/agent`.

The bundled runtime is also old. Node 22 (22.23.5) is in maintenance and reaches end of life on 2027-04-30. Node 26 is the Active LTS line (since 2026-10-28, supported until 2029-04-30). The owner wants the package, the development toolchain and CI on Node 26.

After this goal:

- every command line, from a source checkout or from an installed package, uses the same per-user data directory as the window by default, so the window and the command line see the same study evidence;
- the installed binary started with arguments runs the AI commands on the package's own hosts and Node runtime, on Windows, macOS and Linux;
- the package, `mise.toml` and CI run the latest Node 26 LTS release.

## Done when

Each item below is shown during the run with the evidence named. All of it must hold on the final tree, and the existing gates must stay green.

1. **One data directory.** With no `--db` and no `PASSQUEST_DB`, the command line opens the same `passquest.sqlite` the window opens, on all three platforms. The directory is computed in one place shared by both. `--db` and `PASSQUEST_DB` still take precedence, exactly as today. Evidence: tests for the default and for each override, and a CI case per OS that writes through one entry point and reads it back through the other.
2. **Existing data is never moved silently.** When a database exists in the old default location (`./.passquest/passquest.sqlite` in the working directory) and the per-user one does not, the command line does not move, copy or merge it. It tells the user, once per run, where the data now lives and how to move it themselves. Evidence: a test for the notice, and a test that the old file is untouched.
3. **The window and the command line are safe together.** Writing to the database from the command line while the window has it open neither corrupts it nor loses a write. When safe concurrent use cannot be guaranteed, the command line fails with a clear message instead. Evidence: a test that runs both against one database.
4. **The installed binary runs the AI commands from its own package.** The package's binary started with arguments resolves the package's `agent/` hosts and Node runtime as the window does, on Windows, macOS (`Contents/Resources`) and Linux, with no environment variable set. `PASSQUEST_NODE` and the `PASSQUEST_*_HOST` overrides still win. The source-checkout command line behaves as before. Evidence: a package smoke case per OS that runs one host-backed command through the installed binary with `PATH` cleared of `node`, and shows that the package's runtime ran it.
5. **Node 26 LTS everywhere.** The package bundles the latest Node 26 LTS release for every target, pinned with its official SHA-256 from `SHASUMS256.txt`. `mise.toml` and the CI workflows use Node 26. Q-0013-03 is revisited with the new pin and the reason. Evidence: the pins, the agent, tutor, note, find, assess, provider and package smokes green on all three OSes (the durable host's `node:sqlite` storage included), and the installed package's size before and after in the evidence record.
6. **The learner is told the truth.** `docs/install.md` and `docs/install.zh-CN.md` state where the command line keeps data and how the installed binary can be used from a terminal. `docs/agent.md` and the CLI help agree with them. Evidence: `docs-links`, `install-guide`, and the owner's review.

Constraints:

- No change to scoring, consent, the database schema, the bound window API or the host protocols.
- Nothing is copied or deleted in the learner's data directory except by an explicit command the learner runs.
- The window's own data location and behaviour do not change.

Stop and report at once if, at the start, something only the owner can supply is missing. Otherwise take reversible defaults and disclose them in `docs/decisions/OPEN-QUESTIONS.md` (Q-0020-NN): the exact notice wording and when it shows, how concurrent access is made safe, the exact Node 26 release, and how the package detects that it is running from an install. Stop only for an irreversible or out-of-bounds decision. End with a report that pairs each item above with its evidence and lists the known limits.

## Out of scope

- Moving or migrating existing data automatically.
- Any change to where the window keeps its data.
- Bumping the `@earendil-works` packages (goal 0021).
