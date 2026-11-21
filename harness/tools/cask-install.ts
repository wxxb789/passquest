/**
 * The macOS half of the `cask` workflow (goal 0017 P4, done-when 3): installs
 * the tap's cask on this machine and asserts, one command at a time, the
 * behaviour the goal names. Every assertion prints the exact command and the
 * value it observed — `xattr -p com.apple.quarantine <path> -> exit 1: No such
 * xattr: ...` — so the evidence record can quote this log; an assertion that
 * cannot run fails the run instead of passing it, and a failure quotes the last
 * 60 lines of the command's output, because that is where `brew` prints the
 * offence. Two facts are read from the run rather than assumed: the app's path
 * (the install's own `Moving App 'X.app' to '<path>'` line — the Caskroom path
 * `brew list --cask` reports is a symlink to it) and the list of paths `--zap`
 * must remove (the cask file Homebrew installed from).
 *
 * Modes:
 *   (default)      clone `wxxb789/homebrew-tap` into Homebrew's taps directory,
 *                  require its file to be byte-identical to a fresh render of
 *                  the template, then `brew install --cask wxxb789/tap/pass-quest`
 *                  and run every assertion, including every path in the
 *                  installed cask's zap trash: being gone after the uninstall;
 *                  `brew audit --cask --strict` runs last, after the
 *                  behavioural evidence is already in the log.
 *   --mutant <name> derive the tap cask with one feature removed (`quarantine`:
 *                  the postflight_steps block, `zap`: the zap trash: stanza
 *                  `quit`: the uninstall quit: argument), write the result as
 *                  `Casks/pass-quest.rb` in a scratch tap created with
 *                  `brew tap-new --no-git passquest/mutant`, install it fully
 *                  qualified and require the matching assertion to catch the
 *                  removal — the run that makes the assertions above
 *                  non-vacuous. A cask cannot be installed from a path: current
 *                  Homebrew refuses to load one ("Loading casks from paths,
 *                  URLs or Ruby source is not supported", `cask/cask_loader.rb`),
 *                  so the scratch tap is how a derived cask gets a name. It is
 *                  removed again when the run ends.
 *
 * The tap drift check is the reason the tap clone happens at all: the install
 * job compares the file Homebrew will read with `renderCask` of the tree's own
 * template, so a template edit that never reached the tap fails here instead of
 * being silently installed.
 *
 * This tool asserts an install on this machine; it runs only on macOS (the
 * `install` job) and exits 2 anywhere else, because a Bash "skip" on another OS
 * would report a green run for evidence that never happened.
 */
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import path from "node:path";

import {
  CASK_FACTS,
  CASK_TAP_PATH,
  MACOS_DATA_DIRECTORY,
  renderCask,
  sha256Text,
  statedRelease,
  TAP_REPOSITORY,
  zapTrashPaths,
} from "../lib/cask.ts";
import { startDesktop } from "../lib/desktop.ts";
import type { DesktopRun, LaunchResult } from "../lib/desktop.ts";
import { cleanEnv, launchPage, pageScript, sleep } from "../lib/desktop-page.ts";
import { MIN_TODAY_TEXT } from "../gates/desktop-smoke/boot.ts";
import { field, numberField } from "../lib/json.ts";
import { loggedRun, runMain } from "../lib/process.ts";
import type { RunResult } from "../lib/process.ts";

/** This checkout, so the tool runs from any working directory. */
const ROOT = path.resolve(import.meta.dir, "../..");
const TEMPLATE = ".github/homebrew-cask-template.rb";
/** `wxxb789/homebrew-tap` -> the tap `wxxb789/tap` -> the cask `wxxb789/tap/pass-quest`. */
const [TAP_OWNER = "", TAP_REPO = ""] = TAP_REPOSITORY.split("/");
const TAP_NAME = `${TAP_OWNER}/${TAP_REPO.replace(/^homebrew-/, "")}`;
const TAP_CASK = `${TAP_NAME}/${CASK_FACTS.token}`;
/**
 * The scratch tap the mutant runs install from: created on this machine with
 * `brew tap-new --no-git` (Manpage, "tap-new") and never pushed anywhere.
 * `--no-git` is enough for an install: a directory under `Taps/` is an
 * installed tap (`Tap#installed?`), and the receipt's `tap_git_head` reads nil
 * from it (`cask/cask.rb` `tap_git_head` -> `Tap#git_head` ->
 * `GitRepository#head_ref`, which returns nil, not an error, for a directory
 * without `.git`).
 */
const MUTANT_TAP_NAME = "passquest/mutant";
const MUTANT_TAP_CASK = `${MUTANT_TAP_NAME}/${CASK_FACTS.token}`;
/** The data directory the guides name and the cask's `zap trash:` carries, `~` resolved. */
const DATA_DIRECTORY = path.join(homedir(), MACOS_DATA_DIRECTORY.replace(/^~\//, ""));
const MARKER = "cask-install-marker.txt";
/** How long the uninstall gets to quit the app, and how long a mutant's app must survive. */
const QUIT_WINDOW_MS = 30_000;
const MUTANT_SURVIVAL_MS = 15_000;

type Mutant = "quarantine" | "zap" | "quit";
const MUTANTS: readonly Mutant[] = ["quarantine", "zap", "quit"];
const MUTANT_FEATURE: Record<Mutant, string> = {
  quarantine: "the postflight_steps block",
  zap: "the zap trash: stanza",
  quit: "the uninstall quit: argument",
};

const USAGE = `Assert the behaviour of the ${TAP_CASK} cask on this macOS machine.

  mise exec -- bun harness/tools/cask-install.ts [--mutant <name>]

  (default)         clone ${TAP_REPOSITORY} into Homebrew's taps directory, require the
                    tap file to equal a fresh render of ${TEMPLATE}, install the
                    cask fully qualified, run every assertion, audit it last
  --mutant <name>   write the tap cask with one feature removed as the ${CASK_FACTS.token}
                    cask of the scratch tap ${MUTANT_TAP_NAME} (brew tap-new --no-git),
                    install it fully qualified and require the matching assertion to
                    catch the removal; one of ${MUTANTS.join(", ")}
                    (repeatable, or comma-separated)
  --help            this text

The cask's assertions, each printing its exact command and observed value:
  sw_vers against the floor the cask declares; brew --version; the tap file
  against the render; brew install; the app path the install's own
  "Moving App ... to '<path>'" line names; xattr -p com.apple.quarantine on
  that app and on a nested file; a launch through the smoke hook; the data
  directory marker; the uninstall quitting the running app; every path the
  installed cask file's zap trash: names gone after --zap; brew audit --cask
  --strict. A failing assertion prints the command's last 60 lines, so the
  offence at the end of an audit or an install is in the log.

Exit codes: 0 every assertion passed, 1 a failure (its line is in the log),
2 usage or the wrong platform. This tool never runs on Windows or Linux.`;

interface Options {
  mutants: Mutant[];
  help: boolean;
  error: string | null;
}

function parseArgs(argv: readonly string[]): Options {
  const options: Options = { mutants: [], help: false, error: null };
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index] ?? "";
    if (arg === "--help" || arg === "-h") return { ...options, help: true };
    if (arg === "--mutant") {
      const value = argv[++index];
      if (value === undefined || value === "") return { ...options, error: `--mutant needs one of ${MUTANTS.join(", ")}` };
      for (const name of value.split(",")) {
        if (!(MUTANTS as readonly string[]).includes(name)) {
          return { ...options, error: `--mutant ${name} is not one of ${MUTANTS.join(", ")}` };
        }
        options.mutants.push(name as Mutant);
      }
      continue;
    }
    return { ...options, error: `unknown argument '${arg}'` };
  }
  return options;
}

/** What the run prints and the exit code it ends with. */
class Checker {
  private readonly failures: string[] = [];
  private passed = 0;

  /** One assertion: its key, the exact command and observed value, and why it failed. */
  check(key: string, ok: boolean, observed: string, failure = ""): boolean {
    if (ok) {
      this.passed += 1;
      console.log(`[assert] ${key}: pass — ${observed}`);
      return true;
    }
    const reason = failure === "" ? observed : failure;
    this.failures.push(`${key}: ${reason}`);
    console.log(`[assert] ${key}: FAIL — ${reason}`);
    return false;
  }

  get ok(): boolean {
    return this.failures.length === 0;
  }

  summary(label: string): void {
    console.log(`[summary] ${label}: ${this.passed} assertion(s) passed, ${this.failures.length} failed`);
    for (const failure of this.failures) console.log(`[summary]   FAIL ${failure}`);
  }
}

/** The first lines of a command's output, for one assertion line. */
function observed(result: RunResult, limit = 4): string {
  const text = `${result.stdout}${result.stderr}`.replace(/\r\n/g, "\n").trim();
  if (text === "") return "(no output)";
  return text.split("\n").slice(0, limit).join(" | ");
}

/**
 * The last lines of a command's output, for a failure line. The head of a
 * `brew` command is its progress noise — "audit is a developer command" and the
 * gem fetch — while the offence is printed at the end, so a failure that kept
 * the head hid exactly what it existed to report (round 4's audit assertion
 * failed with its offence outside the logged 12 lines). 60 lines, and the count
 * of the lines left out, so a long output is still bounded.
 */
function observedTail(result: RunResult, limit = 60): string {
  const text = `${result.stdout}${result.stderr}`.replace(/\r\n/g, "\n").trim();
  if (text === "") return "(no output)";
  const lines = text.split("\n");
  const omitted = lines.length - limit;
  const shown = lines.slice(Math.max(0, omitted)).join(" | ");
  return omitted > 0 ? `(${omitted} earlier line(s) omitted) ${shown}` : shown;
}

/** The first line on which two texts differ; the drift failure names it. */
function firstDifference(rendered: string, tap: string): string {
  const left = rendered.split("\n");
  const right = tap.split("\n");
  for (let index = 0; index < Math.max(left.length, right.length); index++) {
    if (left[index] !== right[index]) {
      return `first differing line ${index + 1}: the render has ${JSON.stringify(left[index] ?? "(end of file)")}, the tap file has ${JSON.stringify(right[index] ?? "(end of file)")}`;
    }
  }
  return "the two texts hold the same lines and differ in encoding only";
}

/**
 * One run's mutable state: the checker, the fully qualified name of the cask
 * this run installed, the app to reap, the clone to remove.
 *
 * The name is decided once, from the mode, and every `brew` command that takes
 * a cask name — `install`, `list`, `uninstall --zap`, `audit` — is given it. A
 * bare token is not enough: in a mutant run two installed taps carry `pass-quest`
 * (`passquest/mutant` and the tap clone), and `brew uninstall --zap --cask
 * pass-quest` then refuses to act — "Cask pass-quest exists in multiple taps:
 * passquest/mutant/pass-quest, wxxb789/tap/pass-quest" (CI run 44909338098) —
 * so the uninstall removed nothing and the mutant's own cask stayed installed.
 */
class CaskRun {
  readonly checker = new Checker();
  app: DesktopRun | null = null;
  tapDir: string | null = null;
  mutantTapDir: string | null = null;
  markerCreated = false;

  constructor(readonly temp: string, readonly cask: string) {}
}

/** The data directory the app creates, `~/Library/Application Support/Pass Quest`. */
function markerPath(): string {
  return path.join(DATA_DIRECTORY, MARKER);
}

/** What the install output says about the app it moved: the path, or why none could be read. */
interface MovedApp {
  app: string | null;
  matched: string | null;
  reason: string | null;
}

/**
 * The app Homebrew moved into place, read from the install's own output. `brew`
 * prints `Moving App 'Pass Quest.app' to '/Applications/Pass Quest.app'`
 * (`cask/artifact/moved.rb` `move`); the target is the app that is installed,
 * while the Caskroom path `brew list --cask` reports is a symlink Homebrew
 * leaves behind (`post_move` -> `FileUtils.ln_sf target, source`). Asserting
 * against the Caskroom path therefore asserted against a link: round 4's
 * quarantine checks and boot launch used it, and so did the `--zap` uninstall,
 * which removes the target, not the link. Nothing is assumed about the
 * directory: the path comes from the install, and it must name the app the cask
 * declares.
 */
function movedApp(install: RunResult): MovedApp {
  const text = `${install.stdout}${install.stderr}`.replace(/\r\n/g, "\n");
  const moving = /Moving ([A-Za-z]+) '([^'\n]+)' to '([^'\n]+)'/.exec(text);
  if (moving === null) {
    return { app: null, matched: null, reason: `the install printed no "Moving ..." line` };
  }
  const matched = moving[0];
  const kind = moving[1] ?? "";
  const source = moving[2] ?? "";
  const target = moving[3] ?? "";
  const wanted = `${CASK_FACTS.name}.app`;
  if (source !== wanted) {
    return { app: null, matched, reason: `the install moved ${kind} ${JSON.stringify(source)}, not ${JSON.stringify(wanted)}` };
  }
  return { app: target, matched, reason: null };
}

/**
 * The path the installed app really has, asserted from the install output and
 * used by every later check. A missing line fails the run instead of falling
 * back to a guess: a wrong path would make the quarantine assertions pass
 * against a symlink and the boot launch run the wrong copy.
 */
function installedAppPath(run: CaskRun, install: RunResult): string {
  const moved = movedApp(install);
  run.checker.check(
    "app-path",
    moved.app !== null,
    `the install printed ${JSON.stringify(moved.matched ?? "")}`,
    `${moved.reason ?? "no app path"}; the app's location is taken from the install output, never assumed — its last lines: ${observedTail(install)}`,
  );
  if (moved.app === null) throw new Error("the install named no app path; refusing to assert against a guessed one");
  return moved.app;
}

/**
 * The Caskroom `.app` `brew list --cask` reports, printed with `ls -ld` once as
 * information and never asserted: after `post_move` it is a symlink to the moved
 * app, so the log shows why a check written against it would prove nothing about
 * the installed app.
 */
async function printCaskroomLink(run: CaskRun): Promise<void> {
  const listed = await loggedRun("brew", ["list", "--cask", run.cask]);
  if (listed.code !== 0) {
    console.log(`[info] brew list --cask ${run.cask} -> exit ${listed.code}: ${observed(listed)}`);
    return;
  }
  const app =
    listed.stdout
      .replace(/\r\n/g, "\n")
      .split("\n")
      .map((line) => line.trim())
      .find((line) => line.endsWith(".app")) ?? null;
  if (app === null) {
    console.log(`[info] brew list --cask ${run.cask} lists no Caskroom .app: ${observed(listed)}`);
    return;
  }
  const link = await loggedRun("ls", ["-ld", app]);
  console.log(`[info] ls -ld ${JSON.stringify(app)} -> exit ${link.code}: ${observed(link, 1)}`);
}

/** The one executable in the bundle's `Contents/MacOS`, the smoke hook's target. */
function appExecutable(app: string): string {
  const macos = path.join(app, "Contents", "MacOS");
  const files = existsSync(macos) ? readdirSync(macos).filter((name) => !name.startsWith(".")) : [];
  if (files.length !== 1) throw new Error(`${macos} holds ${files.length} entries; expected exactly one executable`);
  return path.join(macos, files[0] ?? "");
}

/** The quarantine attribute one path carries, as `xattr -p com.apple.quarantine` reports it. */
async function xattrQuarantine(target: string): Promise<{ absent: boolean; present: boolean; detail: string; tail: string }> {
  const result = await loggedRun("xattr", ["-p", "com.apple.quarantine", target]);
  const text = `${result.stdout}${result.stderr}`.replace(/\r\n/g, "\n").trim();
  const command = `xattr -p com.apple.quarantine ${JSON.stringify(target)}`;
  return {
    absent: result.code !== 0 && text.includes("No such xattr"),
    present: result.code === 0 && text !== "",
    detail: `${command} -> exit ${result.code}: ${text.split("\n")[0] ?? "(no output)"}`,
    tail: `${command} -> exit ${result.code}: ${observedTail(result)}`,
  };
}

/** The app this run made, under `$HOME/Library`, matched by the names it writes. */
async function libraryListing(): Promise<string[]> {
  const result = await loggedRun("find", [
    path.join(homedir(), "Library"),
    "-maxdepth",
    "2",
    "(",
    "-iname",
    "*passquest*",
    "-o",
    "-iname",
    "*pass quest*",
    ")",
    "-print",
  ]);
  return result.stdout
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "")
    .sort();
}

function printListing(label: string, entries: readonly string[]): void {
  console.log(`[listing] ${label} — $HOME/Library, maxdepth 2, names matching *passquest* or *pass quest*: ${entries.length} entr(y|ies)`);
  for (const entry of entries) console.log(`[listing]   ${entry.replace(homedir(), "~")}`);
}

/** Polls the app's own pid; the blocking launch needs the app running before the uninstall. */
async function waitForAppPid(app: DesktopRun, timeoutMs: number): Promise<number | null> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const pid = await app.appPid();
    if (typeof pid === "number") return pid;
    await sleep(250);
  }
  return null;
}

/** The app's exit within the window, or null when it is still running. */
async function waitForExit(app: DesktopRun, timeoutMs: number): Promise<LaunchResult | null> {
  return Promise.race([app.result, sleep(timeoutMs).then(() => null)]);
}

/** Runs the installed app through the smoke hook against a temporary data directory. */
async function bootCheck(run: CaskRun, exe: string): Promise<boolean> {
  const problems: string[] = [];
  const script = `
const stage = () => document.querySelector("#stage");
const ready = () => window.__pq === undefined || window.__pq.ready === true;
const content = () => (stage()?.children.length ?? 0) > 0 && document.body.innerText.length > ${MIN_TODAY_TEXT};
const deadline = Date.now() + 15000;
while (!(ready() && content()) && Date.now() < deadline) await new Promise((done) => setTimeout(done, 100));
const errors = document.querySelectorAll("#errbar, .error-bar");
const node = stage();
return {
  textLength: document.body.innerText.length,
  stageChildren: node ? node.children.length : -1,
  errorBars: errors.length,
  ready: window.__pq === undefined ? null : window.__pq.ready === true,
};
`;
  const launched = await launchPage({
    binary: exe,
    cwd: run.temp,
    dir: run.temp,
    label: "boot",
    header: "cask-install: the installed app through the smoke hook",
    where: "boot",
    script: pageScript(script),
    env: { PASSQUEST_DESKTOP_DATA: path.join(run.temp, "data") },
    check: (condition, message) => {
      if (!condition) problems.push(message);
      return condition;
    },
  });
  if (launched === null) {
    return run.checker.check("boot", false, "", `the installed app did not boot: ${problems.join("; ") || "the launch produced no result"}`);
  }
  const value = launched.value;
  const textLength = numberField(value, "textLength") ?? 0;
  const stageChildren = numberField(value, "stageChildren") ?? -1;
  const errorBars = numberField(value, "errorBars") ?? -1;
  const ready = field(value, "ready");
  const detail = `the smoke hook returned body text ${textLength} characters (> ${MIN_TODAY_TEXT}), ${stageChildren} stage children, ${errorBars} error bar(s), ready=${String(ready)}`;
  const ok = problems.length === 0 && textLength > MIN_TODAY_TEXT && stageChildren > 0 && errorBars === 0 && (ready === null || ready === true);
  return run.checker.check("boot", ok, detail, `${detail}; hook failures: ${problems.join("; ")}`);
}

/** Starts the installed app with a script that holds it open, so the uninstall has to quit it. */
async function startRunningApp(run: CaskRun, exe: string): Promise<{ app: DesktopRun; pid: number | null }> {
  const dir = path.join(run.temp, "hold");
  mkdirSync(dir, { recursive: true });
  const script = path.join(dir, "hold.js");
  writeFileSync(
    script,
    `// cask-install: hold the app open while the uninstall runs\nconst until = Date.now() + 9 * 60_000;\nwhile (Date.now() < until) await new Promise((done) => setTimeout(done, 500));\nreturn { held: true };\n`,
  );
  const app = startDesktop({
    binary: exe,
    cwd: run.temp,
    env: {
      ...cleanEnv(),
      PASSQUEST_DESKTOP_SCRIPT: script,
      PASSQUEST_DESKTOP_RESULT: path.join(dir, "hold.result.json"),
      PASSQUEST_DESKTOP_HIDDEN: "1",
      PASSQUEST_DESKTOP_DATA: path.join(run.temp, "data"),
    },
    timeoutMs: 10 * 60_000,
  });
  run.app = app;
  const pid = await waitForAppPid(app, 30_000);
  return { app, pid };
}

/** Creates the data directory the guides name, with a marker the zap must remove. */
async function createDataMarker(run: CaskRun): Promise<void> {
  if (existsSync(DATA_DIRECTORY)) {
    throw new Error(`${DATA_DIRECTORY} already exists; this tool refuses to remove data it did not create`);
  }
  mkdirSync(DATA_DIRECTORY, { recursive: true });
  writeFileSync(markerPath(), `${new Date().toISOString()} the data directory this cask's zap trash: removes\n`);
  run.markerCreated = true;
  console.log(`[state] created ${DATA_DIRECTORY} and wrote ${MARKER} inside it (the guide's data directory)`);
}

/** `brew uninstall --zap` and the three things it must do: quit, remove, remove. */
async function uninstallAndAssert(run: CaskRun, appPath: string, app: DesktopRun | null): Promise<void> {
  const uninstall = await loggedRun("brew", ["uninstall", "--zap", "--cask", run.cask]);
  run.checker.check(
    "uninstall",
    uninstall.code === 0,
    `brew uninstall --zap --cask ${run.cask} -> exit ${uninstall.code}: ${observed(uninstall, 6)}`,
    `brew uninstall --zap --cask ${run.cask} exited ${uninstall.code}: ${observedTail(uninstall)}`,
  );
  if (app !== null) {
    const log = `${uninstall.stdout}${uninstall.stderr}`;
    const quitLine = log
      .replace(/\r\n/g, "\n")
      .split("\n")
      .map((line) => line.trim())
      .find((line) => line.includes("quit successfully"));
    run.checker.check(
      "quit-log",
      quitLine !== undefined,
      `${quitLine ?? "no quit line"} — from the uninstall output`,
      `the uninstall log carries no "Application '${CASK_FACTS.identifier}' quit successfully." line: ${observedTail(uninstall)}`,
    );
    const exit = await waitForExit(app, QUIT_WINDOW_MS);
    run.checker.check(
      "quit-exit",
      exit !== null && !exit.timedOut,
      `the app process exited within ${QUIT_WINDOW_MS / 1000}s of the uninstall (the watchdog did not kill it)`,
      `the app process was still running ${QUIT_WINDOW_MS / 1000}s after the uninstall`,
    );
  }
  const bundle = await loggedRun("ls", ["-d", appPath]);
  run.checker.check(
    "bundle-gone",
    bundle.code !== 0 && !existsSync(appPath),
    `ls -d ${JSON.stringify(appPath)} -> exit ${bundle.code}: ${observed(bundle)}`,
    `the bundle survived the uninstall: ${observedTail(bundle)}`,
  );
  const data = await loggedRun("ls", ["-d", DATA_DIRECTORY]);
  run.checker.check(
    "data-gone",
    data.code !== 0 && !existsSync(DATA_DIRECTORY),
    `ls -d ${JSON.stringify(DATA_DIRECTORY)} -> exit ${data.code}: ${observed(data)} (the marker was inside)`,
    `the data directory survived the uninstall: ${observedTail(data)}`,
  );
}

/**
 * After `--zap`, every path the installed cask file's `zap trash:` names must
 * be gone. The list is read out of that file — the tap's cask, which Homebrew
 * installed from — never a second list in this tool: a hard-coded list would
 * keep passing after the stanza changed, and would have to be kept in step with
 * a file the runner, not this checkout, owns.
 */
async function assertZapPathsGone(run: CaskRun, caskText: string): Promise<void> {
  const entries = zapTrashPaths(caskText);
  if (entries.length === 0) {
    throw new Error("the installed cask file states no zap trash: paths, so the zap assertion would prove nothing");
  }
  console.log(`[state] the installed cask file's zap trash: names ${entries.length} path(s): ${entries.join(", ")}`);
  for (const entry of entries) {
    const target = entry.startsWith("~/") ? path.join(homedir(), entry.slice(2)) : entry;
    const listed = await loggedRun("ls", ["-d", target]);
    const gone = listed.code !== 0 && !existsSync(target);
    const marker = target === DATA_DIRECTORY ? ` (${MARKER} was inside)` : "";
    run.checker.check(
      `zap-gone ${entry}`,
      gone,
      `ls -d ${JSON.stringify(target)} -> exit ${listed.code}: ${observed(listed)}${marker}`,
      `the path the installed cask's zap trash: names survived the uninstall: ${observedTail(listed)}`,
    );
  }
}

/** Clones the tap, requires its file to equal a fresh render, and returns both texts. */
async function cloneTap(run: CaskRun): Promise<string> {
  const brewRepo = await loggedRun("brew", ["--repository"]);
  if (brewRepo.code !== 0) throw new Error(`brew --repository exited ${brewRepo.code}: ${observed(brewRepo)}`);
  const tapDir = path.join(brewRepo.stdout.trim(), "Library", "Taps", ...TAP_REPOSITORY.split("/"));
  if (existsSync(tapDir)) {
    console.log(`[tap] removing the existing clone ${tapDir}`);
    rmSync(tapDir, { recursive: true, force: true });
  }
  mkdirSync(path.dirname(tapDir), { recursive: true });
  const url = `https://github.com/${TAP_REPOSITORY}.git`;
  const cloned = await loggedRun("git", ["clone", "--depth", "1", url, tapDir]);
  run.checker.check("tap-clone", cloned.code === 0, `git clone --depth 1 ${url} -> exit 0`, `git clone exited ${cloned.code}: ${observedTail(cloned)}`);
  if (cloned.code !== 0) throw new Error(`git clone ${url} exited ${cloned.code}`);
  run.tapDir = tapDir;

  const file = path.join(tapDir, CASK_TAP_PATH);
  const text = readFileSync(file, "utf8");
  const stated = statedRelease(text);
  if (stated === null) throw new Error(`${file} states no version and sha256 pair; it is not a rendered cask`);
  const template = readFileSync(path.join(ROOT, TEMPLATE), "utf8");
  const rendered = renderCask(template, stated.version, stated.sha256);
  const equal = text.replaceAll("\r\n", "\n") === rendered.replaceAll("\r\n", "\n");
  console.log(`[tap] ${CASK_TAP_PATH}: version ${stated.version}, sha256 ${stated.sha256}, sha256(file) ${sha256Text(text)}`);
  console.log(`[tap] render of ${TEMPLATE} with those values: sha256 ${sha256Text(rendered)}`);
  run.checker.check(
    "tap-render",
    equal,
    `the tap file is byte-identical to the render (version ${stated.version}, sha256 ${sha256Text(text)})`,
    `the tap drifted from the template; ${firstDifference(rendered, text)}`,
  );
  if (!equal) throw new Error("the tap cask drifted from this tree's template; refusing to assert against it");
  return text;
}

/** Where `Tap#path` puts the scratch tap: `Taps/<user>/homebrew-<repo>` below the Homebrew tree. */
function scratchTapDir(brewRepository: string): string {
  const [user = "", repo = ""] = MUTANT_TAP_NAME.split("/");
  return path.join(brewRepository, "Library", "Taps", user, `homebrew-${repo}`);
}

/** Creates the scratch tap the mutant runs install from, and records it for cleanup. */
async function createMutantTap(run: CaskRun): Promise<void> {
  const brewRepo = await loggedRun("brew", ["--repository"]);
  if (brewRepo.code !== 0) throw new Error(`brew --repository exited ${brewRepo.code}: ${observed(brewRepo)}`);
  const dir = scratchTapDir(brewRepo.stdout.trim());
  if (existsSync(dir)) {
    console.log(`[tap] removing the scratch tap an earlier run left behind: ${dir}`);
    rmSync(dir, { recursive: true, force: true });
  }
  // Recorded before the command, so cleanup takes a half-created tap with it.
  run.mutantTapDir = dir;
  const created = await loggedRun("brew", ["tap-new", "--no-git", MUTANT_TAP_NAME]);
  run.checker.check(
    "mutant-tap",
    created.code === 0,
    `brew tap-new --no-git ${MUTANT_TAP_NAME} -> exit 0: ${observed(created, 1)}`,
    `brew tap-new --no-git ${MUTANT_TAP_NAME} exited ${created.code}: ${observedTail(created)}`,
  );
  if (created.code !== 0) throw new Error(`brew tap-new --no-git ${MUTANT_TAP_NAME} exited ${created.code}`);
}

/** The cask text with one feature removed; throws when the feature is not there to remove. */
function mutate(cask: string, mutant: Mutant): string {
  const lines = cask.split("\n");
  if (mutant === "quarantine") {
    const start = lines.findIndex((line) => /^\s*postflight_steps\s+do\s*$/.test(line));
    if (start < 0) throw new Error("the cask has no postflight_steps block to remove");
    // The block's own `end` sits at the opening line's indentation; a nested
    // block's `end` (there is none today) would be deeper and is skipped.
    const indent = /^(\s*)/.exec(lines[start] ?? "")?.[1] ?? "";
    const end = lines.findIndex((line, index) => index > start && new RegExp(`^${indent}end\\s*$`).test(line));
    if (end < 0) throw new Error("the postflight_steps block has no closing end line");
    return [...lines.slice(0, start), ...lines.slice(end + 1)].join("\n");
  }
  if (mutant === "zap") {
    const start = lines.findIndex((line) => /^\s*zap\s+trash:/.test(line));
    if (start < 0) throw new Error("the cask has no zap trash: stanza to remove");
    const head = (lines[start] ?? "").replace(/^\s*zap\s+trash:\s*/, "");
    let end = start;
    if (head.startsWith("[") && !head.includes("]")) {
      end = lines.findIndex((line, index) => index > start && line.includes("]"));
      if (end < 0) throw new Error("the zap trash: array has no closing bracket line");
    }
    return [...lines.slice(0, start), ...lines.slice(end + 1)].join("\n");
  }
  const quit = lines.findIndex((line) => /^\s*uninstall\s+quit:\s*"[^"]*"\s*$/.test(line));
  if (quit < 0) throw new Error("the cask has no uninstall quit: argument to remove");
  return [...lines.slice(0, quit), ...lines.slice(quit + 1)].join("\n");
}

/**
 * The runner's macOS version against the floor the cask declares; the install
 * job pins `runs-on: macos-26`, and a cask whose `depends_on macos:` is newer
 * than the runner would fail the install with a message that reads like a
 * broken cask, so this names the real cause first.
 */
async function assertMacosFloor(run: CaskRun): Promise<void> {
  const version = await loggedRun("sw_vers", ["-productVersion"]);
  const text = version.stdout.replace(/\r\n/g, "\n").trim();
  const major = Number.parseInt(text.split("\n")[0] ?? "", 10);
  const ok = version.code === 0 && Number.isInteger(major) && major >= CASK_FACTS.minimumMajor;
  run.checker.check(
    "macos",
    ok,
    `sw_vers -productVersion -> exit ${version.code}: ${text === "" ? "(no output)" : text} (the cask declares macOS ${CASK_FACTS.minimumMajor})`,
    `the runner reports macOS ${text === "" ? "(nothing)" : text}, below the cask's macOS ${CASK_FACTS.minimumMajor}; this job needs runs-on: macos-${CASK_FACTS.minimumMajor} or newer — sw_vers -productVersion -> exit ${version.code}: ${observedTail(version)}`,
  );
  if (!ok) throw new Error(`this machine is not macOS ${CASK_FACTS.minimumMajor}+, so the cask cannot install here`);
}

/** Tap mode: the tap file, the full assertion set, then the audit. */
async function runTap(run: CaskRun): Promise<void> {
  const brew = await loggedRun("brew", ["--version"]);
  run.checker.check("brew", brew.code === 0, `brew --version -> exit 0: ${observed(brew, 1)}`, `brew --version exited ${brew.code}: ${observedTail(brew)}`);
  if (brew.code !== 0) throw new Error("Homebrew is not on PATH");

  const tapText = await cloneTap(run);

  const install = await loggedRun("brew", ["install", "--cask", run.cask]);
  run.checker.check(
    "install",
    install.code === 0,
    `brew install --cask ${run.cask} -> exit 0: ${observed(install)}`,
    `brew install --cask ${run.cask} exited ${install.code}: ${observedTail(install)}`,
  );
  if (install.code !== 0) throw new Error(`brew install --cask ${run.cask} exited ${install.code}`);

  const appPath = installedAppPath(run, install);
  const exe = appExecutable(appPath);
  console.log(`[state] installed app ${appPath} (the target of the install's own Moving line), executable ${exe}`);
  await printCaskroomLink(run);

  const attribute = await xattrQuarantine(appPath);
  run.checker.check("quarantine-app", attribute.absent, attribute.detail, `the installed app still carries com.apple.quarantine: ${attribute.tail}`);
  const nested = await xattrQuarantine(exe);
  run.checker.check("quarantine-nested", nested.absent, nested.detail, `a nested file still carries com.apple.quarantine: ${nested.tail}`);
  await bootCheck(run, exe);

  printListing("before the uninstall", await libraryListing());
  await createDataMarker(run);
  run.checker.check(
    "data-marker",
    existsSync(markerPath()),
    `${DATA_DIRECTORY} holds ${MARKER} before the uninstall`,
    `${markerPath()} is missing although this run created it`,
  );

  const { app, pid } = await startRunningApp(run, exe);
  const early = await Promise.race([app.result.then(() => "exited" as const), sleep(3000).then(() => "alive" as const)]);
  run.checker.check(
    "quit-precondition",
    pid !== null && early === "alive",
    `the app is running before the uninstall (pid ${pid}, state ${early})`,
    `the app was not running before the uninstall (pid ${pid}, state ${early}); the quit assertion cannot observe anything`,
  );
  if (pid === null || early !== "alive") throw new Error("the app did not stay running; the uninstall would not meet a running app");

  await uninstallAndAssert(run, appPath, app);
  await assertZapPathsGone(run, tapText);
  printListing("after the uninstall", await libraryListing());

  const audit = await loggedRun("brew", ["audit", "--cask", "--strict", run.cask]);
  run.checker.check(
    "audit",
    audit.code === 0,
    `brew audit --cask --strict ${run.cask} -> exit 0: ${observed(audit, 2)}`,
    `brew audit --cask --strict ${run.cask} exited ${audit.code}; its last 60 lines: ${observedTail(audit)}`,
  );
}

/**
 * One mutant: install the derived cask from the scratch tap and require the
 * matching assertion to catch the removal. Its caller runs only when mutants
 * were asked for, where `run.cask` is the scratch tap's cask.
 */
async function runMutant(run: CaskRun, mutant: Mutant, tapText: string): Promise<void> {
  console.log(`\ncask-install: mutant '${mutant}' — the tap cask with ${MUTANT_FEATURE[mutant]} removed, installed from the scratch tap ${MUTANT_TAP_NAME}`);
  const mutated = mutate(tapText, mutant);
  if (mutated === tapText) throw new Error(`the '${mutant}' mutation changed nothing`);
  if (run.mutantTapDir === null) throw new Error("the scratch tap was not created");
  // The file keeps the cask's own name: Homebrew derives the token from the
  // basename and raises when it disagrees with the header (cask_loader.rb, token_from_path).
  const file = path.join(run.mutantTapDir, CASK_TAP_PATH);
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, mutated);
  console.log(`[mutate] removed ${MUTANT_FEATURE[mutant]}; wrote ${file} (${mutated.length} bytes against the tap file's ${tapText.length}) as ${run.cask}`);

  const install = await loggedRun("brew", ["install", "--cask", run.cask]);
  run.checker.check(
    "install",
    install.code === 0,
    `brew install --cask ${run.cask} -> exit 0: ${observed(install)}`,
    `the '${mutant}' mutant did not install: ${observedTail(install)}`,
  );
  if (install.code !== 0) throw new Error(`the '${mutant}' mutant did not install`);

  // The same install-output path the tap run uses: the mutants assert on the
  // app Homebrew moved, not on the Caskroom symlink that pointed at it.
  const appPath = installedAppPath(run, install);

  if (mutant === "quarantine") {
    const attribute = await xattrQuarantine(appPath);
    run.checker.check(
      "mutant-quarantine",
      attribute.present,
      `${attribute.detail} — the attribute postflight_steps removes is still there`,
      `the attribute is absent although postflight_steps was removed (${attribute.tail}); the quarantine assertion is not specific`,
    );
    // The marker makes the shared helper's `data-gone` assertion mean something
    // here too: without it this mutant never created the directory, so `ls -d`
    // could only fail because the directory was never there.
    await createDataMarker(run);
    await uninstallAndAssert(run, appPath, null);
    return;
  }

  if (mutant === "zap") {
    await createDataMarker(run);
    run.checker.check("data-marker", existsSync(markerPath()), `${DATA_DIRECTORY} holds ${MARKER} before the uninstall`, `${markerPath()} is missing although this run created it`);
    const uninstall = await loggedRun("brew", ["uninstall", "--zap", "--cask", run.cask]);
    run.checker.check("uninstall", uninstall.code === 0, `brew uninstall --zap --cask ${run.cask} -> exit ${uninstall.code}: ${observed(uninstall, 4)}`, `the uninstall exited ${uninstall.code}: ${observedTail(uninstall)}`);
    const listed = await loggedRun("ls", ["-d", DATA_DIRECTORY]);
    run.checker.check(
      "mutant-zap",
      listed.code === 0 && existsSync(markerPath()),
      `ls -d ${JSON.stringify(DATA_DIRECTORY)} -> exit ${listed.code}: ${observed(listed)}; ${MARKER} is still inside`,
      `the data directory is gone although zap trash: was removed; the zap assertion is not specific`,
    );
    const bundle = await loggedRun("ls", ["-d", appPath]);
    run.checker.check("bundle-gone", bundle.code !== 0 && !existsSync(appPath), `ls -d ${JSON.stringify(appPath)} -> exit ${bundle.code}: ${observed(bundle)}`, `the bundle survived the uninstall: ${observedTail(bundle)}`);
    return;
  }

  const { app, pid } = await startRunningApp(run, appExecutable(appPath));
  const early = await Promise.race([app.result.then(() => "exited" as const), sleep(3000).then(() => "alive" as const)]);
  run.checker.check(
    "quit-precondition",
    pid !== null && early === "alive",
    `the app is running before the uninstall (pid ${pid}, state ${early})`,
    `the app was not running (pid ${pid}, state ${early}); the quit mutant cannot observe anything`,
  );
  if (pid === null || early !== "alive") throw new Error("the app did not stay running");
  const uninstall = await loggedRun("brew", ["uninstall", "--zap", "--cask", run.cask]);
  run.checker.check("uninstall", uninstall.code === 0, `brew uninstall --zap --cask ${run.cask} -> exit ${uninstall.code}: ${observed(uninstall, 4)}`, `the uninstall exited ${uninstall.code}: ${observedTail(uninstall)}`);
  const spent = await waitForExit(app, MUTANT_SURVIVAL_MS);
  run.checker.check(
    "mutant-quit",
    spent === null,
    `pid ${pid} is still running ${MUTANT_SURVIVAL_MS / 1000}s after the uninstall; the log carries no quit line`,
    `the app exited although uninstall quit: was removed; the quit assertion is not specific`,
  );
  const bundle = await loggedRun("ls", ["-d", appPath]);
  run.checker.check("bundle-gone", bundle.code !== 0 && !existsSync(appPath), `ls -d ${JSON.stringify(appPath)} -> exit ${bundle.code}: ${observed(bundle)}`, `the bundle survived the uninstall: ${observedTail(bundle)}`);
}

async function cleanup(run: CaskRun): Promise<void> {
  if (run.app !== null) {
    const state = await Promise.race([run.app.result.then(() => "exited" as const), sleep(1500).then(() => "running" as const)]);
    if (state === "running") {
      console.log("[cleanup] killing the app this run launched");
      await run.app.kill();
      await run.app.result;
    }
    run.app = null;
  }
  const listed = await loggedRun("brew", ["list", "--cask", run.cask]);
  if (listed.code === 0) {
    console.log("[cleanup] the cask is still installed; uninstalling it with --zap");
    await loggedRun("brew", ["uninstall", "--zap", "--cask", run.cask]);
  }
  if (run.markerCreated && existsSync(DATA_DIRECTORY)) {
    console.log(`[cleanup] removing the data directory this run created: ${DATA_DIRECTORY}`);
    rmSync(DATA_DIRECTORY, { recursive: true, force: true });
  }
  if (run.tapDir !== null && existsSync(run.tapDir)) {
    console.log(`[cleanup] removing the tap clone ${run.tapDir}`);
    rmSync(run.tapDir, { recursive: true, force: true });
  }
  if (run.mutantTapDir !== null && existsSync(run.mutantTapDir)) {
    console.log(`[cleanup] removing the scratch tap ${run.mutantTapDir}`);
    rmSync(run.mutantTapDir, { recursive: true, force: true });
  }
  try {
    rmSync(run.temp, { recursive: true, force: true });
  } catch (error) {
    console.warn(`cask-install: could not remove ${run.temp}: ${error instanceof Error ? error.message : String(error)}`);
  }
}

async function main(argv: readonly string[]): Promise<number> {
  const options = parseArgs(argv);
  if (options.help) {
    process.stdout.write(`${USAGE}\n`);
    return 0;
  }
  if (options.error !== null) {
    process.stderr.write(`cask-install: ${options.error}\n\n${USAGE}\n`);
    return 2;
  }
  if (process.platform !== "darwin") {
    process.stderr.write(`cask-install: this tool asserts a Homebrew cask install, which is macOS-only (process.platform = ${process.platform}); the macOS job runs it\n`);
    return 2;
  }

  // The one place the run's cask name is decided: tap mode installs the tap's
  // cask, a mutant run installs the derived cask from the scratch tap. Every
  // `brew` command below takes it from the run.
  const mutants = options.mutants.length > 0;
  const run = new CaskRun(mkdtempSync(path.join(tmpdir(), "passquest-cask-install-")), mutants ? MUTANT_TAP_CASK : TAP_CASK);
  try {
    await assertMacosFloor(run);
    if (mutants) {
      console.log(`cask-install: mutants ${options.mutants.join(", ")} (each installs a derived cask from the scratch tap ${MUTANT_TAP_NAME})`);
      const tapText = await cloneTap(run);
      await createMutantTap(run);
      for (const mutant of options.mutants) await runMutant(run, mutant, tapText);
      run.checker.summary(`mutants ${options.mutants.join(", ")}`);
    } else {
      console.log("cask-install: tap mode (the tap's file, installed fully qualified)");
      await runTap(run);
      run.checker.summary("tap mode");
    }
    console.log(`cask-install: ${run.checker.ok ? "every assertion passed" : "FAILED"}`);
    return run.checker.ok ? 0 : 1;
  } catch (error) {
    console.error(`cask-install: ${error instanceof Error ? error.message : String(error)}`);
    run.checker.summary("aborted");
    return 1;
  } finally {
    await cleanup(run);
  }
}

await runMain("cask-install", main);
