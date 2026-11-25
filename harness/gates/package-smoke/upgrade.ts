/**
 * The forward-upgrade case (goal 0013, step P7). It builds the previous
 * release's CLI into a git worktree under the gate's temp tree, creates
 * learner data through that build, opens the data root with the installed
 * package, and proves the new build carried every record forward.
 *
 * - The previous release is PREVIOUS_RELEASE_COMMIT: the commit of the latest
 *   published release tag (v0.15.1). Before any release existed it was the
 *   last main commit before goal 0013 (357b4c6, Q-0013-06); each release moves
 *   it to the new tag's commit (docs/release.md). The commit is fetched (`--depth=1`,
 *   one commit, not the checkout depth) only when the checkout does not
 *   already hold it, because CI checks out depth 2; a failed fetch fails the
 *   case with the git output.
 * - The old data is made through the base commit's `./cmd/passquest` CLI, not
 *   an old package: at that commit the CLI and the desktop open the same
 *   database through the same `internal/store` (the desktop's workspace calls
 *   the same store.Open), so the CLI writes exactly the shape the old
 *   desktop wrote, and the case needs no old package to install.
 *   Independent and assisted attempts, a finished and an active assessment
 *   run, a note pushed to the id-persisting Anki stub, and a note accepted
 *   while the stub is stopped all go through the old CLI's real commands.
 * - The upgrade is the installed app itself: it launches on that data root
 *   through the same per-user root redirection the study case uses, and opens
 *   (and, were the schema older, migrates) the database. The case then
 *   compares the old build's snapshot against the upgraded file with
 *   `bun:sqlite`, read-only and without ATTACH, never while the app runs:
 *   opening may not add or lose a row in any old table first, then every
 *   table of the old schema, every column that existed before, every row that
 *   existed before, field by field, naming every difference. New columns and
 *   tables are allowed.
 * - The installed app then runs twice more, against a stub restarted over its
 *   state file (the same deck the old build's push wrote). First the smoke
 *   hook drives its notes room to drain the queued note: the old build's
 *   active run is in the way (the room's model listing is refused with
 *   `assessment_active` while a run is active), so the script abandons the run
 *   through the window's own resume card — which also exercises the new build
 *   on the old run; the run's row must read abandoned afterwards — and the
 *   room's push button then adds exactly one note, the queued one.
 * - The revision of the pushed note runs through the installed app's own
 *   binary, which is the CLI when given arguments
 *   (`cmd/passquest-desktop/main.go`). The window cannot revise an accepted
 *   note (`Study.OpenNoteDraft` restores only a pending draft and refuses an
 *   accepted one, workspace.OpenNoteDraft, and the notes list has no edit
 *   action), so the packaged binary accepts revision 2 of the pushed note's
 *   draft and pushes it against the same stub, on the packaged Node runtime
 *   and Anki host the binary resolves for itself: it passes its own resources
 *   directory into the CLI's composition root (cmd/passquest-desktop), and
 *   this launch's PATH carries no node, so no PASSQUEST_* resolution override
 *   is set or needed. The stub must show one `updateNoteFields` for the note id the
 *   old build's push created — never a second `addNote` for it — with the
 *   revision's edit in its fields.
 * - A copy of the upgraded database with its schema version set above the
 *   supported one (the P4 Go test's own way) must leave the packaged app in
 *   the `database_newer` error state, and the file and its side files must be
 *   byte-identical after the launch (sha256 before and after; a side file that
 *   appears or disappears counts).
 *
 * Disclosed limits: while the base commit and this build persist the same
 * schema version, no migration statement runs — the case then proves a
 * same-version reopen by the packaged build, not a migration step. The two
 * `user_version` values are read from the old snapshot and the upgraded file
 * at run time and printed in the pass line. The old CLI's
 * note push runs with the repository as cwd so it finds `agent/anki-host.mjs`
 * and its `node_modules`, exactly as the CLI smokes do; the data itself is
 * created by the base commit's binary.
 */
import { Database } from "bun:sqlite";
import { createHash } from "node:crypto";
import { copyFileSync, existsSync, mkdirSync, readFileSync } from "node:fs";
import path from "node:path";

import { startAnkiStub } from "../../lib/anki-stub.ts";
import { pageScript } from "../../lib/desktop-page.ts";
import type { Check } from "../../lib/desktop-page.ts";
import { removeTree } from "../../lib/desktop.ts";
import { field, numberField, stringField } from "../../lib/json.ts";
import { executable, run, tail } from "../../lib/process.ts";
import type { RunResult } from "../../lib/process.ts";
import { seedReviewedSource } from "../../lib/reviewed-source.ts";
import type { GateContext } from "../../lib/types.ts";
import { launch, textEnv, UNREACHABLE_ENDPOINT } from "../desktop-smoke/scenario.ts";
import type { ScenarioRun } from "../desktop-smoke/scenario.ts";

/**
 * The previous release: the commit of tag v0.15.1, the latest published
 * release, which the upgrade check starts from. The full object id, because
 * `git fetch --depth=1 origin <id>` fetches by full id only. After each
 * release, move it to the new tag's commit (docs/release.md).
 */
export const PREVIOUS_RELEASE_COMMIT = "43a09b175a0a8eb954a6210a4cfbf734ac7e1047";

/** One launch site of the installed app: the smoke-hook run and the per-user data root it resolves. */
export interface UpgradeSite {
  run: ScenarioRun;
  dataRoot: string;
}

/** The installed app's own files the revision runs through: its binary (the CLI when given arguments) and its Node runtime and Anki host. */
export interface UpgradeApp {
  binary: string;
  runtime: string;
  hosts: readonly string[];
}

export interface UpgradeInputs {
  ctx: GateContext;
  check: Check;
  /** The gate's temp tree; the case makes its own subdirectories under it. */
  temp: string;
  /** The installed app: the revision of the pushed note runs through its own binary (see the doc comment). */
  app: UpgradeApp;
  /** Creates a per-user-root launch site for the installed app (the study case's `siteOf`). */
  site: (id: string) => UpgradeSite;
}

/** The columns of one table, as PRAGMA table_info reports them. */
interface ColumnInfo {
  name: string;
  pk: number;
}

export interface DatabaseDiff {
  /** The tables of the old schema that were compared, in name order. */
  tables: string[];
  /** Old rows found again in the upgraded database. */
  rows: number;
  /** Field comparisons run (old rows present after the upgrade × old columns). */
  fields: number;
  /** Every difference: a lost table or column, a missing row, or a changed value (naming table, row key and column). */
  problems: string[];
}

/**
 * Copies the database and the side files that exist beside it as one set. The
 * writer has exited first: a copy of a live WAL is not a snapshot.
 */
function copyDatabaseFamily(from: string, to: string): void {
  for (const suffix of ["", "-wal", "-shm", "-journal"]) {
    if (!existsSync(from + suffix)) continue;
    copyFileSync(from + suffix, to + suffix);
  }
}

/** One value in a comparable spelling: type-tagged so `1` and `'1'` differ, blobs hashed as hex. */
function canonical(value: unknown): string {
  if (value === null || value === undefined) return "null";
  if (typeof value === "number") return `n:${value}`;
  if (typeof value === "bigint") return `n:${value}`;
  if (typeof value === "string") return `s:${value}`;
  if (value instanceof Uint8Array) return `b:${Buffer.from(value).toString("hex")}`;
  return `?:${String(value)}`;
}

/** A SQL identifier, quoted. Table and column names come from the old schema, never from a literal. */
function quote(identifier: string): string {
  return `"${identifier.replaceAll('"', '""')}"`;
}

/** A value for a failure line, byte-bounded so one long blob cannot flood the gate log. */
function shorten(value: string): string {
  return value.length <= 80 ? value : `${value.slice(0, 77)}...`;
}

/** Every difference as one line, capped at a generous count so one database cannot flood the gate log; the tail is counted, never dropped silently. */
function listProblems(problems: readonly string[], cap = 20): string {
  if (problems.length <= cap) return problems.join("; ");
  return `${problems.slice(0, cap).join("; ")}; and ${problems.length - cap} more`;
}

/**
 * Compares two databases row by row and field by field, driven by the old
 * schema: the tables and columns are the ones the before database declares,
 * and every old row must still hold identical values in every old column. A
 * lost table, a lost column and a missing row are problems too; new tables and
 * new columns are not. Both files are opened read-only, no ATTACH.
 */
export function compareDatabases(beforePath: string, afterPath: string): DatabaseDiff {
  const diff: DatabaseDiff = { tables: [], rows: 0, fields: 0, problems: [] };
  const before = new Database(beforePath, { readonly: true });
  const after = new Database(afterPath, { readonly: true });
  try {
    const listed = before
      .query<{ name: string }, []>("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
      .all();
    const afterTables = new Set(
      after.query<{ name: string }, []>("SELECT name FROM sqlite_master WHERE type = 'table'").all().map((entry) => entry.name),
    );
    for (const { name } of listed) {
      diff.tables.push(name);
      if (!afterTables.has(name)) {
        diff.problems.push(`table ${name} is missing after the upgrade`);
        continue;
      }
      const columns = before.query<ColumnInfo, []>(`PRAGMA table_info(${quote(name)})`).all();
      const names = columns.map((column) => column.name);
      const afterColumns = new Set(
        after.query<ColumnInfo, []>(`PRAGMA table_info(${quote(name)})`).all().map((column) => column.name),
      );
      const dropped = names.filter((column) => !afterColumns.has(column));
      if (dropped.length > 0) {
        diff.problems.push(`table ${name} lost column(s) ${dropped.join(", ")}`);
        continue;
      }
      const keys = columns
        .filter((column) => column.pk > 0)
        .sort((left, right) => left.pk - right.pk)
        .map((column) => column.name);
      // Every learner table carries a primary key; a keyless one would still be
      // matched and ordered by its rowid.
      const key = keys.length > 0 ? keys : ["rowid"];
      const selected = [...names, ...key.filter((column) => !names.includes(column))];
      const select = `SELECT ${selected.map(quote).join(", ")} FROM ${quote(name)} ORDER BY ${key.map(quote).join(", ")}`;
      const beforeRows = before.query<Record<string, unknown>, []>(select).all();
      const afterRows = after.query<Record<string, unknown>, []>(select).all();
      const afterByKey = new Map<string, Record<string, unknown>>();
      for (const row of afterRows) afterByKey.set(key.map((column) => canonical(row[column])).join("\u0000"), row);
      for (const row of beforeRows) {
        const where = `${name} ${key.map((column) => `${column}=${canonical(row[column])}`).join(",")}`;
        const match = afterByKey.get(key.map((column) => canonical(row[column])).join("\u0000"));
        if (match === undefined) {
          diff.problems.push(`row ${where} is missing after the upgrade`);
          continue;
        }
        for (const column of names) {
          diff.fields++;
          const was = canonical(row[column]);
          const now = canonical(match[column]);
          if (was !== now) diff.problems.push(`row ${where} column ${column}: ${shorten(was)} became ${shorten(now)}`);
        }
      }
      diff.rows += beforeRows.length;
    }
  } finally {
    before.close();
    after.close();
  }
  return diff;
}

/** sha256 of each existing file of the database family, keyed by base name. */
function familyDigests(database: string): Record<string, string> {
  const digests: Record<string, string> = {};
  for (const suffix of ["", "-wal", "-shm", "-journal"]) {
    const file = database + suffix;
    if (!existsSync(file)) continue;
    digests[path.basename(file)] = createHash("sha256").update(readFileSync(file)).digest("hex");
  }
  return digests;
}

/** The file's `user_version`, the schema version SQLite stores; the old snapshot's and the upgraded file's are printed side by side. */
function schemaVersion(database: string): number {
  const db = new Database(database, { readonly: true });
  try {
    return db.query<{ user_version: number }, []>("PRAGMA user_version").get()?.user_version ?? 0;
  } finally {
    db.close();
  }
}

/** Every table's row count, keyed by name, from the file itself. */
function tableRowCounts(database: string): Record<string, number> {
  const db = new Database(database, { readonly: true });
  try {
    const counts: Record<string, number> = {};
    const tables = db
      .query<{ name: string }, []>("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
      .all();
    for (const { name } of tables) counts[name] = db.query<{ n: number }, []>(`SELECT COUNT(*) AS n FROM ${quote(name)}`).get()?.n ?? 0;
    return counts;
  } finally {
    db.close();
  }
}

/**
 * The old tables whose row count changed between the two files, one line per
 * table: opening the old data may not add or lose a row. A table the upgraded
 * file lost is the comparison's own report, not this check's.
 */
export function rowCountProblems(beforePath: string, afterPath: string): string[] {
  const before = tableRowCounts(beforePath);
  const after = tableRowCounts(afterPath);
  const problems: string[] = [];
  for (const [name, was] of Object.entries(before)) {
    const now = after[name];
    if (now === undefined || now === was) continue;
    problems.push(`table ${name}: ${was} row(s) before the open, ${now} after`);
  }
  return problems;
}

/** True when the checkout holds the commit; false when it must be fetched. */
async function holdsCommit(root: string, commit: string): Promise<boolean> {
  const check = await run(executable("git"), ["-C", root, "cat-file", "-e", `${commit}^{commit}`]);
  return check.code === 0;
}

/** The git output of a failed step, for a failure line. */
function gitOutput(result: { stdout: string; stderr: string }): string {
  return tail(`${result.stdout}${result.stderr}`, 3);
}

/** One old-CLI call: the base commit's binary, `--db` at the old data root, `--json`, repository cwd. */
type OldCall = (args: string[], stdin?: string) => Promise<{ code: number; stderr: string; stdout: string }>;

/** A pipe-driven `assess start` run: the stdin the items are answered on, and the collected result. */
interface AssessmentRun {
  stdin: Bun.Subprocess<"pipe", "pipe", "pipe">["stdin"];
  /**
   * The exit code with stdout and stderr read to the end (the assess smoke's
   * own collector): a large report cannot fill a pipe and block the child.
   */
  result: Promise<RunResult>;
}

/** A run of `assess start` on the old CLI, driven over its stdin. */
function startAssessment(oldBin: string, cwd: string, database: string, size: number): AssessmentRun {
  const proc = Bun.spawn({
    cmd: [oldBin, "assess", "start", "--size", String(size), "--minutes", "60", "--db", database, "--json"],
    cwd,
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  });
  const result = (async (): Promise<RunResult> => {
    const [stdout, stderr] = await Promise.all([new Response(proc.stdout).text(), new Response(proc.stderr).text()]);
    return { code: await proc.exited, stdout, stderr };
  })();
  return { stdin: proc.stdin, result };
}

/** The report `assess report RUN` prints, parsed; empty after a failed call. */
async function assessmentReport(old: OldCall, runID: string): Promise<unknown> {
  const result = await old(["assess", "report", runID]);
  try {
    return JSON.parse(result.stdout);
  } catch {
    return {};
  }
}

/** The facts the case asserts about the old build's data, read straight from the database read-only. */
interface OldData {
  independent: number;
  assisted: number;
  finishedRuns: number;
  activeRuns: number;
  abandonedRuns: number;
  /** The active run's id, so the app's own Today must name the same run. */
  activeRunId: number;
  pushedNotes: number;
  queuedNotes: number;
  openDrafts: number;
}

function readOldData(database: string): OldData {
  const db = new Database(database, { readonly: true });
  try {
    const one = (sql: string): number => db.query<{ n: number }, []>(sql).get()?.n ?? 0;
    return {
      independent: one("SELECT COUNT(*) AS n FROM attempts WHERE assistance = 'independent'"),
      assisted: one("SELECT COUNT(*) AS n FROM attempts WHERE assistance = 'assisted'"),
      finishedRuns: one("SELECT COUNT(*) AS n FROM assessment_runs WHERE status = 'completed'"),
      activeRuns: one("SELECT COUNT(*) AS n FROM assessment_runs WHERE status = 'active'"),
      abandonedRuns: one("SELECT COUNT(*) AS n FROM assessment_runs WHERE status = 'abandoned'"),
      activeRunId: one("SELECT COALESCE(MAX(id), 0) AS n FROM assessment_runs WHERE status = 'active'"),
      pushedNotes: one("SELECT COUNT(*) AS n FROM notes WHERE anki_state = 'pushed'"),
      queuedNotes: one("SELECT COUNT(*) AS n FROM notes WHERE anki_state = 'queued'"),
      openDrafts: one("SELECT COUNT(*) AS n FROM note_drafts WHERE status = 'open'"),
    };
  } finally {
    db.close();
  }
}

/** One note row the case needs after the old CLI ran. */
interface NoteRow {
  noteId: string;
  revision: number;
  front: string;
  ankiState: string;
  /** INTEGER in the schema: 0 until Anki answered a push. */
  ankiNoteId: number;
}

function readNotes(database: string): NoteRow[] {
  const db = new Database(database, { readonly: true });
  try {
    return db
      .query<NoteRow, []>(
        "SELECT note_id AS noteId, revision, front, anki_state AS ankiState, anki_note_id AS ankiNoteId FROM notes ORDER BY note_id",
      )
      .all();
  } finally {
    db.close();
  }
}

/**
 * The upgrade launch: the app opens the old data root (the workspace opens the
 * store before the window exists, so this launch is what migrates), and the
 * script reads Today through the bridge. The boot route is not waited on:
 * Today's active assessment run makes the window open the run's route, and
 * this script only needs the service.
 */
const OPEN_SCRIPT = `
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
const deadline = Date.now() + 20000;
while (!(window.mygo && window.mygo.call) && Date.now() < deadline) await sleep(100);
let value = null;
try {
  const today = await window.mygo.call("Study.Today");
  value = { activeRun: today && today.active_run ? today.active_run.id : null, generated: !!(today && today.generated_at) };
} catch (error) {
  value = { error: String(error && error.message) };
}
return value;
`;

/**
 * The queue-drain launch. The old build's active run is in the way first: the
 * window opens its resume card, and the note room's model listing is refused
 * (`assessment_active`) while a run is active, so the script abandons the run
 * through the window — the learner's own path — and then opens the notes
 * room, whose push button needs no draft. The revision itself cannot go
 * through the window: `Study.OpenNoteDraft` restores only a pending draft and
 * refuses an accepted one (workspace.OpenNoteDraft), and the notes list has
 * no edit action, so the case revises through the installed app's own binary
 * against the upgraded database.
 */
const FLUSH_SCRIPT = `
await until("the old run's resume card", () => q('[data-act="run-abandon-ask"]'), 25000);
await click('[data-act="run-abandon-ask"]');
await until("the abandon dialog", () => q('[data-act="run-abandon"]'), 10000);
await click('[data-act="run-abandon"]');
await until("the abandoned run", () => !q('[data-act="run-abandon-ask"]'), 20000);
location.hash = "#/anki";
await until("the notes list", () => q('[data-testid="note"]') && q('[data-testid="sync-counts"]'), 20000);
const before = {
  counts: data('[data-testid="sync-counts"]'),
  notes: all('[data-testid="note"]').map((note) => note.dataset.state),
};
await click('[data-act="push"]');
await until("the drained queue", () => {
  const counts = q('[data-testid="sync-counts"]');
  return counts && counts.dataset.queued === "0" && counts.dataset.pushed === "2" ? counts : null;
}, 30000);
return {
  before,
  after: {
    counts: data('[data-testid="sync-counts"]'),
    notes: all('[data-testid="note"]').map((note) => note.dataset.state),
  },
};
`;

/**
 * The newer-schema launch. The PRELUDE's `__pq.ready` waits for a rendered
 * Today, which a refused database never shows, so this script is its own
 * minimal one: it waits for the window's state card — which carries the typed
 * code in `data-code` — calls the service once, and reports the card's code,
 * its wording in either language and whether an unmapped-code span appeared
 * (a code with wording must not render one).
 */
const NEWER_SCHEMA_SCRIPT = `
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
const deadline = Date.now() + 15000;
while (!document.querySelector('[data-testid="state"][data-code]') && Date.now() < deadline) await sleep(100);
const card = document.querySelector('[data-testid="state"][data-code]');
const text = card ? card.innerText.replace(/\\s+/g, " ").trim() : "";
let call = null;
try {
  await window.mygo.call("Study.Today");
  call = { ok: true };
} catch (error) {
  call = { ok: false, message: String(error && error.message) };
}
return {
  call,
  code: card ? card.getAttribute("data-code") : "",
  unmapped: document.querySelectorAll("[data-unmapped-code]").length,
  title: ["数据来自更新的 Pass Quest", "This data is from a newer Pass Quest"].some((needle) => text.includes(needle)),
  next: ["安装写入这些数据的更新版本", "Install the newer Pass Quest that wrote this data"].some((needle) => text.includes(needle)),
  text: text.slice(0, 200),
};
`;

/** The typed studio error a failed call rejected with: the CallError carries its JSON in the message. */
function rejectionCode(value: unknown): string {
  const message = stringField(field(value, "call"), "message") ?? "";
  try {
    return stringField(JSON.parse(message), "code") ?? `unparsed: ${shorten(message)}`;
  } catch {
    return `unparsed: ${shorten(message)}`;
  }
}

/**
 * Runs the upgrade case over one installed app. Returns the line the gate's
 * pass detail carries; failures are recorded on `check` as they are found.
 */
export async function upgradeCase(inputs: UpgradeInputs): Promise<string> {
  const { ctx, check, temp, site } = inputs;
  const started = performance.now();
  const root = path.resolve(ctx.root);
  const dir = path.join(temp, "upgrade");
  const oldDir = path.join(dir, "old");
  mkdirSync(oldDir, { recursive: true });
  const oldBin = path.join(oldDir, executable("passquest"));
  const worktree = path.join(oldDir, "worktree");
  const data = site("upgrade");
  const database = data.run.database;
  const stateFile = path.join(dir, "anki-state.json");
  let added = false;
  try {
    // The base commit: present in the developer checkout and on a CI run cut
    // from it, fetched by object id when the depth-2 checkout hides it.
    if (!(await holdsCommit(root, PREVIOUS_RELEASE_COMMIT))) {
      const fetched = await run(executable("git"), ["-C", root, "fetch", "--depth=1", "origin", PREVIOUS_RELEASE_COMMIT]);
      if (fetched.code !== 0 || !(await holdsCommit(root, PREVIOUS_RELEASE_COMMIT))) {
        check(false, `upgrade: ${PREVIOUS_RELEASE_COMMIT} is not in this checkout and fetching it failed: ${gitOutput(fetched)}`);
        return "upgrade: the previous release could not be fetched";
      }
    }
    const worktreeAdd = await run(executable("git"), ["-C", root, "worktree", "add", "--detach", worktree, PREVIOUS_RELEASE_COMMIT]);
    if (worktreeAdd.code !== 0) {
      check(false, `upgrade: git worktree add failed: ${gitOutput(worktreeAdd)}`);
      return "upgrade: the previous release's worktree could not be created";
    }
    added = true;
    const built = await run(executable("go"), ["build", "-o", oldBin, "./cmd/passquest"], { cwd: worktree });
    if (built.code !== 0 || !existsSync(oldBin)) {
      check(false, `upgrade: building the previous release's CLI failed: ${gitOutput(built)}`);
      return "upgrade: the previous release's CLI did not build";
    }

    const old: OldCall = (args, stdin) => run(oldBin, [...args, "--db", database, "--json"], { cwd: root, stdin });
    const step = async (args: string[]): Promise<unknown | null> => {
      const result = await old(args);
      if (!check(result.code === 0, `upgrade: the old CLI's \`passquest ${args.join(" ")}\` exit = ${result.code}: ${tail(result.stderr, 4)}`)) {
        return null;
      }
      try {
        return JSON.parse(result.stdout);
      } catch {
        return {};
      }
    };

    // Attempts: the sample practice is independent, the tactic walkthrough
    // assisted. The third attempt answers A, the wrong option, because a
    // correct attempt is refused as a note signal (`low_value`) and it is the
    // queued note's draft.
    if ((await step(["practice", "--answer", "B"])) === null) return "upgrade: the old data could not be created";
    if ((await step(["tactic", "practice", "not-except-polarity", "--stage", "guided", "--answer", "B"])) === null) {
      return "upgrade: the old data could not be created";
    }
    if ((await step(["practice", "--answer", "A"])) === null) return "upgrade: the old data could not be created";

    // A small reviewed source for the assessment runs: 21 single-question
    // units, the fewest the assessment floor (20 unseen items, Q-0009-03) and
    // a second run after the first spends a unit leave room for. One unit per
    // page, so the first run's selection spends exactly one item.
    const seeded = await seedReviewedSource(
      async (args) => {
        const result = await old(args);
        let doc: unknown = {};
        try {
          doc = JSON.parse(result.stdout);
        } catch {
          doc = {};
        }
        return { code: result.code, stderr: result.stderr, doc };
      },
      check,
      path.join(dir, "source"),
      21,
      1,
    );
    if (seeded === null) return "upgrade: the old data's assessment source could not be seeded";

    // A finished run: one item, answered. Extra answer lines are never read.
    const first = startAssessment(oldBin, root, database, 1);
    first.stdin.write("B\n".repeat(21));
    first.stdin.end();
    const firstResult = await first.result;
    if (!check(firstResult.code === 0, `upgrade: the old CLI's finished assessment run exit = ${firstResult.code}: ${tail(firstResult.stderr, 4)}`)) {
      return "upgrade: the old data could not be created";
    }
    const firstReport = await assessmentReport(old, "1");
    check(numberField(field(firstReport, "counts"), "presented") === 1, "upgrade: the finished assessment run presented its one item");

    // An active run: two items, the first answered, the second presented, then
    // the CLI's stdin ends — `run_interrupted`, the documented way a run stays
    // active (the same state a killed process leaves).
    const second = startAssessment(oldBin, root, database, 2);
    second.stdin.write("B\n");
    second.stdin.end();
    const secondResult = await second.result;
    if (!check(secondResult.code === 4, `upgrade: the old CLI's interrupted assessment run exit = ${secondResult.code}, want 4 (run_interrupted): ${tail(secondResult.stderr, 4)}`)) {
      return "upgrade: the old data could not be created";
    }
    const secondReport = await assessmentReport(old, "2");
    check(
      numberField(field(secondReport, "counts"), "presented") === 2,
      `upgrade: the interrupted assessment run presented ${String(numberField(field(secondReport, "counts"), "presented"))} item(s), want 2`,
    );

    // Notes: the pushed note first, then the queued one while the stub is stopped.
    let pushedDraftID = 0;
    const stub1 = startAnkiStub({ stateFile });
    try {
      const proposedA = await step(["note", "propose", "--attempt", "2"]);
      const draftA = numberField(field(proposedA, "draft"), "id");
      if (draftA === undefined) {
        check(false, "upgrade: the old CLI's note propose returned no draft id");
        return "upgrade: the old data could not be created";
      }
      pushedDraftID = draftA;
      if ((await step(["note", "accept", "--draft", String(draftA)])) === null) return "upgrade: the old data could not be created";
      const pushed = await old(["note", "push", "--endpoint", stub1.endpoint]);
      if (!check(pushed.code === 0, `upgrade: the old CLI's note push exit = ${pushed.code}: ${tail(pushed.stderr, 4)}`)) {
        return "upgrade: the old data could not be created";
      }
      check(stub1.notes.size === 1, `upgrade: the old CLI's push put ${stub1.notes.size} note(s) in the stub, want 1`);
    } finally {
      stub1.stop();
    }

    const proposedB = await step(["note", "propose", "--attempt", "3"]);
    const draftB = numberField(field(proposedB, "draft"), "id");
    if (draftB === undefined) {
      check(false, "upgrade: the old CLI's second note propose returned no draft id");
      return "upgrade: the old data could not be created";
    }
    if ((await step(["note", "accept", "--draft", String(draftB)])) === null) return "upgrade: the old data could not be created";
    const queued = await old(["note", "push", "--endpoint", UNREACHABLE_ENDPOINT]);
    let queuedDoc: unknown = {};
    try {
      queuedDoc = JSON.parse(queued.stdout);
    } catch {
      queuedDoc = {};
    }
    check(
      queued.code === 0 && stringField(queuedDoc, "state") === "queued",
      `upgrade: the old CLI's push with the stub stopped exit = ${queued.code}, state = ${String(stringField(queuedDoc, "state"))}`,
    );
    // The queued note's draft closes, so the pushed note's open draft — the
    // one the revision accepts against, which `note accept --draft` needs
    // open — is the old data's only open draft.
    if ((await step(["note", "skip", "--draft", String(draftB)])) === null) return "upgrade: the old data could not be created";

    // The old data is what the case intends, read straight from the file.
    // Opening read-write once and closing recovers a journal an interrupted
    // command may have left, so the snapshot below is a consistent state
    // rather than one SQLite recovers during the comparison.
    new Database(database).close();
    const facts = readOldData(database);
    check(facts.independent >= 2, `upgrade: the old data holds ${facts.independent} independent attempt(s), want 2`);
    check(facts.assisted >= 1, `upgrade: the old data holds ${facts.assisted} assisted attempt(s), want 1`);
    check(facts.finishedRuns === 1, `upgrade: the old data holds ${facts.finishedRuns} finished assessment run(s), want 1`);
    check(facts.activeRuns === 1, `upgrade: the old data holds ${facts.activeRuns} active assessment run(s), want 1`);
    check(facts.pushedNotes === 1 && facts.queuedNotes === 1, `upgrade: the old data holds ${facts.pushedNotes} pushed and ${facts.queuedNotes} queued note(s), want 1 and 1`);
    check(facts.openDrafts === 1, `upgrade: the old data holds ${facts.openDrafts} open note draft(s), want the pushed note's one`);
    const notesBefore = readNotes(database);
    const pushedNote = notesBefore.find((note) => note.ankiState === "pushed");
    const queuedNote = notesBefore.find((note) => note.ankiState === "queued");
    if (pushedNote === undefined || pushedNote.ankiNoteId <= 0) {
      check(false, "upgrade: the pushed note carries no Anki note id");
      return "upgrade: the old data's pushed note carries no id";
    }
    const firstNoteID = pushedNote.ankiNoteId;

    // The upgrade: a snapshot of the old build's file, the installed app's own
    // launch on it, and the field-by-field comparison. The app is closed again
    // before the upgraded file is read.
    const snapshot = path.join(dir, "before.sqlite");
    copyDatabaseFamily(database, snapshot);
    const opened = await launch(data.run, { label: "open", script: OPEN_SCRIPT });
    if (opened === null) return "upgrade: the installed app could not open the old data root";
    check(
      stringField(opened.value, "error") === undefined,
      `upgrade: the installed app's Today failed on the old data: ${String(stringField(opened.value, "error"))}`,
    );
    check(field(opened.value, "generated") === true, "upgrade: the installed app's Today carries no generated_at");
    check(
      numberField(opened.value, "activeRun") === facts.activeRunId,
      `upgrade: the installed app's Today names active run ${String(numberField(opened.value, "activeRun"))}, the old data holds ${facts.activeRunId}`,
    );
    const upgraded = path.join(dir, "after.sqlite");
    copyDatabaseFamily(database, upgraded);
    // The open itself must not touch a row — an insert or a delete fails
    // here, before the field-by-field comparison and before any further
    // action (the queue flush, the revision). New tables may hold anything.
    const counts = rowCountProblems(snapshot, upgraded);
    check(counts.length === 0, `upgrade: opening the old database changed ${counts.length} old table's row count(s): ${listProblems(counts)}`);
    const diff = compareDatabases(snapshot, upgraded);
    check(
      diff.problems.length === 0,
      `upgrade: ${diff.problems.length} difference(s) after the upgrade: ${listProblems(diff.problems)}`,
    );
    const baseVersion = schemaVersion(snapshot);
    const openVersion = schemaVersion(upgraded);
    check(
      baseVersion > 0 && openVersion > 0,
      `upgrade: a database's user_version did not read (snapshot ${baseVersion}, upgraded ${openVersion}), want the store's written version`,
    );
    const schemaLine =
      baseVersion === openVersion
        ? `schema ${baseVersion} -> ${openVersion}, unchanged (a reopen, no migration step)`
        : `schema ${baseVersion} -> ${openVersion} (a migration step ran)`;
    let line = `${diff.tables.length} tables, ${diff.rows} rows, ${diff.fields} fields compared, ${diff.problems.length} difference(s); ${schemaLine}`;

    // The queued note's drain through the installed app's notes room (and,
    // with it, the old active run abandoned through the window), then the
    // revision through the installed app's own binary, against a stub
    // restarted over the same state file — the same deck the old build's
    // push wrote.
    const stub2 = startAnkiStub({ stateFile });
    try {
      const flushed = await launch(data.run, {
        label: "flush",
        env: { PASSQUEST_ANKI_ENDPOINT: stub2.endpoint },
        script: pageScript(FLUSH_SCRIPT),
      });
      if (flushed === null) return `upgrade: ${line}; the queue-drain launch failed`;
      const before = field(flushed.value, "before");
      check(
        stringField(field(before, "counts"), "queued") === "1" && stringField(field(before, "counts"), "pushed") === "1",
        `upgrade: the notes room opens with ${JSON.stringify(field(before, "counts"))}, want the old push counted (pushed 1) and the queued note waiting (queued 1)`,
      );
      const states = (value: unknown) => (Array.isArray(value) ? value.map(String).sort().join(",") : String(value));
      check(
        states(field(before, "notes")) === "pushed,queued",
        `upgrade: the notes list holds ${states(field(before, "notes"))}, want the pushed and the queued note`,
      );
      const after = field(flushed.value, "after");
      check(
        stringField(field(after, "counts"), "queued") === "0" && stringField(field(after, "counts"), "pushed") === "2",
        `upgrade: the window counts after the flush are ${JSON.stringify(field(after, "counts"))}, want pushed 2, queued 0`,
      );
      const adds = stub2.actions.filter((entry) => entry.action === "addNote");
      const flushedUpdates = stub2.actions.filter((entry) => entry.action === "updateNoteFields");
      check(
        adds.length === 1 && flushedUpdates.length === 0,
        `upgrade: the window's push sent ${adds.length} addNote and ${flushedUpdates.length} update action(s), want the queued note added and nothing updated`,
      );
      const newIDs = [...stub2.notes.keys()].filter((id) => id !== firstNoteID);
      check(
        stub2.notes.size === 2 && newIDs.length === 1,
        `upgrade: the stub holds ${stub2.notes.size} note(s) after the flush, want the old note ${firstNoteID} and the queued note's new one`,
      );
      if (queuedNote !== undefined && newIDs.length === 1) {
        const probe = queuedNote.front.split(/\s+/).find((word) => /^[A-Za-z]{4,}$/.test(word)) ?? queuedNote.front.slice(0, 8);
        check(
          (stub2.notes.get(newIDs[0]!)?.Front ?? "").includes(probe),
          `upgrade: the flushed queued note in the stub does not carry its front (${shorten(probe)})`,
        );
      }
      const settledRun = readOldData(database);
      check(
        settledRun.activeRuns === 0 && settledRun.abandonedRuns === 1,
        `upgrade: after the flush the app left ${settledRun.activeRuns} active run(s) and ${settledRun.abandonedRuns} abandoned, want the old active run abandoned by the window`,
      );

      // The revision: the installed app's own binary, which is the CLI when
      // given arguments (cmd/passquest-desktop/main.go), against the upgraded
      // database. The window cannot revise the pushed note (its draft is
      // accepted, and OpenNoteDraft refuses an accepted draft), so the accept
      // and the push go through that binary, launched with the app's own
      // environment. The binary passes its own resources directory into the
      // CLI's composition root, and this launch's PATH carries no node, so the
      // packaged copy's Node runtime and Anki host are the ones that run.
      const ankiHost = inputs.app.hosts.find((host) => path.basename(host) === "anki-host.mjs");
      if (ankiHost === undefined) {
        check(false, "upgrade: the installed app carries no agent/anki-host.mjs; the revision cannot run through it");
        return `upgrade: ${line}; the installed app carries no Anki host`;
      }
      const revisionEnv = {
        ...textEnv(data.run.shared.launchEnv ?? {}),
      };
      const revisionCwd = data.run.shared.cwd ?? root;
      const revision = await run(
        inputs.app.binary,
        ["note", "accept", "--draft", String(pushedDraftID), "--front", `${pushedNote.front} upgrade-edit`, "--db", database, "--json"],
        { cwd: revisionCwd, env: revisionEnv },
      );
      if (!check(revision.code === 0, `upgrade: the revision accept exit = ${revision.code}: ${tail(revision.stderr, 4)}`)) {
        return `upgrade: ${line}; the revision was refused`;
      }
      const pushedRevision = await run(inputs.app.binary, ["note", "push", "--endpoint", stub2.endpoint, "--db", database, "--json"], {
        cwd: revisionCwd,
        env: revisionEnv,
      });
      if (!check(pushedRevision.code === 0, `upgrade: the revision push exit = ${pushedRevision.code}: ${tail(pushedRevision.stderr, 4)}`)) {
        return `upgrade: ${line}; the revision could not be pushed`;
      }
      const updates = stub2.actions.filter((entry) => entry.action === "updateNoteFields");
      const updatedID = updates.length === 1 ? numberField(field(updates[0]!.params, "note"), "id") : undefined;
      check(
        updates.length === 1 && updatedID === firstNoteID,
        `upgrade: the revision sent ${updates.length} update(s) for note id ${String(updatedID)}, want one for the old push's note ${firstNoteID}`,
      );
      check(
        stub2.actions.filter((entry) => entry.action === "addNote").length === adds.length,
        "upgrade: the revision added a second card for the pushed note instead of updating it",
      );
      check(
        (stub2.notes.get(firstNoteID)?.Front ?? "").includes("upgrade-edit"),
        "upgrade: the revised note in the stub does not carry the revision's edit",
      );
      line = `${line}; note ${firstNoteID} revised in place, queued note flushed through the window`;
    } finally {
      stub2.stop();
    }

    // The newer schema: a copy with its version above the supported one (the
    // P4 Go test's own shape — a writer that quit, no side files), launched by
    // the installed app; the refusal must leave every byte alone.
    const future = site("upgrade-future");
    mkdirSync(future.dataRoot, { recursive: true });
    const futurePath = future.run.database;
    copyDatabaseFamily(database, futurePath);
    let futureVersion = 0;
    {
      const db = new Database(futurePath);
      const version = db.query<{ user_version: number }, []>("PRAGMA user_version").get()?.user_version ?? 0;
      futureVersion = version + 1;
      db.run(`PRAGMA user_version = ${futureVersion}`);
      db.close();
    }
    const digestsBefore = familyDigests(futurePath);
    const refused = await launch(future.run, { label: "future", script: NEWER_SCHEMA_SCRIPT });
    if (refused === null) return `upgrade: ${line}; the newer-schema launch failed`;
    const digestsAfter = familyDigests(futurePath);
    check(
      JSON.stringify(digestsBefore) === JSON.stringify(digestsAfter),
      `upgrade: opening a newer-schema database changed it: ${JSON.stringify(digestsBefore)} -> ${JSON.stringify(digestsAfter)}`,
    );
    check(
      rejectionCode(refused.value) === "database_newer",
      `upgrade: a newer-schema database failed with ${rejectionCode(refused.value)}, want database_newer`,
    );
    check(
      stringField(refused.value, "code") === "database_newer",
      `upgrade: the window's state card carries ${String(stringField(refused.value, "code"))}, want database_newer`,
    );
    check(
      numberField(refused.value, "unmapped") === 0,
      `upgrade: the refusal rendered ${String(numberField(refused.value, "unmapped"))} unmapped-code span(s): ${String(stringField(refused.value, "text"))}`,
    );
    // Both halves of the wording: the title alone would pass a card whose
    // next step fell back to the generic retry sentence.
    check(
      field(refused.value, "title") === true,
      `upgrade: the refusal card carries no mapped title for database_newer: ${String(stringField(refused.value, "text"))}`,
    );
    check(
      field(refused.value, "next") === true,
      `upgrade: the refusal card carries no mapped next step for database_newer: ${String(stringField(refused.value, "text"))}`,
    );
    line = `${line}; schema ${futureVersion} refused, ${Object.keys(digestsBefore).length} file(s) unchanged; refusal card: ${JSON.stringify(
      stringField(refused.value, "text"),
    )}`;
    return `upgrade: ${line} in ${((performance.now() - started) / 1000).toFixed(1)}s`;
  } finally {
    if (added) {
      // Every exit path removes this gate's own worktree. `--force` removes a
      // tree whose build left it dirty; a tree git refuses (a handle that a
      // killed tool still held) is deleted, then the removal is retried so
      // git's own bookkeeping goes with it. Never `prune`: it would sweep
      // every stale worktree entry of the repository, not this gate's.
      const removed = await run(executable("git"), ["-C", root, "worktree", "remove", "--force", worktree]);
      if (removed.code !== 0) {
        const cleared = await removeTree(worktree);
        const again = await run(executable("git"), ["-C", root, "worktree", "remove", "--force", worktree]);
        check(
          cleared && again.code === 0,
          `cleanup: the gate's worktree ${worktree} could not be removed (tree ${cleared ? "deleted" : "survived"}; git ${
            again.code === 0 ? "removed the entry" : `refused again: ${gitOutput(again)}`
          })`,
        );
      }
    }
  }
}
