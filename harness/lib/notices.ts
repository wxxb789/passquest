/**
 * The third-party notices a staged package carries (goal 0016 P4): for every
 * package the staged bundles inline, every licence text in that package's own
 * tree — the package's own licence files, the licences of the code it vendors,
 * which the bundle carries the same way (`openai` inlines
 * `openai/internal/qs/stringify.mjs`, whose BSD-3-Clause text lives at
 * `openai/src/internal/qs/LICENSE.md`), and the files it keeps in a directory
 * of licence texts (`THIRD_PARTY_NOTICES/`, `LICENSES/`) — and, where the tree
 * carries no licence text of its own, the SPDX MIT text with the package's
 * author line (goal 0016, default 5). Staging writes the result to
 * `agent/THIRD-PARTY-NOTICES.txt`, so the licences of the code the bundles
 * carry travel with the code.
 *
 * A regular file travels when its name carries a licence word — `LICENSE`,
 * `LICENCE`, `NOTICE`, `COPYING`, and the compound names packages use, like
 * `THIRD-PARTY-NOTICES.txt`, `MIT-LICENSE` or `ThirdPartyNotices.txt` — or
 * when it sits under a directory whose name carries a directory word
 * (`LICENSES/`, `THIRD_PARTY_NOTICES/`, `legal-notices/`), at whatever depth.
 * Source code is not a licence text however it is named: a name ending in a
 * code extension (`.js`, `.mjs`, `.cjs`, `.ts`, `.mts`, `.cts`, `.map`,
 * `.json`) does not travel, so `license-checker.js` stays out; `.txt`,
 * `.md` and a name with no extension always travel.
 *
 * The search is over the package's own tree only: a nested `node_modules`
 * holds other packages, each named by the metafile itself (package.ts,
 * bundlePackages) and noticed for itself. Over-inclusion is the accepted
 * side: a licence text that names code no bundle inlined travels anyway;
 * omission is not.
 *
 * The assembly is a pure function of the package list and a reader
 * (assembleNotices): the harness self-test drives it with fixture packages. A
 * package whose terms the assembly cannot state — no licence text of its own,
 * and not a declared MIT with an author to attribute it to — fails the run
 * rather than shipping without a notice.
 *
 * The assembled text names no machine: the headers carry package names,
 * versions and package-relative file names only, and every other byte comes
 * from a package file or the SPDX fallback. assertLeanAgentTree leans on
 * that: the notices file is exempt from the machine-path *shapes* (`/home/`,
 * `/tmp/`), which licence prose may legitimately contain, and from the
 * temp-directory literal, which is itself machine-spelled ("/tmp" on Linux),
 * so one licence text passes on every OS; the one literal it is still refused
 * for is the checkout root, which this module never writes (the same rule
 * everywhere, harness/lib/package.ts, checkoutRootIn).
 */
import path from "node:path";

/** One package a staged bundle inlined: the name and version its own package.json declares, and the directory it was read from. */
export interface BundlePackage {
  name: string;
  version: string;
  root: string;
}

/** How the assembly reads a package: a file's text, and a directory's entry names. */
export interface NoticeSource {
  /**
   * The file's text, or undefined where the path is not there (ENOENT,
   * ENOTDIR) or is not a regular file. Every other failure throws, so a
   * licence the source could not read never reads as one the package does not
   * carry.
   */
  read(file: string): string | undefined;
  /** The names in dir, sorted; empty where dir is not there. Every other failure throws. */
  list(dir: string): string[];
}

/** One text the assembled file carries, and the header that names it. */
export interface NoticeText {
  /** The section header the file carries, e.g. `openai@7.19.0 / LICENSE`. */
  label: string;
  /** The text, exactly as the file carries it. */
  text: string;
}

/** What a staging's bundles require of the notices file: every package they inlined, and every text they require the file to carry. */
export interface StagedNotices {
  /** The packages the staged bundles inlined, sorted by name; the file names each one. */
  packages: readonly { name: string; version: string }[];
  /** Every text the file must carry, in the order it carries them. */
  texts: readonly NoticeText[];
}

/** The assembled file: what it must carry, and its whole text. */
interface NoticeAssembly extends StagedNotices {
  text: string;
}

/** The licence words a file name carries anywhere, in npm's spellings: `LICENSE`, `LICENCE`, `NOTICE`, `COPYING`, `THIRD-PARTY-NOTICES.txt`, `MIT-LICENSE`. */
const LICENCE_WORD = /licen[cs]e|notice|copying/i;

/** The licence words a directory name carries anywhere: every text file under `LICENSES/`, `THIRD_PARTY_NOTICES/` or `legal-notices/` travels. */
const LICENCE_DIR_WORD = /licen[cs]es|notices/i;

/** The extensions that carry code, not a licence text: a licence word in the name of one of these (`license-checker.js`, `notices.json`) does not make it a notice. `.txt`, `.md` and no extension are never code. */
const CODE_EXTENSIONS = [".js", ".mjs", ".cjs", ".ts", ".mts", ".cts", ".map", ".json"];

/** The directory a package puts the licence texts of the code it embeds in: those are the notice of that code, not evidence of the package's own terms (the SPDX fallback below leans on that). */
const THIRD_PARTY_DIR = "THIRD_PARTY_NOTICES";

/** The directory a package keeps its installed dependencies in: other packages, with their own notices, and no part of this package's tree. */
const NODE_MODULES = "node_modules";

/** The section rule: 80 characters, so a reader can find a section's start in a plain text viewer. */
const RULE = "=".repeat(80);

/** The file's preamble: who wrote it, from what, and that it is not edited by hand. */
const PREAMBLE = [
  "Third-party notices for the packages the staged agent hosts inline.",
  "Assembled by the harness when a package is staged (harness/lib/notices.ts): every",
  "inlined package's own licence files, the files a package carries under",
  "THIRD_PARTY_NOTICES, and the SPDX MIT text with the package's author line where",
  "a package ships no licence file of its own. Do not edit: staging writes this file.",
];

/**
 * The SPDX MIT text, with `<holder>` standing for the author a package
 * declares (spdx.org/licenses/MIT renders the same text; the placeholder
 * there is `<year> <copyright holders>`, and the year is left out because a
 * staging is reproducible and a build date is not part of a copyright line).
 */
const SPDX_MIT = `MIT License

Copyright (c) <holder>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`;

/** A package's package.json, parsed; read twice is cheaper than a wider BundlePackage. */
interface Manifest {
  name?: unknown;
  version?: unknown;
  license?: unknown;
  author?: unknown;
}

/** The manifest of one inlined package, checked against the name and version the metafile gave. */
function manifestOf(pkg: BundlePackage, source: NoticeSource): Manifest {
  const file = path.join(pkg.root, "package.json");
  const raw = source.read(file);
  if (raw === undefined) throw new Error(`${pkg.name}@${pkg.version}: cannot read ${file}; a bundle inlined a package the tree does not carry`);
  let manifest: Manifest;
  try {
    manifest = JSON.parse(raw) as Manifest;
  } catch (error) {
    throw new Error(`${file} is not JSON: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (manifest.name !== pkg.name || manifest.version !== pkg.version) {
    throw new Error(
      `${file} declares ${JSON.stringify(manifest.name)}@${JSON.stringify(manifest.version)}, want ${pkg.name}@${pkg.version}; the bundle's metafile and the package on disk disagree`,
    );
  }
  return manifest;
}

/** The author a manifest declares: npm's string form, or an object's `name`. */
function authorOf(manifest: Manifest): string {
  if (typeof manifest.author === "string") return manifest.author.trim();
  if (manifest.author !== null && typeof manifest.author === "object" && "name" in manifest.author) {
    const name = manifest.author.name;
    if (typeof name === "string") return name.trim();
  }
  return "";
}

/**
 * The SPDX MIT text for a package that ships no licence file of its own: only
 * a package that declares exactly MIT and names an author to hold the
 * copyright can be stated this way, and the fallback's wording is the
 * harness's, not upstream text (a disclosed limit of goal 0016, default 5).
 */
function mitFallback(pkg: BundlePackage, manifest: Manifest): NoticeText {
  const license = typeof manifest.license === "string" ? manifest.license.trim() : "";
  if (license !== "MIT") {
    throw new Error(
      `${pkg.name}@${pkg.version}: ships no licence file of its own and declares the licence ${JSON.stringify(manifest.license ?? null)}; the notices assembly can state only the declared MIT text, so the run fails instead of shipping without a notice`,
    );
  }
  const author = authorOf(manifest);
  if (author === "") {
    throw new Error(
      `${pkg.name}@${pkg.version}: ships no licence file of its own and names no author, so the SPDX MIT fallback cannot name a copyright holder`,
    );
  }
  // A function replacement, so an author carrying `$&` or `$1` arrives
  // verbatim: a string replacement would read it as a replacement pattern and
  // leave the placeholder in the shipped file.
  return {
    label: `${pkg.name}@${pkg.version} / SPDX MIT (no licence file in the package)`,
    text: SPDX_MIT.replace("<holder>", () => author),
  };
}

/** A text as the assembled file carries it: LF line endings, and exactly one trailing newline. */
function asCarried(text: string): string {
  const normalised = text.replace(/\r\n/g, "\n");
  return normalised.endsWith("\n") ? normalised : `${normalised}\n`;
}

/** One text read through the source, or a failure naming the file the assembly needed. */
function readText(source: NoticeSource, file: string, what: string): string {
  const text = source.read(file);
  if (text === undefined) throw new Error(`${what}: cannot read ${file}`);
  return asCarried(text);
}

/**
 * Every licence text in the package's own tree (goal 0016 P4), as absolute
 * paths in name order: each regular file whose name carries a licence word,
 * and each regular file anywhere under a directory whose name carries one
 * (`THIRD_PARTY_NOTICES/`, `LICENSES/`), at any depth, and never one inside a
 * nested `node_modules` — those belong to the packages installed there, which
 * the metafile names separately (bundlePackages) and the assembly notices for
 * themselves. A name that ends in a code extension is not a licence text,
 * licence word or not; `.txt`, `.md` and a name with no extension always can.
 *
 * The whole tree is walked, not the package root alone: an inlined module's
 * licence may sit under the directory a package keeps its sources in while
 * the module the bundle inlines sits outside it, and a licence omitted for
 * that would ship the code without its terms. The directory rule carries the
 * files of a `THIRD_PARTY_NOTICES/` that keeps its notices in subdirectories
 * (`THIRD_PARTY_NOTICES/lodash/LICENSE`), which a listing of the directory's
 * own entries would read as a file and fail on.
 *
 * `read` is the interface's file test — the text of a regular file, and
 * undefined for a directory or a path that is not there — and `list` a
 * directory's names, so the walk needs no third method and the self-test's
 * fixture source drives this same code.
 */
function licenceFiles(source: NoticeSource, root: string): string[] {
  const found: string[] = [];
  const walk = (dir: string, inLicenceDir: boolean): void => {
    for (const name of source.list(dir)) {
      const file = path.join(dir, name);
      const code = CODE_EXTENSIONS.some((extension) => name.toLowerCase().endsWith(extension));
      if (source.read(file) !== undefined) {
        if (!code && (inLicenceDir || LICENCE_WORD.test(name))) found.push(file);
        continue;
      }
      if (name === NODE_MODULES) continue;
      walk(file, inLicenceDir || LICENCE_DIR_WORD.test(name));
    }
  };
  walk(root, false);
  return found.sort();
}

/**
 * One section as the assembled file carries it: the rule, the header naming
 * the package and the source file, the rule, then the text. The installed
 * file is checked as this unit, not by its header and its text apart: five of
 * the real file's sections share the MIT body under different headers, so a
 * body deleted from one section is still present in another, and only a
 * whole-section match sees it (package-smoke/contents.ts, noticesProblems).
 */
export function noticeSection(notice: NoticeText): string {
  return `${RULE}\n${notice.label}\n${RULE}\n${notice.text}`;
}

/**
 * The notices file for the packages the staged bundles inlined: per package,
 * its licence texts at any depth — the files under its `THIRD_PARTY_NOTICES`
 * among them — each under a header naming the package, its version and the
 * source file's name within the package, or the SPDX MIT fallback where the
 * tree carries no licence text of its own. Packages are sorted by name and
 * their texts by file name, so one set of packages always assembles to one
 * text. `source` is the reader: the file system in staging, a fixture map in
 * the self-test.
 */
export function assembleNotices(packages: readonly BundlePackage[], source: NoticeSource): NoticeAssembly {
  const ordered = [...packages].sort((a, b) => a.name.localeCompare(b.name) || a.version.localeCompare(b.version));
  const sections: NoticeText[] = [];
  for (const pkg of ordered) {
    const manifest = manifestOf(pkg, source);
    const what = `${pkg.name}@${pkg.version}`;
    const found = licenceFiles(source, pkg.root);
    // The package's own terms are the texts it carries outside the directory
    // it keeps other code's notices in. A tree whose only licence text is one
    // of those — `@earendil-works/pi-schema` ships
    // `THIRD_PARTY_NOTICES/typebox-LICENSE` and no licence file of its own —
    // still has its own terms stated by the SPDX fallback.
    const thirdDir = path.join(pkg.root, THIRD_PARTY_DIR);
    const own = found.filter((file) => !(file === thirdDir || file.startsWith(`${thirdDir}${path.sep}`)));
    if (own.length === 0) sections.push(mitFallback(pkg, manifest));
    for (const file of found) {
      // Each file's name within its package, `/`-spelled, is what its header
      // names: one package assembles to one label and one text on every OS.
      const relative = path.relative(pkg.root, file).split(path.sep).join("/");
      const label = `${what} / ${relative}`;
      sections.push({ label, text: readText(source, file, label) });
    }
  }
  return {
    packages: ordered.map((pkg) => ({ name: pkg.name, version: pkg.version })),
    texts: sections,
    text: [...PREAMBLE, "", ...sections.map(noticeSection)].join("\n"),
  };
}
