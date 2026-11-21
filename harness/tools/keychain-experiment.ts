/**
 * The goal-0018 P1 experiment: two packages (A from an older commit, B from the pull request head)
 * are installed one over the other in a temp prefix, A stores a key through the window, B reads it
 * — and the same three arms run it: `adhoc` (the package as built), `self-signed` (a throwaway
 * identity the workflow creates in the job) and `project` (the coordinator's signing secrets,
 * skipped when they are not in the environment).
 *
 * The tool never assumes which check macOS applies: before and after every read it dumps the item
 * with `security dump-keychain -a` (its partition list and every trusted application's
 * `requirement:`), learns the running build's real partition ID from an item that build itself
 * stores (cross-checked against the `cdhash:` computed from `codesign -dvvv`), and classifies the
 * refusal as the partition list, the application list, both or neither (`classifyRefusal`). A
 * read that is not a success (`refused`, `blocked` or `failed`) continues into the plan's simulated
 * allow: B's partition ID is added to the item with `security set-generic-password-partition-list`,
 * B reads again (expected to succeed), and a third build C — B's bundle with a bumped
 * `CFBundleShortVersionString`, signed the way its arm signs — reads again (expected to be
 * refused): the difference between "the allow records the app" and "it records the build".
 *
 * A read is classified from what happened, never from a message: `read` (the row showed present and
 * the loopback stub received the key), `refused` (the app ran to completion and its window reported
 * the credential absent or unreadable), `blocked` (the read did not complete — the tool's own
 * watchdog kill, the launch helper's hard timeout, or a window that never rendered the row — and a
 * `pgrep -lf SecurityAgent` sampler saw a SecurityAgent while the launch ran, one probe a second,
 * every probe recorded) or `failed` (anything else, with the failure lines). A `failed` read fails
 * the run: the job must not be green when the experiment it exists for did not happen. `codesign
 * -d -r-` prints a stored requirement as `designated => …` and, for code that has none — an ad-hoc
 * signature — the implied one as a source comment, `# designated => cdhash H"…"` (CODESIGN(1),
 * `-r`): both forms are parsed and `implied` is recorded, an ad-hoc build's requirement is expected
 * to be implied, and a re-signed build whose requirement is still implied fails the arm (the
 * re-sign did not take).
 *
 * The experiment's keychain is its own: created under the run's temp tree, made the user default
 * and the sole user search-list entry, restored and deleted in `finally`. The login keychain is
 * left as it was found: each arm's service is looked up there before and after the arm through
 * `security find-generic-password -s <service> <login keychain>` (its exit code only, never `-g`),
 * any hit is deleted in the `finally` and fails the run, and both values are in the record. It is
 * created with an empty password because `security` takes `create-keychain -p`, `import -P`,
 * `unlock-keychain -p` and `set-*-partition-list -k` as command-line arguments only, and this
 * repository's rule treats `security`/`codesign`/`openssl` argv as public: no secret on a command
 * line. The signing password therefore never reaches argv — openssl reads it from the child's
 * environment (`-passin env:…`), the tool imports a passphrase-less copy (0600, under the run's
 * private temp tree, deleted with it), and every decoded or derived file dies with the run too. A
 * passphrase is never printed and never recorded.
 *
 * Every `security`, `codesign`, `hdiutil`, `openssl`, `plutil`, `ditto` and `pgrep` call goes
 * through `loggedRun`, which prints the exact argv (an empty argument prints as `""`, never as a
 * secret). The record (`--out`) carries the numbers and the raw `security dump-keychain -a` item
 * block; the key is only ever recorded as its SHA-256, and the tool fails the run when any key byte
 * reaches a file under its temp tree (the keychain — the store the item exists in — is the one
 * exemption), the record, or the text this tool printed (every print is captured and scanned before
 * it exits, on top of the redaction every recorded line goes through).
 *
 * `--self-check` runs the dump parser, the designated-requirement parser, the guards and the
 * refusal classifier over fixtures that mirror SecurityTool's own print format
 * (Security-61901.120.67, `keychain_utilities.c` and
 * `clientid.cpp`), on any OS, so the pure parts are verified where the keychain calls cannot run.
 * The tool asserts an experiment on macOS only: it exits 2 anywhere else, because a skip would
 * report evidence that never happened (the cask tools' rule).
 */
import { createHash, randomBytes } from "node:crypto";
import { chmodSync, createReadStream, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import path from "node:path";

import { killTree, listProcesses, removeTree } from "../lib/desktop.ts";
import { cleanEnv, launchPage, pageScript, sleep, startProviderStub, writeStubRegistry, LAUNCH_TIMEOUT_MS, STUB_MODEL, STUB_PROVIDER } from "../lib/desktop-page.ts";
import type { Launched, ProviderStub } from "../lib/desktop-page.ts";
import { sha256Hex } from "../lib/hash.ts";
import { field, stringField } from "../lib/json.ts";
import { mygoFileName, packageNaming } from "../lib/package-build.ts";
import { goTarget, hostTarget, targetDir } from "../lib/package.ts";
import { loggedRun, resultText, runMain, tail } from "../lib/process.ts";
import type { RunResult } from "../lib/process.ts";
import { resolve } from "../lib/repo.ts";

/** The account the app stores every provider key under (internal/desktop/credentials.go). */
const ACCOUNT = `api-key:${STUB_PROVIDER}`;
/** Nothing listens on port 1; the desktop smoke's own value, so no real AnkiConnect is touched. */
const UNREACHABLE_ANKI = "http://127.0.0.1:1";
const ARMS = ["adhoc", "self-signed", "project"] as const;
type ArmId = (typeof ARMS)[number];
/** The SecurityAgent sampler's one probe a second while a launch runs. */
const AGENT_PROBE_MS = 1000;
/** The tool's watchdog kill for every launch: early enough that the helper's own hard timeout stays the backstop. */
const WATCHDOG_KILL_MS = LAUNCH_TIMEOUT_MS - 5000;
/** The lines of a launch's output a failure line and the record keep. */
const OUTPUT_TAIL_LINES = 60;

const USAGE = `Run the goal-0018 P1 Keychain experiment on this macOS machine.

  mise exec -- bun harness/tools/keychain-experiment.ts --a-root <dir> --b-root <dir> --out <file> \\
      [--arms ${ARMS.join(",")}]

  --a-root <dir>  the older tree; its own harness must have run \`bun run package\`
  --b-root <dir>  the tree B (usually the pull request head); the same
  --out <file>    where the JSON record is written (always written, also on a failed run)
  --arms <list>   the arms to run, in the given order; default ${ARMS.join(",")}
  --self-check    run the parser, requirement, guard and classifier cases over fixtures (any OS)
  --help          this text

Identity material, base64 or path, never on argv and never printed:
  PASSQUEST_KEYCHAIN_SELF_SIGNED_P12       path to the self-signed arm's passphrase-less p12
  PASSQUEST_KEYCHAIN_SELF_SIGNED_IDENTITY  its CN; both must be set for the arm to run
  PASSQUEST_MACOS_SIGNING_P12              the project arm's p12, base64 (Actions secret)
  PASSQUEST_MACOS_SIGNING_PASSWORD         its passphrase (Actions secret; read via -passin env:)
  PASSQUEST_MACOS_SIGNING_IDENTITY         its CN; derived from the imported certificate when unset
An arm whose material is absent is recorded as skipped by name: a fork pull request carries no
project secrets, so the project arm is the normal skip. Every other skip fails the run — the
self-signed arm's material always exists on the runner the workflow builds it for.

Exit codes: 0 every arm and guard passed (the project arm's secret-driven skip is not a failure), 1
a guard or an arm failed, an arm selected for this run did not reach its read step, or a read came
back \`failed\` (the record names it and the failure lines are printed; a SecurityAgent a read left
behind fails its arm by design), 2 usage or the wrong platform. This tool never runs on Windows or
Linux.`;

interface Options {
  aRoot: string;
  bRoot: string;
  out: string;
  arms: ArmId[];
  selfCheck: boolean;
  help: boolean;
  error: string | null;
}

function parseArgs(argv: readonly string[]): Options {
  const options: Options = { aRoot: "", bRoot: "", out: "", arms: [...ARMS], selfCheck: false, help: false, error: null };
  const arms: ArmId[] = [];
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index] ?? "";
    if (arg === "--help" || arg === "-h") return { ...options, help: true };
    if (arg === "--self-check") {
      options.selfCheck = true;
      continue;
    }
    if (arg === "--a-root" || arg === "--b-root" || arg === "--out" || arg === "--arms") {
      const next = argv[++index];
      if (next === undefined || next === "") return { ...options, error: `${arg} needs a value` };
      if (arg === "--a-root") options.aRoot = next;
      else if (arg === "--b-root") options.bRoot = next;
      else if (arg === "--out") options.out = next;
      else {
        for (const name of next.split(",")) {
          if (!(ARMS as readonly string[]).includes(name)) return { ...options, error: `--arms ${name} is not one of ${ARMS.join(", ")}` };
          if (!arms.includes(name as ArmId)) arms.push(name as ArmId);
        }
      }
      continue;
    }
    return { ...options, error: `unknown argument '${arg}'` };
  }
  if (arms.length > 0) options.arms = arms;
  if (!options.selfCheck && !options.help) {
    if (options.aRoot === "") return { ...options, error: "--a-root <dir> is required" };
    if (options.bRoot === "") return { ...options, error: "--b-root <dir> is required" };
    if (options.out === "") return { ...options, error: "--out <file> is required" };
  }
  return options;
}

/** The keys this run generated, so printed output can be redacted and files scanned. */
const generatedKeys: string[] = [];

/** Every generated key byte sequence replaced by a marker: the record and the log must not hold it. */
function redact(text: string): string {
  let out = text;
  for (const key of generatedKeys) out = out.split(key).join("<key-bytes-redacted>");
  return out;
}

/** One assertion, its exact command and observed value, and why it failed. */
class Checker {
  private readonly failures: string[] = [];
  private passed = 0;

  check(key: string, ok: boolean, observed: string, failure = ""): boolean {
    if (ok) {
      this.passed += 1;
      console.log(`[assert] ${key}: pass — ${redact(observed)}`);
      return true;
    }
    const reason = failure === "" ? observed : failure;
    this.failures.push(`${key}: ${reason}`);
    console.log(`[assert] ${key}: FAIL — ${redact(reason)}`);
    return false;
  }

  /** A failure without a command to answer for (an arm- or run-level guard). */
  fail(message: string): void {
    this.failures.push(message);
    console.log(`[assert] FAIL — ${redact(message)}`);
  }

  get ok(): boolean {
    return this.failures.length === 0;
  }

  get list(): string[] {
    return [...this.failures];
  }

  summary(label: string): void {
    console.log(`[summary] ${label}: ${this.passed} assertion(s) passed, ${this.failures.length} failed`);
    for (const failure of this.failures) console.log(`[summary]   FAIL ${redact(failure)}`);
  }
}

/** The first lines of a command's output, for one assertion line. */
function observed(result: RunResult, limit = 4): string {
  const text = resultText(result).trim();
  if (text === "") return "(no output)";
  return redact(text.split("\n").slice(0, limit).join(" | "));
}

/** The last lines of a command's output, for a failure line (the 0017 lesson: the offence is at the end). */
function observedTail(result: RunResult, limit = 60): string {
  const text = resultText(result).trim();
  if (text === "") return "(no output)";
  const lines = text.split("\n");
  const omitted = lines.length - limit;
  const shown = lines.slice(Math.max(0, omitted)).join(" | ");
  return redact(omitted > 0 ? `(${omitted} earlier line(s) omitted) ${shown}` : shown);
}

/**
 * `loggedRun` with the harness's own `cleanEnv` — no child inherits a PASSQUEST_* variable a shell
 * carries — plus the variables the caller names deliberately.
 */
async function tool(command: string, args: readonly string[], extra: Record<string, string> = {}): Promise<RunResult> {
  const env: Record<string, string> = {};
  for (const [name, value] of Object.entries(cleanEnv())) {
    if (value !== undefined) env[name] = value;
  }
  return loggedRun(command, [...args], { env: { ...env, ...extra }, replaceEnv: true });
}

/** The SHA-256 of a file, streamed so a 45 MB dmg is never held in memory twice. */
async function sha256File(file: string): Promise<string> {
  const hash = createHash("sha256");
  for await (const chunk of createReadStream(file)) hash.update(chunk as Buffer);
  return hash.digest("hex");
}

/** The quoted paths a `security` keychain query prints, in order. */
function quotedPaths(text: string): string[] {
  return [...text.matchAll(/"([^"\n]+)"/g)].map((match) => match[1]!);
}

// ---------------------------------------------------------------------------------------------------
// The dump parser, the guards and the refusal classifier: pure functions, exercised by --self-check
// against fixtures mirroring SecurityTool/macOS/keychain_utilities.c's own print format.
// ---------------------------------------------------------------------------------------------------

/** One ACL entry of an item, as `security dump-keychain -a` prints it. */
interface AclEntry {
  authorizations: string[];
  description: string;
  /** `applications: <null>` prints as "any": an entry any application may use. */
  applications: number | "any";
  requirements: string[];
}

/** The item under test, as the dump shows it. */
interface DumpItem {
  found: boolean;
  /** The raw item block, verbatim. */
  block: string;
  entries: AclEntry[];
  /** Every partition entry's list, comma-split, in the dump's order. */
  partitions: string[];
  /** True when some non-partition entry trusts any application (the owner ACL entry). */
  anyApplication: boolean;
  /** Every trusted application's `requirement:`, with its entry index. */
  requirements: { entry: number; requirement: string }[];
}

/** The attribute a generic-password item's service/account is printed under, e.g. `"svce"<blob>="x"`. */
function attributeOf(block: string, tag: string): string | null {
  const match = new RegExp(`^[ \\t]*"${tag}"<blob>="(.*)"[ \\t]*$`, "m").exec(block);
  return match === null ? null : match[1]!;
}

/** The block of one generic-password item, from a `security dump-keychain -a` text. */
function parseDumpItem(text: string, service: string, account: string): DumpItem {
  const empty: DumpItem = { found: false, block: "", entries: [], partitions: [], anyApplication: false, requirements: [] };
  for (const block of text.split(/^(?=keychain: )/m)) {
    if (attributeOf(block, "svce") !== service) continue;
    if (attributeOf(block, "acct") !== account) continue;
    const entries: AclEntry[] = [];
    let current: AclEntry | null = null;
    let inAccess = false;
    for (const line of block.split("\n")) {
      if (/^access: [0-9]+ entries$/.test(line)) {
        inAccess = true;
        continue;
      }
      if (!inAccess) continue;
      if (/^    entry [0-9]+:$/.test(line)) {
        current = { authorizations: [], description: "", applications: 0, requirements: [] };
        entries.push(current);
        continue;
      }
      if (current === null) continue;
      const authorizations = /^        authorizations \(([0-9]+)\):(.*)$/.exec(line);
      if (authorizations !== null) {
        current.authorizations = authorizations[2]!.trim().split(/\s+/).filter((tag) => tag !== "");
        continue;
      }
      const description = /^        description: (.*)$/.exec(line);
      if (description !== null) {
        current.description = description[1]!;
        continue;
      }
      const applications = /^        applications \(([0-9]+)\):$/.exec(line);
      if (applications !== null) {
        current.applications = Number.parseInt(applications[1]!, 10);
        continue;
      }
      if (line === "        applications: <null>") {
        current.applications = "any";
        continue;
      }
      const requirement = /^                requirement: (.*)$/.exec(line);
      if (requirement !== null) current.requirements.push(requirement[1]!);
    }
    const partitions: string[] = [];
    const requirements: { entry: number; requirement: string }[] = [];
    let anyApplication = false;
    entries.forEach((entry, index) => {
      if (entry.authorizations.includes("partition_id")) {
        // print_partition_id_list prints an unparsable list as "invalid partition ID: <raw>"; a
        // value like that is not a partition id and must not be treated as one (the item guard
        // then fails, which is the point).
        if (entry.description !== "<NULL>" && entry.description !== "" && !entry.description.startsWith("invalid partition ID:")) {
          for (const value of entry.description.split(",")) {
            const trimmed = value.trim();
            if (trimmed !== "") partitions.push(trimmed);
          }
        }
      } else if (entry.applications === "any") {
        anyApplication = true;
      }
      for (const text of entry.requirements) {
        if (text !== "none") requirements.push({ entry: index, requirement: text });
      }
    });
    return { found: true, block, entries, partitions, anyApplication, requirements };
  }
  return empty;
}

/** Why the two built packages fail the premise guard, or null when they carry different cdhashes. */
function buildGuardProblem(aCdhash: string, bCdhash: string): string | null {
  if (aCdhash === "" || bCdhash === "") return "a build carries no cdhash; codesign -dvvv printed none";
  if (aCdhash.toLowerCase() === bCdhash.toLowerCase()) {
    return `the two builds carry the same cdhash ${aCdhash}; every real upgrade has different ones, so the experiment would prove nothing (the documented fallback is a content divergence in B's scratch copy, never a silently accepted experiment)`;
  }
  return null;
}

/** Why the stored item fails the premise guard (it must exist and carry a partition list), or null. */
function itemGuardProblem(item: DumpItem): string | null {
  if (!item.found) return "the item is not in the dump";
  if (item.partitions.length === 0) {
    return "the item carries no partition entry (an unparsable description counts as none), so the simulated allow would use a value nobody wants";
  }
  return null;
}

/** Why the value codesign computes for the build differs from the partition the dump stores, or null. */
function partitionGuardProblem(computed: string, stored: string | null): string | null {
  if (computed === "") return "the build carries no cdhash, so its cdhash: partition id cannot be computed";
  if (stored === null || stored === "") return "the dump shows no partition value for the build";
  if (computed.toLowerCase() !== stored.toLowerCase()) {
    return `codesign computes ${computed} for the build, but the keychain stored ${stored}; the simulated allow would use a value the running process does not have`;
  }
  return null;
}

/** Whether a designated requirement admits a trusted-application requirement, and how it matched. */
function requirementMatches(designated: string, requirement: string): { matches: boolean; method: string } {
  const normal = (text: string): string => text.replace(/\s+/g, " ").trim();
  const dr = normal(designated);
  const req = normal(requirement);
  if (dr === req) return { matches: true, method: "exact" };
  const cdhashes = (text: string): string[] => [...text.matchAll(/cdhash H"([0-9a-fA-F]+)"/g)].map((match) => match[1]!.toLowerCase());
  const wanted = cdhashes(dr);
  const have = cdhashes(req);
  if (wanted.length > 0 && wanted.some((value) => have.includes(value))) return { matches: true, method: "cdhash" };
  return { matches: false, method: "none" };
}

/** Which of the item's two checks admits the build, and which one therefore refuses it. */
interface Classification {
  partition: "admitted" | "absent" | "unknown";
  application: "admitted-any" | "admitted-requirement" | "refused" | "unknown";
  requirementMatch: string;
  refusesBy: "partition" | "application" | "both" | "neither" | "unknown";
  detail: string;
}

function classifyRefusal(item: DumpItem, partitionId: string | null, designatedRequirement: string | null): Classification {
  const partition =
    partitionId === null || partitionId === ""
      ? "unknown"
      : item.partitions.some((value) => value.toLowerCase() === partitionId.toLowerCase())
        ? "admitted"
        : "absent";
  let application: Classification["application"];
  let requirementMatch = "none";
  if (item.anyApplication) {
    application = "admitted-any";
  } else if (item.requirements.length === 0 || designatedRequirement === null || designatedRequirement === "") {
    application = "unknown";
  } else {
    let admitted = false;
    for (const entry of item.requirements) {
      const match = requirementMatches(designatedRequirement, entry.requirement);
      if (match.matches) {
        admitted = true;
        requirementMatch = match.method;
        break;
      }
    }
    application = admitted ? "admitted-requirement" : "refused";
  }
  const partitionOk = partition === "admitted";
  const applicationOk = application === "admitted-any" || application === "admitted-requirement";
  let refusesBy: Classification["refusesBy"];
  if (partitionOk && applicationOk) refusesBy = "neither";
  else if (partitionOk && application === "refused") refusesBy = "application";
  else if (applicationOk && partition === "absent") refusesBy = "partition";
  else if (partition === "absent" && application === "refused") refusesBy = "both";
  else refusesBy = "unknown";
  const detail =
    `the partition list ${partition === "admitted" ? "admits" : partition === "absent" ? "does not admit" : "is unknown for"} ${partitionId ?? "(no id)"}; ` +
    `the application list is ${application === "admitted-any" ? "any application (the owner entry has no requirements)" : application === "admitted-requirement" ? `a requirement matching the designated requirement (${requirementMatch})` : application === "refused" ? "specific requirements that do not match the designated requirement" : "unknown"}`;
  return { partition, application, requirementMatch, refusesBy, detail };
}

// ---------------------------------------------------------------------------------------------------
// --self-check: fixtures in SecurityTool's own print format, so the parser and guards are verified
// where the keychain calls cannot run (any OS). The must-fail cases assert the guard fails.
// ---------------------------------------------------------------------------------------------------

function fixtureDump(service: string, account: string, access: string): string {
  return [
    `keychain: "/Users/runner/work/_temp/passquest-keychain-experiment-self/experiment.keychain"`,
    `version: 512`,
    `class: "genp"`,
    `attributes:`,
    `    0x00000007 <blob>="${service}"`,
    `    0x00000008 <blob>=<NULL>`,
    `    "acct"<blob>="${account}"`,
    `    "cdat"<timedate>=0x32303236313132323031323334355A00  "20261122012345Z\\000"`,
    `    "labl"<blob>="${service}"`,
    `    "svce"<blob>="${service}"`,
    access,
  ].join("\n");
}

const FRESH_ITEM_ACCESS = [
  `access: 2 entries`,
  `    entry 0:`,
  `        authorizations (1): decrypt`,
  `        don't-require-password`,
  `        description: <NULL>`,
  `        applications: <null>`,
  `    entry 1:`,
  `        authorizations (1): partition_id`,
  `        don't-require-password`,
  `        description: cdhash:1111111111111111111111111111111111111111111111111111111111111111`,
  `        applications: <null>`,
].join("\n");

const ALLOWED_APP_ACCESS = [
  `access: 2 entries`,
  `    entry 0:`,
  `        authorizations (1): decrypt`,
  `        don't-require-password`,
  `        description: <NULL>`,
  `        applications (1):`,
  `            0: 0x00000000000000200C51A9F4  " \\000\\012\\000\\000"`,
  `                requirement: identifier "com.github.wxxb789.passquest" and cdhash H"2222222222222222222222222222222222222222"`,
  `    entry 1:`,
  `        authorizations (1): partition_id`,
  `        don't-require-password`,
  `        description: cdhash:1111111111111111111111111111111111111111111111111111111111111111`,
  `        applications: <null>`,
].join("\n");

const BROKEN_PARTITION_ACCESS = [
  `access: 2 entries`,
  `    entry 0:`,
  `        authorizations (1): decrypt`,
  `        don't-require-password`,
  `        description: <NULL>`,
  `        applications: <null>`,
  `    entry 1:`,
  `        authorizations (1): partition_id`,
  `        don't-require-password`,
  `        description: invalid partition ID: <NULL>`,
  `        applications: <null>`,
].join("\n");

interface SelfCase {
  name: string;
  /** null passes; a string is the failure line. */
  run: () => string | null;
}

const A_PARTITION = "cdhash:1111111111111111111111111111111111111111111111111111111111111111";
const B_PARTITION = "cdhash:2222222222222222222222222222222222222222222222222222222222222222";
const B_DR_CDHASH = `designated => cdhash H"2222222222222222222222222222222222222222"`;

const SELF_CASES: readonly SelfCase[] = [
  {
    name: "a fresh item's any-application owner and partition entry",
    run: () => {
      const item = parseDumpItem(fixtureDump("svc", "acct", FRESH_ITEM_ACCESS), "svc", "acct");
      if (!item.found) return "the item was not found";
      if (item.partitions.join(",") !== A_PARTITION) return `partitions = [${item.partitions.join(", ")}]`;
      if (!item.anyApplication) return "the owner entry was not read as any application";
      if (item.requirements.length !== 0) return `requirements = [${item.requirements.map((entry) => entry.requirement).join(", ")}]`;
      if (item.entries.length !== 2) return `entries = ${item.entries.length}`;
      return null;
    },
  },
  {
    name: "an item of another service is not the item",
    run: () => {
      const item = parseDumpItem(fixtureDump("other", "acct", FRESH_ITEM_ACCESS), "svc", "acct");
      return item.found ? "the other service's item was found" : null;
    },
  },
  {
    name: "an unparsable partition description yields no partition list (the item guard fails)",
    run: () => {
      const item = parseDumpItem(fixtureDump("svc", "acct", BROKEN_PARTITION_ACCESS), "svc", "acct");
      if (item.partitions.length !== 0) return `partitions = [${item.partitions.join(", ")}]`;
      const problem = itemGuardProblem(item);
      return problem === null ? "the item guard passed although there is no partition entry" : null;
    },
  },
  {
    name: "two equal cdhashes fail the pair guard; two different ones pass",
    run: () => {
      if (buildGuardProblem("aa", "aa") === null) return "equal cdhashes passed the guard";
      if (buildGuardProblem("aa", "bb") !== null) return "different cdhashes failed the guard";
      if (buildGuardProblem("", "bb") === null) return "a missing cdhash passed the guard";
      return null;
    },
  },
  {
    name: "the computed partition must equal the stored one",
    run: () => {
      if (partitionGuardProblem(A_PARTITION, A_PARTITION) !== null) return "equal partition ids failed the guard";
      if (partitionGuardProblem(A_PARTITION, null) === null) return "a stored value of null passed the guard";
      if (partitionGuardProblem(A_PARTITION, B_PARTITION) === null) return "different partition ids passed the guard";
      return null;
    },
  },
  {
    name: "the partition list refuses a build whose cdhash is absent",
    run: () => {
      const item = parseDumpItem(fixtureDump("svc", "acct", FRESH_ITEM_ACCESS), "svc", "acct");
      const classification = classifyRefusal(item, B_PARTITION, B_DR_CDHASH);
      if (classification.refusesBy !== "partition") return `refusesBy = ${classification.refusesBy}`;
      if (classification.partition !== "absent") return `partition = ${classification.partition}`;
      if (classification.application !== "admitted-any") return `application = ${classification.application}`;
      return null;
    },
  },
  {
    name: "an application requirement matching the DR admits the build (partition admitted too)",
    run: () => {
      const item = parseDumpItem(fixtureDump("svc", "acct", ALLOWED_APP_ACCESS), "svc", "acct");
      const classification = classifyRefusal(item, A_PARTITION, B_DR_CDHASH);
      if (classification.refusesBy !== "neither") return `refusesBy = ${classification.refusesBy}`;
      if (classification.requirementMatch !== "cdhash") return `requirementMatch = ${classification.requirementMatch}`;
      return null;
    },
  },
  {
    name: "an application requirement it does not match refuses the build",
    run: () => {
      const item = parseDumpItem(fixtureDump("svc", "acct", ALLOWED_APP_ACCESS), "svc", "acct");
      const classification = classifyRefusal(item, A_PARTITION, `designated => cdhash H"3333333333333333333333333333333333333333"`);
      if (classification.refusesBy !== "application") return `refusesBy = ${classification.refusesBy}`;
      if (classification.application !== "refused") return `application = ${classification.application}`;
      return null;
    },
  },
  {
    name: "a DR with no cdhash against an app list of requirements is unknown, never a match",
    run: () => {
      const item = parseDumpItem(fixtureDump("svc", "acct", ALLOWED_APP_ACCESS), "svc", "acct");
      const classification = classifyRefusal(item, A_PARTITION, `designated => identifier "com.github.wxxb789.passquest" and certificate root = H"4444"`);
      return classification.requirementMatch === "none" ? null : "a requirement was matched although it should not be";
    },
  },
  {
    name: "codesign's implied requirement (a source comment) parses, and only an ad-hoc build may carry one",
    run: () => {
      const parsed = parseDesignatedRequirement(`Executable=/tmp/A.app/Contents/MacOS/passquest-desktop\n# designated => cdhash H"2222222222222222222222222222222222222222"\n`);
      if (!parsed.implied) return "the commented form was not read as the implied one";
      if (parsed.designatedRequirement !== `cdhash H"2222222222222222222222222222222222222222"`) return `designatedRequirement = ${JSON.stringify(parsed.designatedRequirement)}`;
      if (requirementProblem(parsed, false) !== null) return "an ad-hoc build's implied requirement failed the guard";
      if (requirementProblem(parsed, true) === null) return "a re-signed build's implied requirement passed, although the re-sign did not take";
      return null;
    },
  },
  {
    name: "a stored requirement (the plain form) parses and satisfies the re-sign rule",
    run: () => {
      const stored = `identifier "com.github.wxxb789.passquest" and certificate root = H"4444444444444444444444444444444444444444"`;
      const parsed = parseDesignatedRequirement(`Executable=/tmp/A.app/Contents/MacOS/passquest-desktop\ndesignated => ${stored}\n`);
      if (parsed.implied) return "the plain form was read as implied";
      if (parsed.designatedRequirement !== stored) return `designatedRequirement = ${JSON.stringify(parsed.designatedRequirement)}`;
      const problem = requirementProblem(parsed, true);
      return problem === null ? null : `a stored requirement failed the re-sign rule: ${problem}`;
    },
  },
  {
    name: "an output with no designated requirement at all is empty and fails the guard",
    run: () => {
      const parsed = parseDesignatedRequirement("Executable=/tmp/A.app/Contents/MacOS/passquest-desktop\nSignature=adhoc\n");
      if (parsed.designatedRequirement !== "" || parsed.implied) return `parsed = ${JSON.stringify(parsed)}`;
      return requirementProblem(parsed, false) === null ? "an empty requirement passed the guard" : null;
    },
  },
  {
    name: "a read is classified from what happened, and only a SecurityAgent sample makes it blocked",
    run: () => {
      const provider: ProviderStub = { endpoint: "", received: [], respond: () => new Response("{}"), stop: () => {} };
      const scan = (matches: number): AgentScan => ({
        probes: 3,
        error: null,
        matches: Array.from({ length: matches }, (_, index) => ({
          at: `2026-11-22T00:00:0${index}.000Z`,
          lines: [`${1000 + index} /System/Library/CoreServices/SecurityAgent.app/Contents/MacOS/SecurityAgent`],
        })),
      });
      const outcome = (kind: LaunchOutcome["kind"], value: unknown, matches: number): LaunchOutcome => ({
        kind,
        value,
        output: "…",
        messages: ["the launch did not complete"],
        agentScan: scan(matches),
      });
      const killedWithAgent = classifyRead(outcome("killed", undefined, 1), "k", provider, "fixture");
      if (killedWithAgent.outcome !== "blocked") return `a kill with a SecurityAgent sampled gave ${killedWithAgent.outcome}`;
      const killedWithout = classifyRead(outcome("killed", undefined, 0), "k", provider, "fixture");
      if (killedWithout.outcome !== "failed") return `a kill with no SecurityAgent sampled gave ${killedWithout.outcome}`;
      const stuckWithAgent = classifyRead(outcome("ok", { state: "no-row" }, 1), "k", provider, "fixture");
      if (stuckWithAgent.outcome !== "blocked") return `a window that never rendered the row, with an agent, gave ${stuckWithAgent.outcome}`;
      const stuckWithout = classifyRead(outcome("ok", { state: "no-credential" }, 0), "k", provider, "fixture");
      if (stuckWithout.outcome !== "failed") return `a window that never showed the credential state, without an agent, gave ${stuckWithout.outcome}`;
      const refused = classifyRead(outcome("ok", { state: "missing", errorBar: "the credential store could not be read" }, 0), "k", provider, "fixture");
      if (refused.outcome !== "refused") return `a missing row gave ${refused.outcome}`;
      const carried: ProviderStub = { ...provider, received: [{ authorization: "Bearer k", path: "/v1/chat/completions", raw: "{}", sha256: "0" }] };
      const read = classifyRead(outcome("ok", { state: "present" }, 0), "k", carried, "fixture");
      if (read.outcome !== "read" || !read.providerCarriedKey) return `a present row the stub saw the key for gave ${read.outcome} (carried=${String(read.providerCarriedKey)})`;
      const presentWithout = classifyRead(outcome("ok", { state: "present" }, 0), "k", provider, "fixture");
      if (presentWithout.outcome !== "failed") return `a present row the stub never saw the key for gave ${presentWithout.outcome}`;
      const empty = classifyRead(outcome("ok", {}, 0), "k", provider, "fixture");
      return empty.outcome === "failed" ? null : `an unreadable page state gave ${empty.outcome}`;
    },
  },
];

function selfCheck(): number {
  let failed = 0;
  for (const item of SELF_CASES) {
    const problem = item.run();
    if (problem === null) console.log(`[self-check] ${item.name}: pass`);
    else {
      failed += 1;
      console.log(`[self-check] ${item.name}: FAIL — ${problem}`);
    }
  }
  console.log(`[self-check] ${SELF_CASES.length - failed} of ${SELF_CASES.length} case(s) passed`);
  return failed === 0 ? 0 : 1;
}

// ---------------------------------------------------------------------------------------------------
// Page scripts: A stores a fresh key, B reads the item A stored (the stub proves the key left the
// OS store), or B stores its own item so its real partition ID can be read from the dump.
// ---------------------------------------------------------------------------------------------------

function storeScript(key: string, label: string): string {
  return pageScript(`
const KEY = ${JSON.stringify(key)};
const row = '.rrow[data-provider="${STUB_PROVIDER}"]';
location.hash = "#/settings";
await until("the stub provider row", () => q(row + ' [data-credential="missing"]'));
await click('[data-act="cred"][data-v="${STUB_PROVIDER}"]');
const input = await until("the key field", () => q('#credform input[name="key"]'));
input.value = KEY;
await click('[data-act="credential-save"]');
await until("the stored key", () => q(row + ' [data-credential="present"]'), 20000);
return { stored: true, label: ${JSON.stringify(label)} };
`);
}

function readScript(label: string): string {
  return pageScript(`
const row = '.rrow[data-provider="${STUB_PROVIDER}"]';
location.hash = "#/settings";
// Neither wait throws: a window that never renders the row, or never shows the credential
// state, inside its own bound is the blocked-read shape, and the tool reads that from the
// returned state (with the SecurityAgent samples the launch recorded) instead of a string.
// Both bounds stay inside the smoke hook's own 40 s script bound.
const rowDeadline = Date.now() + 15000;
while (!q(row) && Date.now() < rowDeadline) await sleep(100);
if (!q(row)) {
  const bar = q("#errbar, .error-bar");
  return { state: "no-row", errorBar: bar ? bar.textContent : "", label: ${JSON.stringify(label)} };
}
const deadline = Date.now() + 20000;
let state = "no-credential";
for (;;) {
  const found = q(row + ' [data-credential]');
  if (found) {
    const value = String(found.dataset.credential ?? "");
    if (value === "present" || value === "missing") { state = value; break; }
  }
  if (Date.now() > deadline) break;
  await sleep(100);
}
if (state !== "present") {
  const bar = q("#errbar, .error-bar");
  return { state: state, errorBar: bar ? bar.textContent : "", label: ${JSON.stringify(label)} };
}
await click('[data-act="model-test"][data-v="${STUB_MODEL}"]');
await until("the verified model", () => q(row + ' [data-capability="verified"]'), 30000);
return { state: "present", verified: true, label: ${JSON.stringify(label)} };
`);
}

// ---------------------------------------------------------------------------------------------------
// The experiment's own plumbing
// ---------------------------------------------------------------------------------------------------

interface RootRecord {
  label: string;
  root: string;
  commit: string;
  status: string;
  goarch: string;
  dmg: { name: string; path: string; bytes: number; sha256: string };
}

interface SignatureRecord {
  designatedRequirement: string;
  /** True when `codesign -d -r-` printed the requirement as a source comment (the implied one). */
  implied: boolean;
  requirementOutput: string;
  identifier: string;
  cdhash: string;
  teamIdentifier: string;
  authorities: string[];
  signature: string;
  dvvOutput: string;
  dvvvOutput: string;
  computedPartition: string;
}

/** One `pgrep -lf SecurityAgent` hit, with the probe's instant. */
interface AgentSample {
  at: string;
  lines: string[];
}

/**
 * What the one-probe-a-second `pgrep -lf SecurityAgent` sampler saw while a launch ran. The
 * `blocked` bucket needs it: a prompt nobody could click is the one thing that makes a read that
 * did not complete a blocked read rather than a failed one.
 */
interface AgentScan {
  probes: number;
  /** The first probe that could not run at all; a scan that cannot run leaves `matches` empty. */
  error: string | null;
  matches: AgentSample[];
}

interface LaunchRecord {
  where: string;
  kind: "ok" | "killed" | "failed";
  value: unknown;
  messages: string[];
  outputTail: string;
  /** The SecurityAgent samples taken while this launch ran (one probe a second). */
  agentScan: AgentScan;
}

interface ItemRecord {
  found: boolean;
  block: string;
  partitions: string[];
  anyApplication: boolean;
  requirements: { entry: number; requirement: string }[];
  entryCount: number;
}

interface ReadRecord {
  /**
   * `read` (the row showed present and the stub received the key), `refused` (the app ran to
   * completion and reported the credential absent or unreadable), `blocked` (the read did not
   * complete and a SecurityAgent was sampled while it ran), `failed` (anything else, which fails the
   * run) or `not-run` (the arm stopped before its read; also a failure).
   */
  outcome: "read" | "refused" | "blocked" | "failed" | "not-run";
  detail: string;
  providerCarriedKey: boolean;
  launch: LaunchRecord;
  classification?: Classification;
  securityAgent?: SecurityAgentRecord;
}

interface SecurityAgentRecord {
  found: string[];
  killed: boolean;
  left: string[];
}

interface ArmRecord {
  id: ArmId;
  identity: { kind: "none" | "self-signed" | "project"; cn?: string; source?: string; sha1?: string; couldNotSign?: boolean };
  skipped?: string;
  service: string;
  account: string;
  keySha256: string;
  a: SignatureRecord;
  b: SignatureRecord;
  drSame: boolean;
  /**
   * The login keychain lookups for this arm's service: the exit code before the arm, and one entry
   * per service the arm created afterwards (the arm's own and the allow's `-b` service). Exit 0 is
   * "present"; the lookup never asks for the secret (`-g`/`-w` are never passed, both would
   * withdraw it). Absent when the arm was skipped before it looked.
   */
  login?: { before: { service: string; exit: number }; after: { service: string; exit: number }[] };
  store: { stored: boolean; launch: LaunchRecord };
  itemAfterStore: ItemRecord;
  read: ReadRecord;
  itemAfterRead: ItemRecord;
  allow?: {
    bService: string;
    bKeySha256: string;
    bItem: ItemRecord;
    bPartition: string;
    bPartitionComputed: string;
    list: string;
    command: string;
    exit: number;
    itemAfterAllow: ItemRecord;
    reread: ReadRecord;
    expected: string;
  };
  third?: {
    versionBefore: string;
    versionAfter: string;
    signature: SignatureRecord;
    read: ReadRecord;
    expected: string;
    drEqualsB: boolean;
  };
  failures: string[];
}

interface ExperimentRecord {
  tool: string;
  goal: string;
  startedAt: string;
  endedAt: string | null;
  platform: { swVers: string; machine: string };
  runs: { a?: RootRecord; b?: RootRecord };
  keychain: {
    path: string;
    password: string;
    defaultBefore: string | null;
    searchListBefore: string[] | null;
    /** While the experiment ran: the experiment keychain is the user default and the sole entry. */
    defaultDuring: string | null;
    searchListDuring: string[] | null;
    /** After the restore: equal to the Before values when the restore took. */
    defaultAfter: string | null;
    searchListAfter: string[] | null;
    deleted: boolean;
    /** The learner's own keychain, and every synthetic service this run found there (expected: none). */
    loginPath: string;
    /** Each hit: the service, the lookup's exit code and whether the deletion took. */
    loginResidue: { service: string; found: number; deleted: boolean }[];
  };
  guards: { buildsDiffer: boolean | null; aCdhash: string; bCdhash: string; log: { prints: number; keyBytes: number } };
  arms: ArmRecord[];
  failures: string[];
  notes: string[];
  status: string;
}

interface LaunchOutcome {
  kind: "ok" | "killed" | "failed";
  value: unknown;
  output: string;
  messages: string[];
  agentScan: AgentScan;
}

interface Ctx {
  temp: string;
  keychain: string;
  provider: ProviderStub;
  registry: string;
  record: ExperimentRecord;
  checker: Checker;
  a: RootRecord;
  b: RootRecord;
  stopReason: string | null;
  guardsDone: boolean;
  /** The learner's own keychain: looked up before and after every arm, and swept in the `finally`. */
  loginPath: string;
}

/** A global condition that makes the rest of the run meaningless. */
class StopRun extends Error {}

/** One arm's failure; the next arm still runs, so its evidence is kept. */
class ArmFailure extends Error {}

interface SignatureFields {
  designatedRequirement: string;
  implied: boolean;
  requirementOutput: string;
  identifier: string;
  cdhash: string;
  teamIdentifier: string;
  authorities: string[];
  signature: string;
  dvvOutput: string;
  dvvvOutput: string;
  computedPartition: string;
}

/**
 * The designated requirement `codesign -d -r-` printed, and whether it is the implied one. The
 * command writes a stored requirement as `designated => …` and, for code that carries none — an
 * ad-hoc signature — the implied requirement as a source comment, `# designated => cdhash H"…"`
 * (CODESIGN(1), `-r, --requirements`).
 */
function parseDesignatedRequirement(text: string): { designatedRequirement: string; implied: boolean } {
  const match = /^\s*(#)?\s*designated => (.*)$/m.exec(text);
  if (match === null) return { designatedRequirement: "", implied: false };
  return { designatedRequirement: match[2]!.trim(), implied: match[1] !== undefined };
}

/**
 * Why a printed requirement cannot be used, or null. `explicitRequired` is the rule after a
 * re-sign: the signature must carry the requirement, so the implied form means the re-sign did not
 * take.
 */
function requirementProblem(fields: { designatedRequirement: string; implied: boolean }, explicitRequired: boolean): string | null {
  if (fields.designatedRequirement === "") return "codesign -d -r- printed no designated requirement";
  if (explicitRequired && fields.implied) {
    return "codesign prints the designated requirement as a source comment, the implied one, so the signature carries none: the re-sign did not take";
  }
  return null;
}

function signatureOf(requirement: RunResult, dvv: RunResult, dvvv: RunResult): SignatureFields {
  const requirementText = resultText(requirement);
  const fieldText = `${resultText(dvv)}\n${resultText(dvvv)}`;
  const fields = (name: string): string => new RegExp(`^${name}=(.*)$`, "m").exec(fieldText)?.[1]?.trim() ?? "";
  const cdhash = fields("CDHash");
  const parsed = parseDesignatedRequirement(requirementText);
  return {
    designatedRequirement: parsed.designatedRequirement,
    implied: parsed.implied,
    requirementOutput: requirementText.trim(),
    identifier: fields("Identifier"),
    cdhash,
    teamIdentifier: fields("TeamIdentifier"),
    authorities: [...fieldText.matchAll(/^Authority=(.*)$/gm)].map((match) => match[1]!.trim()),
    signature: fields("Signature"),
    dvvOutput: resultText(dvv).trim(),
    dvvvOutput: resultText(dvvv).trim(),
    computedPartition: cdhash === "" ? "" : `cdhash:${cdhash.toLowerCase()}`,
  };
}

function launchRecord(outcome: LaunchOutcome | null, where: string): LaunchRecord {
  const empty: AgentScan = { probes: 0, error: null, matches: [] };
  if (outcome === null) {
    return { where, kind: "failed", value: null, messages: ["the launch produced no outcome"], outputTail: "", agentScan: empty };
  }
  return {
    where,
    kind: outcome.kind,
    value: outcome.value,
    messages: outcome.messages.map((message) => redact(message)),
    outputTail: tail(redact(outcome.output), OUTPUT_TAIL_LINES),
    agentScan: {
      probes: outcome.agentScan.probes,
      error: outcome.agentScan.error === null ? null : redact(outcome.agentScan.error),
      matches: outcome.agentScan.matches.map((sample) => ({ at: sample.at, lines: sample.lines.map((line) => redact(line)) })),
    },
  };
}

function itemRecord(item: DumpItem): ItemRecord {
  return {
    found: item.found,
    block: item.block,
    partitions: item.partitions,
    anyApplication: item.anyApplication,
    requirements: item.requirements,
    entryCount: item.entries.length,
  };
}

async function inspectRoot(root: string, label: string, checker: Checker): Promise<RootRecord> {
  const mygo = readFileSync(path.join(root, "mygo.json"), "utf8");
  const naming = packageNaming(mygo);
  const target = hostTarget();
  const goarch = goTarget(target).split("/")[1] ?? "";
  const name = mygoFileName(naming, "darwin", goarch);
  if (name === undefined) throw new StopRun(`mygo.json names no darwin package for ${goarch}`);
  const dir = resolve(root, targetDir(target));
  const file = path.join(dir, name);
  if (!existsSync(file)) {
    const listing = existsSync(dir) ? readdirSync(dir).join(", ") : "(no directory)";
    checker.fail(`${label}: ${file} does not exist; run \`bun run package\` in ${root} first (the directory holds ${listing})`);
    throw new StopRun(`${label} carries no ${name} under ${targetDir(target)}`);
  }
  const head = await tool("git", ["-C", root, "rev-parse", "HEAD"]);
  const status = await tool("git", ["-C", root, "status", "--porcelain"]);
  const record: RootRecord = {
    label,
    root,
    commit: head.code === 0 ? head.stdout.trim() : `(git failed: ${observed(head)})`,
    status: status.code === 0 && status.stdout.trim() === "" ? "clean" : `dirty/failed: ${status.stdout.trim() || observed(status)}`,
    goarch,
    dmg: { name, path: file, bytes: statSync(file).size, sha256: await sha256File(file) },
  };
  console.log(`[root ${label}] ${root} at ${record.commit} (${record.status}); ${name} ${record.dmg.bytes} bytes sha256 ${record.dmg.sha256}`);
  return record;
}

/**
 * The SecurityAgent sampler: one `pgrep -lf SecurityAgent` probe a second until `stop()`. Every
 * probe is counted, every hit is kept with its instant, and a probe that cannot run at all is
 * recorded once (its empty `matches` then means "nothing was seen", not "nothing could be seen") —
 * this is the evidence the `blocked` bucket rests on.
 */
function startAgentSampler(): { scan: AgentScan; stop: () => Promise<void> } {
  const scan: AgentScan = { probes: 0, error: null, matches: [] };
  let running = true;
  const loop = (async () => {
    while (running) {
      const at = new Date().toISOString();
      const probe = await tool("pgrep", ["-lf", "SecurityAgent"]);
      scan.probes += 1;
      if (probe.code === 0) {
        const lines = resultText(probe)
          .split("\n")
          .map((line) => line.trim())
          .filter((line) => line !== "");
        if (lines.length > 0) scan.matches.push({ at, lines });
      } else if (probe.code !== 1 && scan.error === null) {
        // `pgrep` exits 1 when nothing matches; anything else is the probe itself failing.
        scan.error = `pgrep exited ${probe.code}: ${observedTail(probe, 4)}`;
      }
      if (!running) break;
      await sleep(AGENT_PROBE_MS);
    }
  })();
  return {
    scan,
    stop: async () => {
      running = false;
      await loop;
    },
  };
}

/** One sampler line for a launch's log line. */
function agentSummary(scan: AgentScan): string {
  if (scan.matches.length === 0) {
    return `the SecurityAgent sampler saw none in ${scan.probes} probe(s)${scan.error === null ? "" : ` (${scan.error})`}`;
  }
  const last = scan.matches[scan.matches.length - 1]!;
  return `the SecurityAgent sampler saw a SecurityAgent in ${scan.matches.length} of ${scan.probes} probe(s), last at ${last.at}: ${last.lines.join(" | ")}`;
}

/** The launch site's shared environment and the launchPage call, with the page script removed after it. */
async function launchWith(
  ctx: Ctx,
  spec: { binary: string; dir: string; label: string; where: string; script: string; service: string; dataDir: string },
): Promise<LaunchOutcome> {
  mkdirSync(spec.dataDir, { recursive: true });
  const messages: string[] = [];
  const sampler = startAgentSampler();
  const startedAt = Date.now();
  let launched: Launched | null = null;
  try {
    launched = await launchPage({
      binary: spec.binary,
      cwd: ctx.temp,
      dir: spec.dir,
      label: spec.label,
      header: `keychain-experiment: goal 0018 P1 (${spec.where})`,
      where: spec.where,
      script: spec.script,
      env: {
        PASSQUEST_DESKTOP_DATA: spec.dataDir,
        PASSQUEST_DESKTOP_SECRET_SERVICE: spec.service,
        PASSQUEST_PROVIDER_REGISTRY: ctx.registry,
        PASSQUEST_ANKI_ENDPOINT: UNREACHABLE_ANKI,
      },
      // Only failures are recorded: a message pushed for every launch, whatever it says, would
      // turn any failed launch into whatever the later reader matched its text against.
      check: (condition, message) => {
        if (!condition) messages.push(message);
        return condition;
      },
      // The tool's own watchdog kill: the launch helper's hard timeout stays the backstop, and a
      // kill comes back as `Launched.killed` instead of a line a reader would have to parse.
      killWhen: async () => Date.now() - startedAt >= WATCHDOG_KILL_MS,
    });
  } finally {
    await sampler.stop();
  }
  const scriptFile = path.join(spec.dir, `${spec.label}.js`);
  if (existsSync(scriptFile)) {
    rmSync(scriptFile, { force: true });
    console.log(`[cleanup] removed the launch script ${scriptFile} (it carried the key for the window)`);
  }
  if (launched === null) {
    const outcome: LaunchOutcome = { kind: "failed", value: undefined, output: "", messages, agentScan: sampler.scan };
    console.log(`[launch ${spec.where}] did not complete — ${messages.map((message) => redact(message)).join("; ") || "no failure line"}; ${redact(agentSummary(sampler.scan))}`);
    return outcome;
  }
  const kind: LaunchOutcome["kind"] = launched.killed ? "killed" : "ok";
  const outcome: LaunchOutcome = { kind, value: launched.value, output: launched.output, messages, agentScan: sampler.scan };
  const line =
    kind === "killed"
      ? `killed by the tool's watchdog at ${Math.round((Date.now() - startedAt) / 1000)} s (the launch helper's ${LAUNCH_TIMEOUT_MS / 1000} s timeout was the backstop)`
      : `exited; value ${JSON.stringify(outcome.value)}`;
  console.log(`[launch ${spec.where}] ${line}; ${redact(agentSummary(sampler.scan))}`);
  return outcome;
}

function classifyRead(outcome: LaunchOutcome, key: string, provider: ProviderStub, where: string): ReadRecord {
  const record = launchRecord(outcome, where);
  const seen = outcome.agentScan.matches.length > 0;
  if (outcome.kind !== "ok") {
    // The read did not complete. Which bucket it belongs to is decided by what was observed while
    // it ran: a SecurityAgent up at the time is the prompt path, and nothing else is `blocked`.
    const observed = `the launch ended ${outcome.kind} (${outcome.messages.join("; ") || "no failure line"}) and ${agentSummary(outcome.agentScan)}`;
    return {
      outcome: seen ? "blocked" : "failed",
      detail: seen ? `${observed}; a read that did not complete while a SecurityAgent was up is the prompt path` : observed,
      providerCarriedKey: false,
      launch: record,
    };
  }
  const state = stringField(outcome.value, "state") ?? "";
  const carried = provider.received.some((request) => request.authorization === `Bearer ${key}`);
  if (state === "present") {
    return carried
      ? { outcome: "read", detail: "the row showed present and the loopback stub received the key bytes", providerCarriedKey: true, launch: record }
      : { outcome: "failed", detail: "the row showed present but the loopback stub never received the key bytes", providerCarriedKey: false, launch: record };
  }
  const bar = stringField(outcome.value, "errorBar") ?? "";
  const barText = bar === "" ? "" : ` (the window's error bar reads ${JSON.stringify(bar)})`;
  if (state === "missing") {
    return {
      outcome: "refused",
      detail: `the row stayed missing${barText}; the dump decides which check refused`,
      providerCarriedKey: false,
      launch: record,
    };
  }
  if (state === "no-row" || state === "no-credential") {
    // The window ran to completion without ever telling the page what the credential state was.
    const stuck =
      state === "no-row"
        ? "the window never rendered the provider row inside its own 15 s bound"
        : "the window never showed the credential state inside its own 20 s bound";
    const detail = `${stuck}${barText}; ${agentSummary(outcome.agentScan)}`;
    return {
      outcome: seen ? "blocked" : "failed",
      detail: seen ? `${detail}; a read that did not complete while a SecurityAgent was up is the prompt path` : detail,
      providerCarriedKey: false,
      launch: record,
    };
  }
  return { outcome: "failed", detail: `the window reported state ${JSON.stringify(state)}${barText}`, providerCarriedKey: false, launch: record };
}

function readLine(arm: ArmId, label: string, read: ReadRecord, expected: string): void {
  console.log(`[read ${arm}${label === "" ? "" : ` ${label}`}] outcome=${read.outcome} (expected ${expected}): ${read.detail}`);
}

/** One `security dump-keychain -a` per stage, parsed for the item. */
async function dumpItem(ctx: Ctx, dir: string, label: string, service: string): Promise<{ record: ItemRecord; raw: DumpItem }> {
  await tool("security", ["unlock-keychain", "-p", "", ctx.keychain]);
  const result = await tool("security", ["dump-keychain", "-a", ctx.keychain]);
  const file = path.join(dir, `dump-${label}.txt`);
  writeFileSync(file, result.stdout);
  if (result.code !== 0) throw new ArmFailure(`security dump-keychain -a exited ${result.code}: ${observedTail(result)}`);
  const item = parseDumpItem(result.stdout, service, ACCOUNT);
  console.log(
    `[dump ${label}] item ${item.found ? "present" : "MISSING"}: ${item.partitions.length} partition value(s) [${item.partitions.join(", ")}], ${item.entries.length} access entr(y|ies), applications ${item.anyApplication ? "any" : `${item.requirements.length} requirement(s)`}`,
  );
  if (item.found) {
    for (const line of item.block.split("\n")) console.log(`[dump ${label}]   ${line}`);
  }
  return { record: itemRecord(item), raw: item };
}

/** The keychain snapshot `security default-keychain`/`list-keychains` print, or nulls when unreadable. */
async function keychainSnapshot(): Promise<{ default: string | null; list: string[] | null }> {
  const def = await tool("security", ["default-keychain", "-d", "user"]);
  const list = await tool("security", ["list-keychains", "-d", "user"]);
  return {
    default: def.code === 0 ? (quotedPaths(def.stdout)[0] ?? null) : null,
    list: list.code === 0 ? quotedPaths(list.stdout) : null,
  };
}

/**
 * Whether one of this run's services exists in the login keychain: the exit code of a lookup that
 * names the keychain and never asks for the secret (`-g` and `-w` both withdraw it, so neither is a
 * neutral way to ask). Exit 0 is "present"; anything else is "not found" and `security` says why.
 */
async function loginLookup(service: string, keychain: string): Promise<number> {
  const found = await tool("security", ["find-generic-password", "-s", service, keychain]);
  return found.code;
}

/**
 * The signing fields `codesign -d -r-`/`-dvv`/`-dvvv` give one installed app, recorded verbatim.
 * `explicitRequired` is set for a read taken after a re-sign: the requirement must then be stored,
 * not printed as the implied one.
 */
async function readSignature(app: string, executable: string, label: string, explicitRequired: boolean): Promise<SignatureRecord> {
  const requirement = await tool("codesign", ["-d", "-r-", app]);
  const dvv = await tool("codesign", ["-dvv", app]);
  const dvvv = await tool("codesign", ["-dvvv", executable]);
  if (requirement.code !== 0 || dvv.code !== 0 || dvvv.code !== 0) {
    throw new ArmFailure(
      `${label}: codesign failed (${requirement.code}/${dvv.code}/${dvvv.code}): ${observedTail(requirement.code !== 0 ? requirement : dvv.code !== 0 ? dvv : dvvv)}`,
    );
  }
  const record = signatureOf(requirement, dvv, dvvv);
  const problem = requirementProblem(record, explicitRequired);
  if (problem !== null) throw new ArmFailure(`${label}: ${problem}: ${observedTail(requirement)}`);
  if (record.cdhash === "") {
    throw new ArmFailure(`${label}: codesign printed no CDHash: ${observedTail(dvvv)}`);
  }
  console.log(
    `[signature ${label}] identifier ${record.identifier} cdhash ${record.cdhash} signature ${record.signature || "(none)"} team ${record.teamIdentifier || "(none)"}; DR ${record.implied ? "(implied, printed as a source comment) " : ""}${record.designatedRequirement}`,
  );
  return record;
}

/** Installs one `.dmg`'s `.app` into `prefix`, naming the bundle and its executable by listing, as the smoke does. */
async function installFromDmg(ctx: Ctx, dmg: RootRecord["dmg"], prefix: string, label: string): Promise<{ app: string; executable: string; bundle: string }> {
  const mount = path.join(ctx.temp, `mount-${label}`);
  mkdirSync(mount, { recursive: true });
  const attach = await tool("hdiutil", ["attach", "-nobrowse", "-readonly", "-mountpoint", mount, dmg.path]);
  if (!ctx.checker.check(`install ${label}: hdiutil attach`, attach.code === 0, `hdiutil attach … -> exit 0: ${observed(attach, 2)}`, `hdiutil attach exited ${attach.code}: ${observedTail(attach)}`)) {
    throw new ArmFailure(`the dmg of ${label} could not be attached`);
  }
  try {
    const rootEntries = readdirSync(mount, { withFileTypes: true });
    const bundles = rootEntries.filter((entry) => entry.isDirectory() && entry.name.endsWith(".app"));
    if (
      !ctx.checker.check(
        `install ${label}: one .app in the image`,
        bundles.length === 1,
        `the image holds ${bundles.map((entry) => entry.name).join(", ")}`,
        `the image ${dmg.name} holds ${rootEntries.map((entry) => entry.name).join(", ") || "nothing"}; expected exactly one .app`,
      )
    ) {
      throw new ArmFailure(`${dmg.name} holds no single .app`);
    }
    const bundle = bundles[0]!.name;
    const app = path.join(prefix, bundle);
    rmSync(app, { recursive: true, force: true });
    mkdirSync(prefix, { recursive: true });
    const copy = await tool("ditto", [path.join(mount, bundle), app]);
    if (!ctx.checker.check(`install ${label}: ditto`, copy.code === 0, `ditto ${bundle} -> exit 0`, `ditto exited ${copy.code}: ${observedTail(copy)}`)) {
      throw new ArmFailure(`ditto could not copy ${bundle}`);
    }
    const macos = path.join(app, "Contents", "MacOS");
    const executables = existsSync(macos) ? readdirSync(macos).filter((name) => !name.startsWith(".")) : [];
    if (
      !ctx.checker.check(
        `install ${label}: one executable in Contents/MacOS`,
        executables.length === 1,
        `Contents/MacOS holds ${executables.join(", ")}`,
        `Contents/MacOS holds ${executables.length} entries (${executables.join(", ")}); expected exactly one executable`,
      )
    ) {
      throw new ArmFailure("the bundle holds no single executable");
    }
    console.log(
      `[install ${label}] ${dmg.name} mounted at ${mount} (image holds ${rootEntries.map((entry) => entry.name).join(", ")}); installed ${app} (the bundle the image named); executable ${executables[0]} (the one non-hidden entry of Contents/MacOS)`,
    );
    return { app, executable: path.join(macos, executables[0]!), bundle };
  } finally {
    const detach = await tool("hdiutil", ["detach", mount]);
    ctx.checker.check(`install ${label}: hdiutil detach`, detach.code === 0, `hdiutil detach ${mount} -> exit 0`, `hdiutil detach ${mount} exited ${detach.code}: ${observedTail(detach)}`);
  }
}

/** An arm's signing identity, imported into the experiment's keychain and usable by codesign. */
interface Identity {
  cn: string;
  sha1: string | null;
  findIdentity: string;
  couldNotSign: boolean;
  source: string;
}

/** The identity material an arm needs, from the environment; a problem string means the arm is skipped. */
interface IdentityInput {
  kind: "self-signed" | "project";
  p12File?: string;
  p12Base64?: string;
  password: string | null;
  cn: string | null;
  source: string;
  problem?: string;
}

function identityInput(id: ArmId): IdentityInput | null {
  if (id === "adhoc") return null;
  if (id === "self-signed") {
    const file = process.env.PASSQUEST_KEYCHAIN_SELF_SIGNED_P12 ?? "";
    const cn = process.env.PASSQUEST_KEYCHAIN_SELF_SIGNED_IDENTITY ?? "";
    const missing = [file === "" ? "PASSQUEST_KEYCHAIN_SELF_SIGNED_P12" : "", cn === "" ? "PASSQUEST_KEYCHAIN_SELF_SIGNED_IDENTITY" : ""].filter((name) => name !== "").join(", ");
    if (missing !== "") return { kind: "self-signed", password: null, cn: null, source: "PASSQUEST_KEYCHAIN_SELF_SIGNED_P12/IDENTITY", problem: `${missing} not set` };
    if (!existsSync(file)) return { kind: "self-signed", p12File: file, password: null, cn, source: "PASSQUEST_KEYCHAIN_SELF_SIGNED_P12", problem: `${file} does not exist` };
    return { kind: "self-signed", p12File: file, password: null, cn, source: "PASSQUEST_KEYCHAIN_SELF_SIGNED_P12" };
  }
  const base64 = process.env.PASSQUEST_MACOS_SIGNING_P12 ?? "";
  const password = process.env.PASSQUEST_MACOS_SIGNING_PASSWORD ?? "";
  const missing = [base64 === "" ? "PASSQUEST_MACOS_SIGNING_P12" : "", password === "" ? "PASSQUEST_MACOS_SIGNING_PASSWORD" : ""].filter((name) => name !== "").join(", ");
  if (missing !== "") {
    return { kind: "project", password: null, cn: null, source: "PASSQUEST_MACOS_SIGNING_P12/PASSWORD", problem: `${missing} not set (a fork pull request carries no project secrets)` };
  }
  const cn = (process.env.PASSQUEST_MACOS_SIGNING_IDENTITY ?? "").trim();
  return { kind: "project", p12Base64: base64, password, cn: cn === "" ? null : cn, source: "PASSQUEST_MACOS_SIGNING_P12/PASSWORD" };
}

/** The subject's CN, however LibreSSL/OpenSSL spell it; null when there is none. */
function commonNameOf(subjectOutput: string): string | null {
  const match = /CN\s*=\s*([^,\n/]+)/.exec(subjectOutput);
  return match === null ? null : match[1]!.trim();
}

/** Decodes the material, imports it into the experiment keychain, and unlocks it for codesign. */
async function importIdentity(ctx: Ctx, id: ArmId, input: IdentityInput, dir: string): Promise<Identity> {
  let importable: string;
  if (input.kind === "self-signed") {
    importable = input.p12File!;
  } else {
    // The coordinator's secret travels base64 in the environment; it is decoded 0600 under the run's
    // private tree and re-packaged with an empty passphrase, because SecurityTool takes the
    // passphrase as an argv argument only — see the tool header.
    const decoded = path.join(dir, "project-signing.p12");
    writeFileSync(decoded, Buffer.from(input.p12Base64!, "base64"), { mode: 0o600 });
    chmodSync(decoded, 0o600);
    if (statSync(decoded).size === 0) throw new ArmFailure("PASSQUEST_MACOS_SIGNING_P12 decoded to an empty file");
    const pem = path.join(dir, "project-signing.pem");
    const decode = await tool("openssl", ["pkcs12", "-in", decoded, "-nodes", "-passin", "env:PASSQUEST_MACOS_SIGNING_PASSWORD", "-out", pem], {
      PASSQUEST_MACOS_SIGNING_PASSWORD: input.password!,
    });
    chmodSync(pem, 0o600);
    if (
      !ctx.checker.check(
        `identity ${id}: openssl pkcs12 -in`,
        decode.code === 0,
        `openssl pkcs12 -in … -passin env:… -> exit 0 (the passphrase never reaches argv)`,
        `openssl pkcs12 exited ${decode.code}: ${observedTail(decode)}`,
      )
    ) {
      throw new ArmFailure("the project p12 could not be decoded");
    }
    importable = path.join(dir, "project-signing-passphraseless.p12");
    const reexport = await tool("openssl", ["pkcs12", "-export", "-in", pem, "-out", importable, "-passout", "pass:"]);
    chmodSync(importable, 0o600);
    if (
      !ctx.checker.check(
        `identity ${id}: openssl pkcs12 -export`,
        reexport.code === 0,
        `openssl pkcs12 -export -passout pass: -> exit 0`,
        `openssl pkcs12 -export exited ${reexport.code}: ${observedTail(reexport)}`,
      )
    ) {
      throw new ArmFailure("the project p12 could not be re-exported with an empty passphrase");
    }
  }
  const imported = await tool("security", ["import", importable, "-f", "pkcs12", "-k", ctx.keychain, "-P", "", "-T", "/usr/bin/codesign"]);
  const importedCount = /([0-9]+) identit(?:y|ies) imported/.exec(imported.stdout)?.[1] ?? "0";
  ctx.checker.check(
    `identity ${id}: security import`,
    imported.code === 0 && importedCount !== "0",
    `security import … -P "" -> exit ${imported.code}: ${observed(imported, 2)}`,
    `security import exited ${imported.code} or imported no identity: ${observedTail(imported)}`,
  );
  if (imported.code !== 0 || importedCount === "0") throw new ArmFailure(`the ${id} identity could not be imported`);
  const partitioned = await tool("security", ["set-key-partition-list", "-S", "apple-tool:,apple:", "-s", "-k", "", ctx.keychain]);
  ctx.checker.check(
    `identity ${id}: set-key-partition-list`,
    partitioned.code === 0,
    `security set-key-partition-list -S apple-tool:,apple: -s -k "" -> exit 0`,
    `security set-key-partition-list exited ${partitioned.code}: ${observedTail(partitioned)}`,
  );
  if (partitioned.code !== 0) throw new ArmFailure("codesign cannot use the imported key without its partition list");
  // The command prints every key it touched; an empty result means no signing key matched (the
  // codesign step below is the authority, but the log should carry what this step saw).
  console.log(`[identity ${id}] set-key-partition-list prints: ${observed(partitioned, 3)}`);
  const listed = await tool("security", ["find-identity", "-p", "codesigning", ctx.keychain]);
  console.log(`[identity ${id}] security find-identity -p codesigning -> exit ${listed.code}: ${observed(listed, 4)}`);
  let cn = input.cn;
  if (cn === null) {
    const certFile = path.join(dir, "identity-cert.pem");
    const certificateOut = await tool("openssl", ["pkcs12", "-in", importable, "-nokeys", "-passin", "pass:", "-out", certFile]);
    const subject = await tool("openssl", ["x509", "-in", certFile, "-noout", "-subject"]);
    cn =
      certificateOut.code === 0 && subject.code === 0
        ? commonNameOf(`${subject.stdout}${subject.stderr}`)
        : ([...listed.stdout.matchAll(/^[ \t]*[0-9]+\)[ \t]+[0-9A-Fa-f]{40}[ \t]+"([^"]+)"/gm)].map((match) => match[1]!)[0] ?? null);
    console.log(`[identity ${id}] derived the certificate's CN ${JSON.stringify(cn)} from ${certificateOut.code === 0 ? "the imported p12" : "security find-identity"}`);
  }
  if (cn === null || cn === "") throw new ArmFailure(`the ${id} identity's CN could not be named; set the environment's CN variable`);
  const certificate = await tool("security", ["find-certificate", "-a", "-Z", "-c", cn, ctx.keychain]);
  const sha1 = /SHA-1 hash: ([0-9a-fA-F]{40})/.exec(certificate.stdout)?.[1]?.toLowerCase() ?? null;
  console.log(`[identity ${id}] imported (${imported.stdout.trim() || "(no output)"}); certificate SHA-1 ${sha1 ?? "(not printed)"}; signing as ${JSON.stringify(cn)}`);
  return { cn, sha1, findIdentity: listed.stdout.trim(), couldNotSign: false, source: input.source };
}

/** `codesign --force --deep --sign <identity>` — MyGo's own argv without the production-only flags. */
async function signBundle(app: string, identity: string): Promise<RunResult> {
  return tool("codesign", ["--force", "--deep", "--sign", identity, app]);
}

/** Signs the installed bundle with the identity, falling back to the certificate's SHA-1, or fails the arm. */
async function signOrFail(ctx: Ctx, arm: ArmRecord, identity: Identity, app: string): Promise<void> {
  let signed = await signBundle(app, identity.cn);
  if (signed.code !== 0) {
    console.log(`[sign ${arm.id}] codesign with ${JSON.stringify(identity.cn)} exited ${signed.code}: ${observedTail(signed)}`);
    if (identity.sha1 !== null) {
      console.log(`[sign ${arm.id}] retrying with the certificate's SHA-1 ${identity.sha1} (the plan's fallback)`);
      signed = await signBundle(app, identity.sha1);
    }
  }
  if (signed.code !== 0) {
    identity.couldNotSign = true;
    arm.identity.couldNotSign = true;
    ctx.checker.fail(
      `arm ${arm.id}: the identity could not sign the installed bundle; security find-identity -p codesigning prints: ${identity.findIdentity || "(no output)"}; codesign's last output: ${observedTail(signed)}`,
    );
    throw new ArmFailure(`the ${arm.id} arm could not sign (could not sign is not a self-signed result)`);
  }
}

/** After a read that is not a success: name, and clear, a SecurityAgent a prompt left behind (the plan's guard). */
async function settleSecurityAgent(ctx: Ctx, arm: ArmRecord, into: ReadRecord): Promise<void> {
  const table = await listProcesses();
  if ("error" in table) {
    ctx.checker.fail(`arm ${arm.id}: the process table cannot be read after the ${into.outcome} read: ${table.error}`);
    return;
  }
  const agents = table.items.filter((entry) => (entry.command.split(" ")[0] ?? "").endsWith("/SecurityAgent") || entry.command === "SecurityAgent");
  if (agents.length === 0) {
    into.securityAgent = { found: [], killed: false, left: [] };
    console.log(`[agent ${arm.id}] no SecurityAgent after the ${into.outcome} read`);
    return;
  }
  const lines = agents.map((entry) => `${entry.pid} ${entry.command}`);
  console.log(`[agent ${arm.id}] SecurityAgent left by the ${into.outcome} read: ${lines.join(" | ")}; killing it (a prompt nobody could click), then re-checking`);
  ctx.checker.fail(`arm ${arm.id}: a SecurityAgent was left by the ${into.outcome} read — ${lines.join(" | ")} — killed so the next arm starts clean (the plan's guard fails the arm)`);
  for (const entry of agents) await killTree(entry.pid);
  await sleep(1500);
  const again = await listProcesses();
  const left = "error" in again ? [] : again.items.filter((entry) => (entry.command.split(" ")[0] ?? "").endsWith("/SecurityAgent") || entry.command === "SecurityAgent");
  into.securityAgent = { found: lines, killed: true, left: left.map((entry) => `${entry.pid} ${entry.command}`) };
  if (left.length > 0) throw new StopRun(`a SecurityAgent survived the kill: ${into.securityAgent.left.join(" | ")}; stopping before the next arm`);
}

/** Everything one arm does, in the plan's order; failures are recorded and the next arm still runs. */
async function runArm(ctx: Ctx, id: ArmId): Promise<void> {
  const service = `passquest-keychain-experiment-${process.pid}-${Date.now()}-${id}`;
  const arm: ArmRecord = {
    id,
    identity: { kind: id === "adhoc" ? "none" : id },
    service,
    account: ACCOUNT,
    keySha256: "",
    a: {} as SignatureRecord,
    b: {} as SignatureRecord,
    drSame: false,
    store: { stored: false, launch: launchRecord(null, `${id} A store`) },
    itemAfterStore: { found: false, block: "", partitions: [], anyApplication: false, requirements: [], entryCount: 0 },
    read: { outcome: "not-run", detail: "the arm stopped before its read step", providerCarriedKey: false, launch: launchRecord(null, `${id} B read`) },
    itemAfterRead: { found: false, block: "", partitions: [], anyApplication: false, requirements: [], entryCount: 0 },
    failures: [],
  };
  ctx.record.arms.push(arm);
  const input = identityInput(id);
  if (input !== null && input.problem !== undefined) {
    arm.skipped = input.problem;
    console.log(`[arm ${id}] skipped — ${input.problem}`);
    return;
  }
  const dir = path.join(ctx.temp, `arm-${id}`);
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  chmodSync(dir, 0o700);
  try {
    arm.login = { before: { service, exit: await loginLookup(service, ctx.loginPath) }, after: [] };
    console.log(`[login ${id}] before the arm: the login keychain lookup for ${service} exited ${arm.login.before.exit} (0 would mean the item is already there)`);
    let identity: Identity | null = null;
    if (input !== null) {
      identity = await importIdentity(ctx, id, input, dir);
      arm.identity = { kind: input.kind, cn: identity.cn, source: identity.source, sha1: identity.sha1 ?? undefined };
    }
    await tool("security", ["unlock-keychain", "-p", "", ctx.keychain]);

    // A: install, sign for the arm, record.
    const prefix = path.join(dir, "install");
    const installedA = await installFromDmg(ctx, ctx.a.dmg, prefix, `${id}-a`);
    const aBuilt = await readSignature(installedA.app, installedA.executable, `${id}-a-built`, false);
    if (identity !== null) await signOrFail(ctx, arm, identity, installedA.app);
    // A re-signed build must carry its requirement: an implied one means the re-sign did not take.
    arm.a = identity === null ? aBuilt : await readSignature(installedA.app, installedA.executable, `${id}-a`, true);

    // A stores the key; the dump shows the ACL the learner would later allow against.
    const keyA = randomBytes(16).toString("hex");
    generatedKeys.push(keyA);
    arm.keySha256 = sha256Hex(keyA);
    const stored = await launchWith(ctx, {
      binary: installedA.executable,
      dir,
      label: `${id}-a-store`,
      where: `${id} A store`,
      script: storeScript(keyA, `${id}-a-store`),
      service,
      dataDir: path.join(dir, "data-a"),
    });
    arm.store.launch = launchRecord(stored, `${id} A store`);
    arm.store.stored = field(stored.value, "stored") === true;
    if (
      !ctx.checker.check(
        `arm ${id}: A stored the key through the window`,
        arm.store.stored,
        `the store script returned stored=${String(field(stored.value, "stored"))}`,
        `A could not store the key: kind ${stored.kind}; ${stored.messages.join("; ")}; the app's last output lines: ${arm.store.launch.outputTail}`,
      )
    ) {
      throw new ArmFailure("A never stored the key");
    }
    const afterStore = await dumpItem(ctx, dir, `${id}-after-store`, service);
    arm.itemAfterStore = afterStore.record;
    const itemProblem = itemGuardProblem(afterStore.raw);
    if (itemProblem !== null) {
      ctx.checker.fail(`arm ${id}: ${itemProblem}`);
      throw new ArmFailure(itemProblem);
    }
    const aPartition = afterStore.raw.partitions.length === 1 ? afterStore.raw.partitions[0]! : null;
    const partitionProblem = partitionGuardProblem(arm.a.computedPartition, aPartition);
    if (partitionProblem !== null) {
      ctx.checker.fail(`arm ${id}: ${partitionProblem}`);
      throw new ArmFailure(partitionProblem);
    }
    console.log(`[state ${id}] A's stored partition ${aPartition} equals the computed ${arm.a.computedPartition}`);

    // B: install over A, sign for the arm, record; the pair guard runs on the first arm.
    const installedB = await installFromDmg(ctx, ctx.b.dmg, prefix, `${id}-b`);
    const bBuilt = await readSignature(installedB.app, installedB.executable, `${id}-b-built`, false);
    if (!ctx.guardsDone) {
      const problem = buildGuardProblem(aBuilt.cdhash, bBuilt.cdhash);
      // The log scan in the same object is filled in at the end of the run.
      ctx.record.guards.buildsDiffer = problem === null;
      ctx.record.guards.aCdhash = aBuilt.cdhash;
      ctx.record.guards.bCdhash = bBuilt.cdhash;
      if (!ctx.checker.check(`guard: the two builds carry different cdhashes`, problem === null, `${aBuilt.cdhash} != ${bBuilt.cdhash}`, problem ?? "")) {
        throw new StopRun(problem!);
      }
      ctx.guardsDone = true;
    }
    if (identity !== null) await signOrFail(ctx, arm, identity, installedB.app);
    arm.b = identity === null ? bBuilt : await readSignature(installedB.app, installedB.executable, `${id}-b`, true);
    arm.drSame = arm.a.designatedRequirement === arm.b.designatedRequirement;
    console.log(
      `[state ${id}] A and B carry the same designated requirement: ${arm.drSame} (A ${arm.a.implied ? "implied " : ""}${arm.a.designatedRequirement}; B ${arm.b.implied ? "implied " : ""}${arm.b.designatedRequirement})`,
    );

    // B reads A's item.
    await tool("security", ["unlock-keychain", "-p", "", ctx.keychain]);
    ctx.provider.received.length = 0;
    const read = await launchWith(ctx, {
      binary: installedB.executable,
      dir,
      label: `${id}-b-read`,
      where: `${id} B read`,
      script: readScript(`${id}-b-read`),
      service,
      dataDir: path.join(dir, "data-b"),
    });
    arm.read = classifyRead(read, keyA, ctx.provider, `${id} B read`);
    readLine(id, "before the allow", arm.read, "read, refused or blocked (a failed read is red at the arm's end)");
    const afterRead = await dumpItem(ctx, dir, `${id}-after-read`, service);
    arm.itemAfterRead = afterRead.record;
    arm.read.classification = classifyRefusal(afterRead.raw, arm.b.computedPartition, arm.b.designatedRequirement);
    console.log(`[classify ${id}] refusesBy=${arm.read.classification.refusesBy}: ${arm.read.classification.detail}`);
    // Every read that is not a success continues: a prompt has to be cleared and covered even when
    // its bucket is uncertain (a `failed` read is red at the arm's end, not silently left alone).
    if (arm.read.outcome !== "read") await settleSecurityAgent(ctx, arm, arm.read);
    if (arm.read.outcome !== "read") {
      await simulateAllow(ctx, arm, { dir, service, installed: installedB, keyA });
    }
  } catch (error) {
    if (error instanceof StopRun) throw error;
    const message = error instanceof Error ? error.message : String(error);
    arm.failures.push(message);
    ctx.checker.fail(`arm ${id}: ${message}`);
  }
  await concludeArm(ctx, arm);
}

/**
 * The end of one arm: the login keychain is looked up again for every service the arm created, and
 * the arm's reads are checked against what a green run may report. A read whose outcome is `failed`
 * means the experiment did not happen for that arm, so it fails the run (the record keeps the
 * failure lines, printed here in full).
 */
async function concludeArm(ctx: Ctx, arm: ArmRecord): Promise<void> {
  if (arm.skipped !== undefined) return;
  const login = arm.login;
  if (login !== undefined) {
    for (const service of [arm.service, ...(arm.allow === undefined ? [] : [arm.allow.bService])]) {
      login.after.push({ service, exit: await loginLookup(service, ctx.loginPath) });
    }
    const hits = login.after.filter((entry) => entry.exit === 0);
    console.log(
      `[login ${arm.id}] after the arm: ${login.after.map((entry) => `${entry.service} -> exit ${entry.exit}`).join("; ")}${hits.length === 0 ? "" : " (a hit is deleted, and fails the run, in the finally)"}`,
    );
  }
  if (arm.read.outcome === "not-run") {
    ctx.checker.fail(`arm ${arm.id}: the arm did not reach its read step (${arm.failures.join("; ") || "no failure was recorded"}), so this run proves nothing about it`);
    return;
  }
  readFailure(ctx, `arm ${arm.id}`, arm.read);
  if (arm.allow !== undefined) readFailure(ctx, `arm ${arm.id} after the allow`, arm.allow.reread);
  if (arm.third !== undefined) readFailure(ctx, `arm ${arm.id} third build`, arm.third.read);
}

/** One read's `failed` outcome: the run must not be green, and the launch's own last lines are printed. */
function readFailure(ctx: Ctx, where: string, read: ReadRecord): void {
  if (read.outcome !== "failed") return;
  console.log(`[tail ${where}] the last ${OUTPUT_TAIL_LINES} lines of the launch's output:\n${read.launch.outputTail}`);
  ctx.checker.fail(`${where}: the read failed (${read.detail}), so this run proves nothing and must not be green`);
}

/** The plan's simulated allow: B's runtime partition id joins the item, then a third build C. */
async function simulateAllow(
  ctx: Ctx,
  arm: ArmRecord,
  spec: { dir: string; service: string; installed: { app: string; executable: string; bundle: string }; keyA: string },
): Promise<void> {
  const { dir, service, installed } = spec;
  const bService = `${service}-b`;
  const keyB = randomBytes(16).toString("hex");
  generatedKeys.push(keyB);
  const stored = await launchWith(ctx, {
    binary: installed.executable,
    dir,
    label: `${arm.id}-b-store`,
    where: `${arm.id} B store`,
    script: storeScript(keyB, `${arm.id}-b-store`),
    service: bService,
    dataDir: path.join(dir, "data-b-store"),
  });
  if (
    !ctx.checker.check(
      `allow ${arm.id}: B stored its own item`,
      field(stored.value, "stored") === true,
      `the store script returned stored=${String(field(stored.value, "stored"))}`,
      `B could not store its own item: kind ${stored.kind}; ${stored.messages.join("; ")}; the app's last output lines: ${tail(redact(stored.output), OUTPUT_TAIL_LINES)}`,
    )
  ) {
    throw new ArmFailure("B never stored its own item, so its runtime partition id is unknown");
  }
  const bDump = await dumpItem(ctx, dir, `${arm.id}-b-item`, bService);
  if (bDump.raw.partitions.length !== 1) {
    throw new ArmFailure(`B's own item carries ${bDump.raw.partitions.length} partition values (${bDump.raw.partitions.join(", ")}); expected exactly one to learn B's runtime partition id`);
  }
  const bPartition = bDump.raw.partitions[0]!;
  const bComputed = arm.b.computedPartition;
  if (
    !ctx.checker.check(
      `allow ${arm.id}: B's runtime partition equals its computed one`,
      bPartition.toLowerCase() === bComputed.toLowerCase(),
      `${bPartition} == ${bComputed}`,
      `the keychain stored ${bPartition} for B but codesign computes ${bComputed}`,
    )
  ) {
    throw new ArmFailure("B's runtime partition id is not the computed one; the simulated allow would use a value nobody wants");
  }
  const existing = arm.itemAfterStore.partitions;
  const list = [...existing, bPartition].join(",");
  const set = await tool("security", ["set-generic-password-partition-list", "-S", list, "-k", "", "-s", service, "-a", ACCOUNT, ctx.keychain]);
  const command = `security set-generic-password-partition-list -S "${list}" -k "" -s ${service} -a ${ACCOUNT} <keychain>`;
  if (!ctx.checker.check(`allow ${arm.id}: add B's partition to the item`, set.code === 0, `${command} -> exit 0`, `${command} exited ${set.code}: ${observedTail(set)}`)) {
    throw new ArmFailure("the simulated allow could not add B's partition id");
  }
  console.log(`[allow ${arm.id}] ${command} (the learner's Always Allow records the requester's requirement and partition; a runner can click nothing, so this simulates the partition half)`);
  await tool("security", ["unlock-keychain", "-p", "", ctx.keychain]);
  const afterAllow = await dumpItem(ctx, dir, `${arm.id}-after-allow`, service);
  const applied = [...arm.itemAfterStore.partitions, bPartition].every((value) =>
    afterAllow.raw.partitions.some((stored) => stored.toLowerCase() === value.toLowerCase()),
  );
  if (!ctx.checker.check(`allow ${arm.id}: the dump shows the new partition list`, applied, `the item now carries [${afterAllow.raw.partitions.join(", ")}]`, `the item still carries [${afterAllow.raw.partitions.join(", ")}]; the allow did not change the stored ACL`)) {
    throw new ArmFailure("the simulated allow did not change the stored partition list");
  }
  ctx.provider.received.length = 0;
  const reread = await launchWith(ctx, {
    binary: installed.executable,
    dir,
    label: `${arm.id}-b-read-again`,
    where: `${arm.id} B read after the allow`,
    script: readScript(`${arm.id}-b-read-again`),
    service,
    dataDir: path.join(dir, "data-b-again"),
  });
  const rereadRecord = classifyRead(reread, spec.keyA, ctx.provider, `${arm.id} B read after the allow`);
  readLine(arm.id, "after the allow", rereadRecord, "read");
  arm.allow = {
    bService,
    bKeySha256: sha256Hex(keyB),
    bItem: bDump.record,
    bPartition,
    bPartitionComputed: bComputed,
    list,
    command,
    exit: set.code,
    itemAfterAllow: afterAllow.record,
    reread: rereadRecord,
    expected: "read",
  };
  await thirdBuild(ctx, arm, { dir, service, installed, keyA: spec.keyA });
}

/** C: B's bundle with a bumped version, signed the way the arm signs, installed over B. */
async function thirdBuild(
  ctx: Ctx,
  arm: ArmRecord,
  spec: { dir: string; service: string; installed: { app: string; executable: string; bundle: string }; keyA: string },
): Promise<void> {
  const { dir, service, installed } = spec;
  const cDir = path.join(dir, "third");
  mkdirSync(cDir, { recursive: true });
  const cApp = path.join(cDir, installed.bundle);
  rmSync(cApp, { recursive: true, force: true });
  const copy = await tool("ditto", [installed.app, cApp]);
  if (!ctx.checker.check(`third ${arm.id}: ditto B's bundle`, copy.code === 0, `ditto -> exit 0`, `ditto exited ${copy.code}: ${observedTail(copy)}`)) {
    throw new ArmFailure("the third build's copy failed");
  }
  const plist = path.join(cApp, "Contents", "Info.plist");
  const before = await tool("plutil", ["-extract", "CFBundleShortVersionString", "raw", "-o", "-", plist]);
  const versionBefore = before.code === 0 ? before.stdout.trim() : "";
  if (versionBefore === "") throw new ArmFailure(`the third build's Info.plist carries no CFBundleShortVersionString: ${observedTail(before)}`);
  const versionAfter = `${versionBefore}.1`;
  const setVersion = await tool("plutil", ["-replace", "CFBundleShortVersionString", "-string", versionAfter, plist]);
  if (!ctx.checker.check(`third ${arm.id}: bump CFBundleShortVersionString`, setVersion.code === 0, `plutil -replace CFBundleShortVersionString -string ${versionAfter} -> exit 0`, `plutil exited ${setVersion.code}: ${observedTail(setVersion)}`)) {
    throw new ArmFailure("the third build's version could not be bumped");
  }
  const identity = arm.identity.kind === "none" ? "-" : (arm.identity.cn ?? "");
  const signed = await signBundle(cApp, identity);
  if (!ctx.checker.check(`third ${arm.id}: sign the bumped bundle`, signed.code === 0, `codesign --force --deep --sign ${JSON.stringify(identity)} -> exit 0`, `codesign exited ${signed.code}: ${observedTail(signed)}`)) {
    throw new ArmFailure("the third build could not be signed");
  }
  rmSync(installed.app, { recursive: true, force: true });
  const install = await tool("ditto", [cApp, installed.app]);
  if (!ctx.checker.check(`third ${arm.id}: install over B`, install.code === 0, `ditto -> exit 0`, `ditto exited ${install.code}: ${observedTail(install)}`)) {
    throw new ArmFailure("the third build could not be installed over B");
  }
  // C is signed the way its arm signs: an ad-hoc C is expected to print the implied requirement.
  const signature = await readSignature(installed.app, installed.executable, `${arm.id}-c`, arm.identity.kind !== "none");
  console.log(`[state ${arm.id}] the third build carries version ${versionAfter} (was ${versionBefore}), cdhash ${signature.cdhash}; its DR equals B's: ${signature.designatedRequirement === arm.b.designatedRequirement}`);
  ctx.provider.received.length = 0;
  const read = await launchWith(ctx, {
    binary: installed.executable,
    dir,
    label: `${arm.id}-c-read`,
    where: `${arm.id} C read`,
    script: readScript(`${arm.id}-c-read`),
    service,
    dataDir: path.join(dir, "data-c"),
  });
  const record = classifyRead(read, spec.keyA, ctx.provider, `${arm.id} C read`);
  readLine(arm.id, "the third build", record, "refused");
  arm.third = {
    versionBefore,
    versionAfter,
    signature,
    read: record,
    expected: "refused",
    drEqualsB: signature.designatedRequirement === arm.b.designatedRequirement,
  };
  if (record.outcome !== "read") await settleSecurityAgent(ctx, arm, record);
}

/** One file's occurrences of every generated key. */
function keyOccurrences(file: string, keys: readonly string[]): number {
  let bytes: Buffer;
  try {
    bytes = readFileSync(file);
  } catch {
    return 0;
  }
  let count = 0;
  for (const key of keys) {
    const needle = Buffer.from(key, "utf8");
    let from = 0;
    for (;;) {
      const at = bytes.indexOf(needle, from);
      if (at === -1) break;
      count += 1;
      from = at + 1;
    }
  }
  return count;
}

/** Every file under a tree, so the leak guard can name the carrier. */
function filesUnder(dir: string): string[] {
  if (!existsSync(dir)) return [];
  const files: string[] = [];
  const walk = (current: string): void => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const child = path.join(current, entry.name);
      if (entry.isDirectory()) walk(child);
      else files.push(child);
    }
  };
  walk(dir);
  return files;
}

/** Writes the record, redacting (and failing on) any key bytes it would carry. */
function writeRecord(record: ExperimentRecord, checker: Checker, stopReason: string | null, out: string): void {
  record.failures = stopReason === null ? checker.list : [...checker.list, `stopped: ${stopReason}`];
  record.status = checker.ok ? (stopReason === null ? "ok" : "stopped") : "failed";
  record.endedAt = new Date().toISOString();
  let text = `${JSON.stringify(record, null, 2)}\n`;
  const carriers = generatedKeys.filter((key) => text.includes(key));
  if (carriers.length > 0) {
    for (const key of carriers) text = text.split(key).join("<key-bytes-redacted>");
    checker.fail(`the record would have carried ${carriers.length} key byte sequence(s); they were redacted (this still fails the run)`);
    record.failures = checker.list;
    record.status = "failed";
    text = `${JSON.stringify(record, null, 2)}\n`;
  }
  for (const key of generatedKeys) {
    if (text.includes(key)) throw new Error("the record still carries key bytes after redaction; refusing to write it");
  }
  writeFileSync(out, text);
  console.log(`[record] wrote ${out} (${Buffer.byteLength(text, "utf8")} bytes, status ${record.status})`);
}

async function runExperiment(options: Options): Promise<number> {
  const checker = new Checker();
  // Everything this tool prints, captured so the run can scan its own log for key bytes: the log is
  // uploaded next to the record, and a redaction the record gets must not be missing from it.
  const printed: string[] = [];
  for (const level of ["log", "error"] as const) {
    const original = console[level].bind(console);
    console[level] = (...args: unknown[]) => {
      printed.push(args.map((arg) => (typeof arg === "string" ? arg : String(arg))).join(" "));
      original(...args);
    };
  }
  const loginPath = path.join(homedir(), "Library", "Keychains", "login.keychain-db");
  const record: ExperimentRecord = {
    tool: "harness/tools/keychain-experiment.ts",
    goal: "0018",
    startedAt: new Date().toISOString(),
    endedAt: null,
    platform: { swVers: "", machine: "" },
    runs: {},
    keychain: {
      path: "",
      password: "empty (see the tool header)",
      defaultBefore: null,
      searchListBefore: null,
      defaultDuring: null,
      searchListDuring: null,
      defaultAfter: null,
      searchListAfter: null,
      deleted: false,
      loginPath,
      loginResidue: [],
    },
    guards: { buildsDiffer: null, aCdhash: "", bCdhash: "", log: { prints: 0, keyBytes: 0 } },
    arms: [],
    failures: [],
    notes: [
      "The key is recorded only as its SHA-256; the launch scripts that carried it for the window are removed after each launch, and the keychain is deleted with the run.",
      "A read is classified from what happened, never from a message: `read`, `refused`, `blocked` (the read did not complete and the one-probe-a-second SecurityAgent sampler saw a SecurityAgent while the launch ran) or `failed`; a `failed` read fails the run, so a green run is one where the experiment actually happened.",
      "The login keychain is left as found: each arm's service is looked up there before and after the arm (the lookup's exit code only, never -g, which would withdraw the secret), and the finally deletes any hit and fails the run.",
    ],
    status: "running",
  };
  const temp = mkdtempSync(path.join(tmpdir(), "passquest-keychain-experiment-"));
  const keychain = path.join(temp, "experiment.keychain");
  const registry = path.join(temp, "registry.json");
  const provider = startProviderStub();
  writeStubRegistry(registry, provider.endpoint);
  const swVers = await tool("sw_vers", ["-productVersion"]);
  const machine = await tool("uname", ["-m"]);
  record.platform.swVers = swVers.stdout.trim();
  record.platform.machine = machine.stdout.trim();
  record.keychain.path = keychain;
  console.log(`[host] macOS ${record.platform.swVers} on ${record.platform.machine}`);

  let stopReason: string | null = null;
  try {
    const a = await inspectRoot(path.resolve(options.aRoot), "a", checker);
    const b = await inspectRoot(path.resolve(options.bRoot), "b", checker);
    record.runs = { a, b };
    if (a.commit !== "(git failed)" && a.commit === b.commit) {
      checker.fail(`A and B are the same commit ${a.commit}; an upgrade needs two`);
    }

    const create = await tool("security", ["create-keychain", "-p", "", keychain]);
    if (!checker.check("keychain: security create-keychain", create.code === 0, `security create-keychain -p "" ${keychain} -> exit 0`, `security create-keychain exited ${create.code}: ${observedTail(create)}`)) {
      throw new StopRun("the experiment keychain could not be created");
    }
    const before = await keychainSnapshot();
    record.keychain.defaultBefore = before.default;
    record.keychain.searchListBefore = before.list;
    console.log(`[keychain] before: default ${before.default ?? "(none)"}; search list ${(before.list ?? []).join(", ") || "(none)"}`);
    const setDefault = await tool("security", ["default-keychain", "-d", "user", "-s", keychain]);
    const setList = await tool("security", ["list-keychains", "-d", "user", "-s", keychain]);
    checker.check(
      "keychain: made the experiment keychain the user default and sole search entry",
      setDefault.code === 0 && setList.code === 0,
      `security default-keychain -s -> ${setDefault.code}; security list-keychains -s -> ${setList.code}`,
      `security default-keychain exited ${setDefault.code}, security list-keychains exited ${setList.code}: ${observedTail(setDefault.code === 0 ? setList : setDefault)}`,
    );
    const during = await keychainSnapshot();
    record.keychain.defaultDuring = during.default;
    record.keychain.searchListDuring = during.list;
    checker.check(
      "keychain: the experiment keychain is the user default and the sole search entry",
      during.default === keychain && (during.list ?? []).length === 1 && during.list?.[0] === keychain,
      `default ${during.default ?? "(none)"}; search list ${(during.list ?? []).join(", ") || "(none)"}`,
      `the app can still reach another store: default ${during.default ?? "(none)"}, search list ${(during.list ?? []).join(", ") || "(none)"}; the item would land somewhere this run does not measure`,
    );

    const ctx: Ctx = { temp, keychain, provider, registry, record, checker, a, b, stopReason: null, guardsDone: false, loginPath };
    for (const id of options.arms) {
      if (ctx.stopReason !== null) break;
      console.log(`\n[arm ${id}] starting`);
      try {
        await runArm(ctx, id);
      } catch (error) {
        if (error instanceof StopRun) {
          ctx.stopReason = error.message;
          break;
        }
        throw error;
      }
    }
    // A run that proved nothing is red: the pair guard's premise must have been compared, and only
    // the project arm's secret-driven skip is one the plan sanctions.
    if (record.guards.buildsDiffer === null) {
      checker.fail("guard: the two builds' cdhashes were never compared (no arm reached the pair guard), so the run's premise is unproven");
    }
    for (const arm of record.arms) {
      if (arm.skipped === undefined || arm.id === "project") continue;
      checker.fail(`arm ${arm.id}: skipped (${arm.skipped}); only the project arm's secret-driven skip is expected, so this run proves nothing about the ${arm.id} arm`);
    }
    stopReason = ctx.stopReason;
  } catch (error) {
    if (error instanceof StopRun) stopReason = error.message;
    else throw error;
  } finally {
    provider.stop();
    if (record.keychain.defaultBefore !== null) {
      const restored = await tool("security", ["default-keychain", "-d", "user", "-s", record.keychain.defaultBefore]);
      console.log(`[cleanup] restored the default keychain to ${record.keychain.defaultBefore} (exit ${restored.code})`);
    }
    if (record.keychain.searchListBefore !== null && record.keychain.searchListBefore.length > 0) {
      const restored = await tool("security", ["list-keychains", "-d", "user", "-s", ...record.keychain.searchListBefore]);
      console.log(`[cleanup] restored the user search list to ${record.keychain.searchListBefore.join(", ")} (exit ${restored.code})`);
    }
    // After the restore, so the record shows the state the run left behind rather than the state it
    // made for itself (that one is `defaultDuring`/`searchListDuring`).
    const after = await keychainSnapshot();
    record.keychain.defaultAfter = after.default;
    record.keychain.searchListAfter = after.list;
    if (record.keychain.defaultBefore !== null) {
      checker.check(
        "keychain: the user default keychain was restored",
        after.default === record.keychain.defaultBefore,
        `default ${after.default ?? "(none)"}, as before the run`,
        `the default keychain is ${after.default ?? "(none)"}; this run found ${record.keychain.defaultBefore}`,
      );
    }
    if (record.keychain.searchListBefore !== null) {
      const want = record.keychain.searchListBefore.join(", ");
      checker.check(
        "keychain: the user search list was restored",
        (after.list ?? []).join(", ") === want,
        `search list ${(after.list ?? []).join(", ") || "(none)"}, as before the run`,
        `the search list is ${(after.list ?? []).join(", ") || "(none)"}; this run found ${want || "(empty)"}`,
      );
    }
    const deleted = await tool("security", ["delete-keychain", keychain]);
    record.keychain.deleted = deleted.code === 0;
    console.log(`[cleanup] deleted the experiment keychain ${keychain} (exit ${deleted.code})`);
    // The learner's own keychain, left as found: every service this run created is looked up there,
    // and a hit is deleted and fails the run — the item belongs in the experiment keychain measured.
    let loginLookups = 0;
    for (const arm of record.arms) {
      if (arm.skipped !== undefined) continue;
      for (const service of [arm.service, ...(arm.allow === undefined ? [] : [arm.allow.bService])]) {
        loginLookups += 1;
        const found = await loginLookup(service, loginPath);
        if (found !== 0) continue;
        const deletion = await tool("security", ["delete-generic-password", "-s", service, loginPath]);
        record.keychain.loginResidue.push({ service, found, deleted: deletion.code === 0 });
        checker.fail(
          `the login keychain held the synthetic item ${service} (find-generic-password exit 0); it was deleted (exit ${deletion.code}) — the item should have been in the experiment keychain`,
        );
      }
    }
    console.log(`[cleanup] the login keychain ${loginPath}: ${loginLookups} lookup(s), ${record.keychain.loginResidue.length} hit(s) (a hit is deleted and fails the run)`);
    const leaks = filesUnder(temp)
      .filter((file) => path.resolve(file) !== path.resolve(keychain))
      .map((file) => ({ file, count: keyOccurrences(file, generatedKeys) }))
      .filter((entry) => entry.count > 0);
    if (leaks.length > 0) {
      checker.fail(`the key bytes reached the filesystem: ${leaks.map((entry) => `${entry.file} (${entry.count})`).join(", ")}`);
    } else {
      console.log(`[guard] no key bytes under ${temp} outside the keychain itself (which the run deletes)`);
    }
    // The log the workflow uploads: every line this tool printed, scanned for the keys it generated.
    const logHits = generatedKeys.filter((key) => printed.join("\n").includes(key));
    record.guards.log = { prints: printed.length, keyBytes: logHits.length };
    if (logHits.length > 0) {
      checker.fail(
        `the key bytes reached this run's log: ${logHits.length} of ${generatedKeys.length} generated key(s) appear in the ${printed.length} line(s) this tool printed, and the log is uploaded next to the record`,
      );
    } else {
      console.log(`[guard] no key bytes in the ${printed.length} line(s) this tool printed`);
    }
    if (stopReason !== null) console.log(`[stop] ${stopReason}`);
    // The record is written after the temp tree is gone: a write that throws then cannot leave the
    // decoded signing material (the project arm re-exports a passphrase-less p12) behind.
    if (!(await removeTree(temp))) console.log(`[cleanup] could not remove ${temp}`);
    else console.log(`[cleanup] removed ${temp} and every dump or decoded signing material in it`);
    writeRecord(record, checker, stopReason, path.resolve(options.out));
  }
  checker.summary("keychain experiment");
  return checker.ok ? 0 : 1;
}

async function main(argv: readonly string[]): Promise<number> {
  const options = parseArgs(argv);
  if (options.help) {
    process.stdout.write(`${USAGE}\n`);
    return 0;
  }
  if (options.error !== null) {
    process.stderr.write(`keychain-experiment: ${options.error}\n\n${USAGE}\n`);
    return 2;
  }
  if (options.selfCheck) return selfCheck();
  if (process.platform !== "darwin") {
    process.stderr.write(`keychain-experiment: this tool asserts a Keychain experiment, which is macOS-only (process.platform = ${process.platform}); the macOS job runs it\n`);
    return 2;
  }
  return runExperiment(options);
}

await runMain("keychain-experiment", main);
