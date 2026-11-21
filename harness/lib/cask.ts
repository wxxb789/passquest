/**
 * The Homebrew cask template and the invariants the install-guide gate pins on
 * the file it renders (goal 0017 P3). The template under `.github/` carries
 * `@VERSION@`/`@SHA256@`; the release job substitutes the release's own values
 * and writes the tap's `Casks/pass-quest.rb`, so the render here is the gate's
 * proof that this machine writes that file without a leftover placeholder and
 * with the stanzas a current Homebrew accepts.
 *
 * `@UPPERCASE@` is the placeholder form on purpose. Homebrew's install-step
 * runner expands its own `{{...}}` tokens at install time — `{{appdir}}` and
 * friends — and leaves an unknown `{{...}}` verbatim (Cask-Cookbook,
 * "Interpolation in steps blocks"), so a `{{VERSION}}` placeholder would collide
 * with the tokens the cask must keep and a leftover one could reach the tap
 * silently. `LEFTOVER` rejects every `@…@` token whose name starts with a letter,
 * in either case, so a renamed or lower-cased placeholder fails rather than
 * shipping as literal text.
 *
 * This module is pure text: it reads no cask, runs no Homebrew (a gate makes no
 * external network call and `brew` exists on no Windows runner) and reports
 * problems as strings the gate prefixes.
 */
import { createHash } from "node:crypto";

/** The token the template carries where the release's version goes. */
const VERSION_TOKEN = "@VERSION@";
/** The token the template carries where the release asset's SHA-256 goes. */
const SHA256_TOKEN = "@SHA256@";
/** A placeholder nobody substituted: any `@name@` token whose name starts with
 * a letter, in either case (the template's own tokens are `@UPPERCASE@`).
 * `{{...}}` stays out of this on purpose, because Homebrew's install-step tokens
 * are meant to survive the render. */
const LEFTOVER = /@[A-Za-z][A-Za-z0-9_-]*@/g;

/** The macOS release each codename `depends_on macos:` may mean (mygo.json's `macos.minimumSystemVersion`). */
const MACOS_CODE = [
  { major: 14, name: ":sonoma" },
  { major: 15, name: ":sequoia" },
  { major: 26, name: ":tahoe" },
  { major: 27, name: ":golden_gate" },
] as const;

/**
 * The facts the cask must state. The gate replaces `minimumMajor` and
 * `identifier` with `mygo.json`'s values, and `dataDirectory` is
 * `MACOS_DATA_DIRECTORY` below — the one constant the install guides' own fact
 * imports — so a cask that disagrees with the guides or the manifest fails
 * before any push; the remaining fields are this module's own literals, with
 * `version` and `sha256` the published v0.15.0 release's pair.
 */
export interface CaskFacts {
  /** e.g. "Pass Quest" — the `name` and the `.app`; the install guides must spell it the same. */
  name: string;
  /** The bundle identifier the app registers under; `uninstall quit:` carries it. */
  identifier: string;
  /** The macOS major mygo.json declares as the floor; `depends_on macos:` must not be lower. */
  minimumMajor: number;
  /** `Casks/<token>.rb` in the tap; also the cask's header token and the release asset's stem. */
  token: string;
  /** The per-user data directory the install guides name; `zap trash:` carries the same `MACOS_DATA_DIRECTORY` constant. */
  dataDirectory: string;
  /** `owner/repo`; the homepage and the release URL are checked against it. */
  repository: string;
  /** The release the render comes from; the `version` line must state it. */
  version: string;
  /** That release's `.dmg` digest; the `sha256` line must state it. */
  sha256: string;
}

/**
 * The version and SHA-256 the self-test and the gate's pass proof render with:
 * the published v0.15.0 `pass-quest-0.15.0-arm64.dmg` (44,963,840 bytes) and
 * the SHA-256 of that artifact, computed from the download and identical to
 * the line v0.15.0's own `SHA256SUMS.txt` states (plan fact 6). One pair, so a
 * fixture cannot disagree with the gate's proof about what "the v0.15.0
 * render" is.
 */
export const CASK_VERSION = "0.15.0";
export const CASK_SHA256 = "477938dc1635971e23b2de952e04c684a87eda5c29fece7508d8b42c787b3e66";

/**
 * The second render's pair, used by `placeholderProblems`: values no release
 * carries, so a template that reads them back after this render is proven to
 * substitute its placeholders rather than state hard-coded values.
 */
const SYNTHETIC_VERSION = "9.8.7";
const SYNTHETIC_SHA256 = "ab".repeat(32);

/**
 * The per-user data directory the app keeps on macOS: the install guides name
 * it and `CASK_FACTS.dataDirectory` below is this same constant, so the cask's
 * `zap trash:` cannot point at a directory the guides do not name.
 */
export const MACOS_DATA_DIRECTORY = "~/Library/Application Support/Pass Quest";

export const CASK_FACTS: CaskFacts = {
  name: "Pass Quest",
  identifier: "com.github.wxxb789.passquest",
  minimumMajor: 26,
  token: "pass-quest",
  dataDirectory: MACOS_DATA_DIRECTORY,
  repository: "wxxb789/passquest",
  version: CASK_VERSION,
  sha256: CASK_SHA256,
};

/**
 * Where the release job writes the cask in the tap. Homebrew names a cask after
 * the file it was loaded from — `cask/cask_loader.rb` raises
 * `CaskTokenMismatchError` when the header token and the file basename disagree —
 * so the basename is the token, never a second literal that could drift.
 */
export const CASK_TAP_PATH = `Casks/${CASK_FACTS.token}.rb`;

/**
 * The public tap repository that carries the cask (goal 0017, default 2).
 * `harness/tools/cask-publish.ts` pushes to it over the deploy key and
 * `harness/tools/cask-install.ts` clones it into Homebrew's taps directory, so
 * the one literal is here instead of in two tools that could drift apart.
 */
export const TAP_REPOSITORY = "wxxb789/homebrew-tap";

/** The template with the release's version and SHA-256 in every placeholder. */
export function renderCask(template: string, version: string, sha256: string): string {
  return template.replaceAll(VERSION_TOKEN, version).replaceAll(SHA256_TOKEN, sha256);
}

/** The SHA-256 of a text's UTF-8 bytes, the digest the cask tools print for a render. */
export function sha256Text(text: string): string {
  return createHash("sha256").update(text).digest("hex");
}

/**
 * The version and SHA-256 a rendered cask states, or null when either line is
 * absent or not one quoted string. The install tool reads the tap's own copies
 * with this and re-renders the template with them: equal bytes mean the tap and
 * the template agree, different bytes mean the tap drifted from the tree.
 */
export function statedRelease(rendered: string): { version: string; sha256: string } | null {
  const version = quotedValue(rendered, "version");
  const sha256 = quotedValue(rendered, "sha256");
  if (version === null || sha256 === null) return null;
  return { version, sha256 };
}

/**
 * The problems a template has as a template. A second render with the synthetic
 * pair must read back the values it was given on the `version` and `sha256`
 * lines: a line that ignores the render is hard-coded (or the token is gone),
 * and the release job would keep writing one release even though the pinned
 * golden was re-rendered to match the template.
 */
export function placeholderProblems(template: string): string[] {
  const problems: string[] = [];
  const rendered = renderCask(template, SYNTHETIC_VERSION, SYNTHETIC_SHA256);
  const version = quotedValue(rendered, "version");
  if (version !== SYNTHETIC_VERSION) {
    problems.push(
      `the version line must carry ${VERSION_TOKEN}: rendered with ${SYNTHETIC_VERSION}, it states ${version === null ? "no version line" : JSON.stringify(version)}; a hard-coded version would freeze the tap at one release`,
    );
  }
  const sha256 = quotedValue(rendered, "sha256");
  if (sha256 !== SYNTHETIC_SHA256) {
    problems.push(
      `the sha256 line must carry ${SHA256_TOKEN}: rendered with a synthetic digest, it states ${sha256 === null ? "no sha256 line" : JSON.stringify(sha256)}; a hard-coded digest would freeze the tap at one release`,
    );
  }
  return problems;
}

/** The token on the `cask "<token>" do` header line, or null when that line is absent or not that shape. */
function headerToken(text: string): string | null {
  return /^\s*cask\s+"([^"]+)"\s+do\s*$/m.exec(text)?.[1] ?? null;
}

/** The value of one `key "..."` stanza, or null when the stanza is absent or holds something else. */
function quotedValue(text: string, key: string): string | null {
  const pattern = new RegExp(`^\\s*${key}\\s+(?:"([^"]*)"|'([^']*)')\\s*$`, "m");
  const found = pattern.exec(text);
  if (found === null) return null;
  return (found[1] ?? found[2])!.replace(/\\"/g, '"').replace(/\\'/g, "'");
}

/** The symbol one `depends_on <key>: :value` states, or null when the stanza is absent or not one symbol. */
function dependsOn(text: string, key: string): string | null {
  return new RegExp(`^\\s*depends_on\\s+${key}:\\s+(:[A-Za-z0-9_]+)\\s*$`, "m").exec(text)?.[1] ?? null;
}

/**
 * The quoted value of a `quit:`-style argument, or null when that argument is
 * absent or holds a list. The stanza name is the anchor, so a nested `do`/`end`
 * block needs no parser: `uninstall` carries exactly one such argument.
 */
function argumentValue(text: string, stanza: string, key: string): string | null {
  const found = new RegExp(`^\\s*${stanza}\\s+${key}:\\s*(?:"([^"]*)"|'([^']*)')\\s*$`, "m").exec(text);
  if (found === null) return null;
  return (found[1] ?? found[2])!.replace(/\\"/g, '"').replace(/\\'/g, "'");
}

/** The body of the `livecheck do … end` block, or null when there is no such block. */
function livecheckBlock(text: string): string | null {
  const lines = text.split("\n");
  const start = lines.findIndex((line) => /^\s*livecheck\s+do\s*$/.test(line));
  if (start < 0) return null;
  const body: string[] = [];
  for (let index = start + 1; index < lines.length; index++) {
    if (/^\s*end\s*$/.test(lines[index])) break;
    body.push(lines[index]);
  }
  return body.join("\n");
}

/**
 * The quoted string literals in a text, in order, with their quotes stripped
 * and their escapes resolved.
 */
function quotedStrings(text: string): string[] {
  return [...text.matchAll(/"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'/g)].map((match) =>
    match[0].slice(1, -1).replace(/\\"/g, '"').replace(/\\'/g, "'"),
  );
}

/**
 * The body of an array: the text from just after its `[` to the `]` that closes
 * it, quotes respected, or to the end when nothing closes it.
 */
function arrayBody(text: string, from: number): string {
  let quote = "";
  for (let index = from; index < text.length; index++) {
    const char = text[index];
    if (quote !== "") {
      if (char === "\\") index++;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'") quote = char;
    else if (char === "]") return text.slice(from, index);
  }
  return text.slice(from);
}

/**
 * The `zap trash:` entries a cask states. Both forms Homebrew writes parse: the
 * single quoted string, and the array `brew generate-zap` emits
 * (`dev-cmd/generate-zap.rb`, one string per path) — on one line or several, so
 * quoted text after the closing `]` is never read as a path. Every entry is one
 * literal path — `cask/artifact/abstract_uninstall.rb` resolves each directive
 * value as a whole — so a comma never separates paths and is never split on.
 *
 * Exported for `harness/tools/cask-install.ts`: the macOS install job reads the
 * list back out of the cask file Homebrew installed from and requires every path
 * to be gone after `--zap`, so the assertion follows the cask instead of a
 * second, hand-kept copy of the paths that could drift from it.
 */
export function zapTrashPaths(rendered: string): string[] {
  const lines = rendered.split("\n");
  const start = lines.findIndex((line) => /^\s*zap\s+trash:/.test(line));
  if (start < 0) return [];
  const head = lines[start].replace(/^\s*zap\s+trash:\s*/, "");
  if (!head.startsWith("[")) return quotedStrings(head);
  const body = [head.slice(1), ...lines.slice(start + 1)].join("\n");
  return quotedStrings(arrayBody(body, 0));
}

/** The quoted values of the first `args:` array, or null when the cask states no such array. */
function argsArray(rendered: string): string[] | null {
  const start = /^\s*args:\s*\[/m.exec(rendered);
  if (start === null) return null;
  return quotedStrings(arrayBody(rendered, start.index + start[0].length));
}

/**
 * The problems a rendered cask has against the facts. Reads the text as a cask:
 * every required stanza present with the right value, `@…@` placeholders gone,
 * Homebrew's own `{{...}}` tokens untouched. The checks are per line — the
 * stanza names are the contract, and a cask is short enough that a line means
 * one stanza.
 */
export function caskProblems(rendered: string, facts: CaskFacts): string[] {
  const problems: string[] = [];
  const release = `https://github.com/${facts.repository}/releases/download/v#{version}/${facts.token}-#{version}-arm64.dmg`;

  const leftovers = rendered.match(LEFTOVER) ?? [];
  if (leftovers.length > 0) {
    problems.push(`the rendered cask still carries ${leftovers.join(", ")}; a placeholder nobody substituted would reach the tap verbatim`);
  }

  const token = headerToken(rendered);
  if (token !== facts.token) {
    problems.push(
      `the header must be cask "${facts.token}" do — Homebrew refuses to load a cask whose header token and file name disagree (cask/cask_loader.rb); it is ${token === null ? "absent" : `cask "${token}"`}`,
    );
  }

  const version = quotedValue(rendered, "version");
  if (version !== facts.version) {
    problems.push(`version must be ${facts.version}, the release this cask was rendered from; it is ${version === null ? "absent" : JSON.stringify(version)}`);
  }

  const sha256 = quotedValue(rendered, "sha256");
  if (sha256 === null || !/^[0-9a-f]{64}$/.test(sha256)) {
    problems.push(`sha256 must be one 64-character lowercase hex string; the release's own SHA256SUMS.txt is the source`);
  } else if (sha256 !== facts.sha256) {
    problems.push(`sha256 must be the release asset's own digest ${facts.sha256}; it is ${sha256}`);
  }

  if (!rendered.includes(release)) {
    problems.push(`url must be ${release} (v + the cask version, the ${facts.token}-<version>-arm64.dmg release asset)`);
  }

  if (!new RegExp(`^\\s*app\\s+"${facts.name}\\.app"\\s*$`, "m").test(rendered)) {
    problems.push(`app must be "${facts.name}.app"`);
  }

  if (!new RegExp(`^\\s*homepage\\s+"https://github\\.com/${facts.repository}"\\s*$`, "m").test(rendered)) {
    problems.push(`homepage must be https://github.com/${facts.repository}`);
  }

  const desc = quotedValue(rendered, "desc");
  if (desc === null) problems.push(`no desc stanza; the cask must state what the app is`);
  else if (desc.length > 80) problems.push(`desc is ${desc.length} characters, above the 80-character limit ("${desc}")`);
  else if (desc.trimStart().startsWith(facts.name)) problems.push(`desc starts with the app's own name, which the cask title already carries: "${desc}"`);

  const macos = dependsOn(rendered, "macos");
  if (macos === null) {
    problems.push(`depends_on macos: must state the minimum as a code symbol; it is absent or not one symbol`);
  } else {
    const code = MACOS_CODE.find((entry) => entry.major === facts.minimumMajor);
    if (code === undefined) problems.push(`harness/lib/cask.ts knows no macOS codename for major ${facts.minimumMajor}`);
    else if (macos !== code.name) {
      problems.push(`depends_on macos: ${macos} against ${code.name} (macOS ${code.major}, the floor mygo.json declares)`);
    }
  }

  if (dependsOn(rendered, "arch") !== ":arm64") problems.push(`depends_on arch: must be :arm64; the release carries one arm64 dmg`);

  // The livecheck contract, from Homebrew's own sources: a cask's livecheck url
  // symbol is a string or :url/:homepage (Cask-Cookbook, "livecheck"), and
  // :stable resolves only for a Formula (livecheck/packages.rb). GithubLatest
  // has priority 0 (livecheck/strategy/github_latest.rb), and Homebrew drops
  // priority-0 strategies unless the block names them (livecheck/strategy.rb),
  // so `url :url` alone selects nothing and the strategy must be named.
  const livecheck = livecheckBlock(rendered);
  if (livecheck === null || !/^\s*url\s+:url\s*$/m.test(livecheck) || !/^\s*strategy\s+:github_latest\s*$/m.test(livecheck)) {
    problems.push(`livecheck must be a block holding url :url and strategy :github_latest; url :stable is Formula-only and GithubLatest must be named`);
  }

  if (argumentValue(rendered, "uninstall", "quit") !== facts.identifier) {
    problems.push(`uninstall quit: must be ${facts.identifier}, the identifier the app registers under`);
  }

  const zap = zapTrashPaths(rendered);
  if (!zap.includes(facts.dataDirectory)) {
    problems.push(`zap trash: must carry the data directory the install guides name: "${facts.dataDirectory}"`);
  }
  for (const entry of zap) {
    if (entry.includes(",")) {
      problems.push(`zap trash: "${entry}" holds several paths in one string; Homebrew resolves each directive value as one literal path, so the paths must be separate strings`);
    }
  }

  if (!/^\s*postflight_steps\s+do\s*$/m.test(rendered)) {
    problems.push(`postflight_steps must clear com.apple.quarantine; Homebrew quarantines the download, so without it the first launch meets Gatekeeper`);
  } else {
    if (!rendered.includes(`"com.apple.quarantine"`)) problems.push(`postflight_steps must name com.apple.quarantine`);
    // The step's own line, never a comment about it: the header explains both
    // `{{appdir}}` and `must_succeed: false`, so an unanchored match would
    // accept a cask whose step lost the token or would abort on exit 1. The
    // args are compared element by element: `-r` without `-d` never deletes
    // the attribute, and `-d` is what clears the flag.
    const args = argsArray(rendered);
    if (args === null) {
      problems.push(`postflight_steps must clear the flag on the installed app through {{appdir}} in its args`);
    } else {
      const quarantine = "com.apple.quarantine";
      const appPath = `{{appdir}}/${facts.name}.app`;
      const accepted = [
        ["-rd", quarantine, appPath],
        ["-dr", quarantine, appPath],
      ];
      if (!accepted.some((expected) => JSON.stringify(expected) === JSON.stringify(args))) {
        problems.push(
          `the xattr run's args must be ["-rd", "${quarantine}", "${appPath}"] (or -dr: the same two flags); they are ${JSON.stringify(args)}, so the quarantine flag can survive the install`,
        );
      }
    }
    if (!/^\s*must_succeed:\s*false\s*$/m.test(rendered)) {
      problems.push(`run must pass must_succeed: false: xattr exits non-zero when the attribute is already absent, the state this step wants`);
    }
  }

  return problems;
}
