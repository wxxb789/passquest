/**
 * Renders the tap's cask from a published GitHub release and, with `--push`,
 * commits it to `wxxb789/homebrew-tap` over the deploy key in
 * `HOMEBREW_TAP_DEPLOY_KEY` (goal 0017 P4, done-when 4).
 *
 * The digest is never typed by a human and never taken on trust: the tool asks
 * `gh` for the release's `.dmg` and `SHA256SUMS.txt`, computes the SHA-256 of
 * the downloaded asset and requires the release's own line to state the same
 * value before anything is rendered. Without `--push` the tool stops after
 * writing and hashing the render; that dry run is what the `cask` workflow runs
 * on a pull request that touches the template, `harness/lib/cask.ts`, this file
 * or `mygo.json`.
 *
 * The deploy key travels from the environment to a `0600` file inside the
 * run's `0700` temporary directory (`umask 077` while it is written), is handed
 * to git and ssh through `GIT_SSH_COMMAND` — never on a command line, where
 * `ps` would show it — and the file is deleted when the run ends, however it
 * ends. Every gh, git and ssh child runs with an environment this tool builds
 * from scratch, so the key never reaches one. Output names the ssh remote and
 * the commit id, never the key or its bytes.
 *
 * The push runs git with the system and global configuration replaced by an
 * empty file: a global `url.<base>.insteadOf git@github.com:` rewrite (common on
 * a Windows checkout that authenticates over https) or a global credential
 * helper would otherwise carry the push with a different credential than the
 * deploy key, and the tap's only credential is meant to be that key. ssh runs
 * with `-F /dev/null`, so `~/.ssh/config` — Host, ProxyCommand, IdentityFile —
 * cannot add an identity or move the connection either. The one identity the
 * commit needs is the tap owner's (goal 0017, the coordinator's rule): it lives
 * in the run's own `-c user.name/user.email` and in the child's
 * `GIT_AUTHOR_*`/`GIT_COMMITTER_*`, so neither the machine's global identity
 * nor an inherited variable can rename the commit.
 */
import { createHash } from "node:crypto";
import { createReadStream, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { CASK_FACTS, CASK_TAP_PATH, CASK_VERSION, renderCask, TAP_REPOSITORY } from "../lib/cask.ts";
import { sha256Hex } from "../lib/hash.ts";
import { loggedRun, resultText, runMain, tail } from "../lib/process.ts";

/** This checkout, so the tool runs from any working directory. */
const ROOT = path.resolve(import.meta.dir, "../..");
const TEMPLATE = ".github/homebrew-cask-template.rb";
/** The pinned v0.15.0 render; a dry run of that version reports its byte comparison. */
const GOLDEN = "harness/gates/testdata/pass-quest-0.15.0.rb";
/** Where the cask goes: `Casks/pass-quest.rb` inside the one tap repository. */
const TAP_REMOTE = `git@github.com:${TAP_REPOSITORY}.git`;
/** The commit the tap receives: the owner's identity, set per command; no global identity is read. */
const COMMIT_NAME = "wxxb789";
const COMMIT_EMAIL = "wxxb789@outlook.com";

const USAGE = `Render a release's cask and, with --push, commit it to ${TAP_REPOSITORY}.

  mise exec -- bun harness/tools/cask-publish.ts --tag <vX.Y.Z> [--push]

  --tag <vX.Y.Z>  the published release; its own .dmg and SHA256SUMS.txt are read
  --push          push the rendered cask to the tap; without it nothing is pushed
  --help          this text

Dry run (reads the release only; prints the release's digest and the render's):

  mise exec -- bun harness/tools/cask-publish.ts --tag v0.15.0

The coordinator's one-time tap seed, run before the local key copy is deleted:

  HOMEBREW_TAP_DEPLOY_KEY="$(cat "$HOME/.ssh/passquest-tap-deploy")" \\
    mise exec -- bun harness/tools/cask-publish.ts --tag v0.15.0 --push

The key is written from HOMEBREW_TAP_DEPLOY_KEY to a 0600 file and deleted when
the run ends; it is never printed, never passed as a command-line argument and
never placed in a child's environment, which this tool builds from scratch.
The push runs git with GIT_CONFIG_NOSYSTEM=1 and an empty GIT_CONFIG_GLOBAL, so
a global url.*.insteadOf rewrite cannot route it through another credential,
and ssh with -F /dev/null, so ~/.ssh/config cannot add an identity. The commit
is made as ${COMMIT_NAME} <${COMMIT_EMAIL}> with an explicit -c user.name /
-c user.email and the matching GIT_AUTHOR_*/GIT_COMMITTER_* variables, so the
tap records the owner and neither a global identity nor an inherited variable
can rename it.`;

interface Options {
  tag: string;
  push: boolean;
}

function parseArgs(argv: readonly string[]): { help: true } | { error: string } | { options: Options } {
  let tag = "";
  let push = false;
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index] ?? "";
    if (arg === "--help" || arg === "-h") return { help: true };
    if (arg === "--push") {
      push = true;
      continue;
    }
    if (arg === "--tag") {
      const value = argv[++index];
      if (value === undefined || value === "") return { error: "--tag needs the release tag, e.g. --tag v0.15.0" };
      tag = value;
      continue;
    }
    if (arg.startsWith("--tag=")) {
      tag = arg.slice("--tag=".length);
      continue;
    }
    return { error: `unknown argument '${arg}'` };
  }
  if (tag === "") return { error: "--tag <vX.Y.Z> is required" };
  if (!/^v[0-9]+\.[0-9]+\.[0-9]+$/.test(tag)) return { error: `--tag ${tag} must look like v0.15.0` };
  return { options: { tag, push } };
}

/** The digest a `SHA256SUMS.txt` states for `name`, or null when it states none. */
function sha256Line(text: string, name: string): string | null {
  for (const line of text.replace(/\r\n/g, "\n").split("\n")) {
    const match = /^([0-9a-f]{64})\s+\*?(.+?)\s*$/.exec(line);
    if (match !== null && match[2] === name) return match[1]!;
  }
  return null;
}

/** Streams the file into one SHA-256, so a 45 MB asset is never held in memory twice. */
async function sha256File(file: string): Promise<string> {
  const hash = createHash("sha256");
  for await (const chunk of createReadStream(file)) hash.update(chunk as Buffer);
  return hash.digest("hex");
}

/**
 * The key file the push uses: the environment value normalized to LF and one
 * trailing newline (OpenSSH wants the footer line terminated), written 0600
 * under `umask 077` so no other read is possible between create and mode. On
 * Windows the mode is not enforced (NTFS ACLs decide, and the file lives under
 * the run's temporary directory), so the key's protection there is the private
 * `0700` tree and its deletion — the file exists only for the push.
 */
function writeDeployKey(dir: string, value: string): string {
  const file = path.join(dir, "homebrew-tap-deploy-key");
  const text = value.replace(/\r\n/g, "\n");
  const previous = process.umask(0o077);
  try {
    writeFileSync(file, text.endsWith("\n") ? text : `${text}\n`, { mode: 0o600 });
  } finally {
    process.umask(previous);
  }
  return file;
}

/**
 * The ssh command git runs: this run's key only, host key accepted on first
 * use, no prompt, and `-F /dev/null` so `~/.ssh/config` cannot name another
 * identity file, ProxyCommand or Host block. `/dev/null` reads as the null
 * device both on the POSIX runners and in Git for Windows' POSIX shell (the
 * one Windows case this runs in, the coordinator's local seed).
 */
function sshCommand(keyFile: string): string {
  const key = keyFile.replace(/\\/g, "/");
  return [
    "ssh",
    "-F",
    "/dev/null",
    "-i",
    // One POSIX shell word; GIT_SSH_COMMAND is executed by a shell, so quoting is the contract.
    `'${key.replace(/'/g, "'\\''")}'`,
    "-o",
    "IdentitiesOnly=yes",
    "-o",
    "StrictHostKeyChecking=accept-new",
    "-o",
    "BatchMode=yes",
  ].join(" ");
}

/**
 * The environment a child sees: the OS-level minimum this tool carries itself
 * plus the names the caller lists. `replaceEnv` then hands exactly this map, so
 * the deploy key in this process's environment cannot reach gh, git or ssh, and
 * no inherited `GIT_*` variable can rename the commit. `gh` is given the
 * variables it resolves its config directory through, and its token when the
 * workflow's step exports one.
 */
function childEnv(names: readonly string[] = []): Record<string, string> {
  const env: Record<string, string> = {};
  for (const name of ["PATH", "HOME", "SystemRoot", "TEMP", "TMP", "TMPDIR", ...names]) {
    const value = process.env[name];
    if (value !== undefined && value !== "") env[name] = value;
  }
  return env;
}

async function main(argv: readonly string[]): Promise<number> {
  const parsed = parseArgs(argv);
  if ("help" in parsed) {
    process.stdout.write(`${USAGE}\n`);
    return 0;
  }
  if ("error" in parsed) {
    process.stderr.write(`cask-publish: ${parsed.error}\n\n${USAGE}\n`);
    return 2;
  }
  const { tag, push } = parsed.options;
  const version = tag.slice(1);
  const dmg = `${CASK_FACTS.token}-${version}-arm64.dmg`;

  const temp = mkdtempSync(path.join(tmpdir(), "passquest-cask-"));
  let keyFile: string | null = null;
  try {
    console.log(`cask-publish: ${CASK_FACTS.repository} release ${tag} -> ${TAP_REPOSITORY} (${push ? "push" : "dry run"})`);
    const ghEnv = childEnv(["XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA", "GH_CONFIG_DIR", "GH_TOKEN", "GITHUB_TOKEN"]);

    // The release's own bytes: download the asset and its checksum file, then
    // require the two to agree before anything is rendered.
    const downloadDir = path.join(temp, "release");
    mkdirSync(downloadDir, { recursive: true });
    const download = await loggedRun(
      "gh",
      [
        "release",
        "download",
        tag,
        "--repo",
        CASK_FACTS.repository,
        "--pattern",
        dmg,
        "--pattern",
        "SHA256SUMS.txt",
        "--dir",
        downloadDir,
      ],
      { env: ghEnv, replaceEnv: true },
    );
    if (download.code !== 0) {
      return fail(`gh release download ${tag} exited ${download.code}: ${tail(resultText(download), 6)}`);
    }
    const dmgPath = path.join(downloadDir, dmg);
    const sumsPath = path.join(downloadDir, "SHA256SUMS.txt");
    if (!existsSync(dmgPath)) return fail(`${tag} carries no ${dmg}`);
    if (!existsSync(sumsPath)) return fail(`${tag} carries no SHA256SUMS.txt`);
    const computed = await sha256File(dmgPath);
    const stated = sha256Line(readFileSync(sumsPath, "utf8"), dmg);
    if (stated === null) return fail(`${tag}'s SHA256SUMS.txt states no digest for ${dmg}`);
    if (stated !== computed) {
      return fail(`sha256 mismatch for ${dmg}: ${tag}'s SHA256SUMS.txt states ${stated}, the downloaded file hashes to ${computed}`);
    }
    console.log(`[release] ${dmg} (${statSync(dmgPath).size} bytes) sha256 ${computed}`);
    console.log(`[release] ${tag}'s SHA256SUMS.txt states the same digest`);

    const template = readFileSync(path.join(ROOT, TEMPLATE), "utf8");
    const rendered = renderCask(template, version, computed);
    const renderedPath = path.join(temp, path.basename(CASK_TAP_PATH));
    writeFileSync(renderedPath, rendered);
    console.log(`[render] ${TEMPLATE} with ${tag} -> ${renderedPath}`);
    console.log(`[render] version ${version}, sha256 ${computed}, rendered sha256 ${sha256Hex(rendered)}`);
    if (version === CASK_VERSION && existsSync(path.join(ROOT, GOLDEN))) {
      const equal = rendered === readFileSync(path.join(ROOT, GOLDEN), "utf8");
      console.log(
        `[render] ${equal ? "equals" : "differs from"} the pinned render ${GOLDEN}${equal ? " byte for byte" : " (re-render the golden and review the diff)"}`,
      );
    }

    if (!push) {
      console.log("[push] skipped: dry run (pass --push to commit the render to the tap)");
      return 0;
    }

    // A draft or pre-release never reaches the tap, whatever triggered this run;
    // the workflow guards the release event, and the tool refuses the tag too.
    const view = await loggedRun(
      "gh",
      ["release", "view", tag, "--repo", CASK_FACTS.repository, "--json", "isDraft,isPrerelease", "--jq", ".isDraft, .isPrerelease"],
      { env: ghEnv, replaceEnv: true },
    );
    if (view.code !== 0) return fail(`gh release view ${tag} exited ${view.code}: ${tail(resultText(view), 6)}`);
    const flags = view.stdout.replace(/\r\n/g, "\n").trim().split("\n");
    if (flags[0] === "true" || flags[1] === "true") {
      return fail(`${tag} is a draft or pre-release (draft=${flags[0] ?? "?"}, prerelease=${flags[1] ?? "?"}); a draft release changes nothing`);
    }

    const key = process.env.HOMEBREW_TAP_DEPLOY_KEY ?? "";
    if (key.trim() === "") {
      return fail("HOMEBREW_TAP_DEPLOY_KEY is not set; the tap push needs the deploy key (see --help)");
    }
    keyFile = writeDeployKey(temp, key);
    console.log(`[push] remote ${TAP_REMOTE}; key from HOMEBREW_TAP_DEPLOY_KEY in a 0600 temp file (not printed, deleted when the run ends)`);

    // The deploy key is the only credential this push may use: an empty global
    // config file drops any `url.*.insteadOf` rewrite or credential helper.
    const emptyConfig = path.join(temp, "gitconfig");
    writeFileSync(emptyConfig, "");
    // The owner's identity is set here as well as in the commit's `-c`
    // arguments, so an inherited GIT_* variable cannot rename the commit even
    // where `replaceEnv` is not the mechanism dropping it.
    const gitEnv: Record<string, string> = {
      ...childEnv(),
      GIT_SSH_COMMAND: sshCommand(keyFile),
      GIT_TERMINAL_PROMPT: "0",
      GIT_SSH_VARIANT: "ssh",
      GIT_CONFIG_NOSYSTEM: "1",
      GIT_CONFIG_GLOBAL: emptyConfig,
      GIT_AUTHOR_NAME: COMMIT_NAME,
      GIT_AUTHOR_EMAIL: COMMIT_EMAIL,
      GIT_COMMITTER_NAME: COMMIT_NAME,
      GIT_COMMITTER_EMAIL: COMMIT_EMAIL,
    };
    const clone = path.join(temp, "tap");
    const cloned = await loggedRun("git", ["clone", "--depth", "1", TAP_REMOTE, clone], { env: gitEnv, replaceEnv: true });
    if (cloned.code !== 0) return fail(`git clone ${TAP_REMOTE} exited ${cloned.code}: ${tail(resultText(cloned), 6)}`);

    const tapFile = path.join(clone, CASK_TAP_PATH);
    mkdirSync(path.dirname(tapFile), { recursive: true });
    const current = existsSync(tapFile) ? readFileSync(tapFile, "utf8") : null;
    // Line endings are normalized before the comparison: a checkout that turned
    // LF into CRLF is the same render, and `git add` would commit no change for it.
    if (current !== null && current.replaceAll("\r\n", "\n") === rendered.replaceAll("\r\n", "\n")) {
      console.log(`[push] ${TAP_REPOSITORY} already carries this render (${CASK_TAP_PATH}); nothing to commit`);
      return 0;
    }
    writeFileSync(tapFile, rendered);
    console.log(`[push] wrote ${CASK_TAP_PATH} (sha256 ${sha256Hex(rendered)})`);

    for (const args of [
      ["add", "--", CASK_TAP_PATH],
      ["-c", `user.name=${COMMIT_NAME}`, "-c", `user.email=${COMMIT_EMAIL}`, "commit", "-m", `Update Pass Quest cask from ${tag}`],
    ]) {
      const git = await loggedRun("git", args, { env: gitEnv, cwd: clone, replaceEnv: true });
      if (git.code !== 0) return fail(`git ${args.join(" ")} exited ${git.code}: ${tail(resultText(git), 6)}`);
    }
    const head = await loggedRun("git", ["rev-parse", "HEAD"], { env: gitEnv, cwd: clone, replaceEnv: true });
    if (head.code !== 0) return fail(`git rev-parse HEAD exited ${head.code}: ${tail(resultText(head), 6)}`);
    const pushed = await loggedRun("git", ["push", "origin", "HEAD:main"], { env: gitEnv, cwd: clone, replaceEnv: true });
    if (pushed.code !== 0) return fail(`git push origin HEAD:main exited ${pushed.code}: ${tail(resultText(pushed), 6)}`);
    console.log(`[push] committed ${head.stdout.trim()} and pushed HEAD:main to ${TAP_REPOSITORY}`);
    return 0;
  } finally {
    if (keyFile !== null) {
      try {
        rmSync(keyFile, { force: true });
      } catch {
        // The tree removal below takes the directory; a locked key file is not a leak.
      }
    }
    try {
      rmSync(temp, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
    } catch (error) {
      console.warn(`cask-publish: could not remove ${temp}: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
}

function fail(text: string): number {
  console.error(`cask-publish: ${text}`);
  return 1;
}

await runMain("cask-publish", main);
