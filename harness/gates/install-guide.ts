import { existsSync } from "node:fs";

import {
  caskProblems,
  CASK_FACTS,
  CASK_SHA256,
  CASK_TAP_PATH,
  CASK_VERSION,
  MACOS_DATA_DIRECTORY,
  placeholderProblems,
  renderCask,
} from "../lib/cask.ts";
import type { CaskFacts } from "../lib/cask.ts";
import { field, stringField } from "../lib/json.ts";
import { readEach, resolve } from "../lib/repo.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

/**
 * The two install guides are one document in two languages, so this gate reads
 * them as a pair: a fact moved to a section where it does not belong, a command
 * that drifts between the sides, a link that loses its zh-CN counterpart, a
 * dropped screenshot place or a document that still offers an old macOS fails
 * here. Each fact is pinned inside its own section, never anywhere in the file,
 * so relocating a sentence is a failure and not a keyword match. Pure text on
 * every OS; the tap install needs a network and is a CI job, not a gate. A
 * string a guide quotes from the app is checked against the app's own sources,
 * so a quote a learner never sees fails here; so does the removed right-click
 * → Open path. The cask template the guides' `brew install` line installs is
 * read as a third document: rendered with the published v0.15.0 values, its
 * stanzas must match the same facts and its bytes must equal the pinned render,
 * so a template edit is a reviewable diff (P3).
 */
export const ENGLISH_GUIDE = "docs/install.md";
export const CHINESE_GUIDE = "docs/install.zh-CN.md";
export const RELEASE_NOTES = ".github/release-notes.md";
/** The cask template P4's release job renders into the tap's `CASK_TAP_PATH`. */
export const CASK_TEMPLATE_PATH = ".github/homebrew-cask-template.rb";
/** The pinned render of the template with `CASK_VERSION` and `CASK_SHA256`: an edited template must match this file byte for byte. */
export const CASK_GOLDEN_PATH = "harness/gates/testdata/pass-quest-0.15.0.rb";
const MANIFEST = "mygo.json";

/** The one Homebrew command both guides and the release notes name (P3 renders the matching cask). */
export const TAP_COMMAND = "brew install --cask wxxb789/tap/pass-quest";
/** The Terminal equivalent of the Open Anyway exception; quoted, because the app path holds a space. */
export const QUARANTINE_COMMAND = 'xattr -dr com.apple.quarantine "/Applications/Pass Quest.app"';
/** The bundle identifier `mygo.json` declares; the cask's `uninstall quit:` carries it (P3). */
const BUNDLE_ID = "com.github.wxxb789.passquest";
/** The directory the English guide's relative links resolve against. */
const GUIDE_DIR = ENGLISH_GUIDE.slice(0, ENGLISH_GUIDE.lastIndexOf("/") + 1);

/** The app files the gate reads, so a string the guides quote must still be in the source that ships it. */
export const APP_SOURCES = [
  "internal/desktop/paths.go",
  "internal/desktop/webview.go",
  "desktop/web/src/core/i18n.ts",
  "desktop/web/src/views/settings.ts",
] as const;
/** The app name's one Go declaration; a guide spells the name out where the app formats it in. */
const APP_NAME_SOURCE = "internal/desktop/paths.go";
const APP_NAME_DECLARATION = /AppName\s*=\s*"([^"]+)"/;

/** The `##` sections of the English guide, in order; the Chinese guide mirrors the structure position by position. */
const GUIDE_SECTIONS = [
  "Supported systems",
  "Windows",
  "macOS",
  "Linux",
  "Where your data lives, and how to remove it",
  "Upgrade",
  "What CI proves, and what is untested",
] as const;

type Pin = string | RegExp;

interface Fact {
  section: (typeof GUIDE_SECTIONS)[number];
  what: string;
  english: readonly Pin[];
  chinese: readonly Pin[];
}

/**
 * Every fact and the single section that must carry it. The English and Chinese
 * pins are the same fact in each language: an ASCII token (a command, a UI name
 * the system itself shows, a version), or the wording the guide uses for it.
 */
const FACTS: readonly Fact[] = [
  {
    section: "Supported systems",
    what: "Windows 11 as a supported system",
    english: ["Windows 11"],
    chinese: ["Windows 11"],
  },
  {
    section: "Supported systems",
    what: "Windows 10 as unsupported and untested",
    english: ["Windows 10", /unsupported/, /untested/],
    chinese: ["Windows 10", "不受支持", "未经测试"],
  },
  {
    section: "Supported systems",
    what: "Windows 10 as not blocked",
    english: ["not blocked"],
    chinese: ["不会被阻止"],
  },
  {
    section: "Supported systems",
    what: "Linux as second-tier",
    english: ["second-tier"],
    chinese: ["第二梯队"],
  },
  {
    section: "Windows",
    what: "the SmartScreen step (More info, then Run anyway)",
    english: ["SmartScreen", "More info", "Run anyway"],
    chinese: ["SmartScreen", "更多信息", "仍要运行"],
  },
  {
    section: "Windows",
    what: "the WebView2 prerequisite",
    english: ["WebView2"],
    chinese: ["WebView2"],
  },
  {
    section: "macOS",
    what: "the Open Anyway flow under Privacy & Security",
    english: ["Open Anyway", "Privacy & Security"],
    chinese: ["Open Anyway", "仍要打开", "Privacy & Security", "隐私与安全性"],
  },
  {
    section: "macOS",
    what: "the source and date of the Open Anyway flow (Apple's support page 102445 and the macOS User Guide)",
    english: ["support.apple.com/en-us/102445", "2026-05-27", "mh40616"],
    chinese: ["support.apple.com/en-us/102445", "2026-05-27", "mh40616"],
  },
  {
    section: "macOS",
    what: "the source of the supported macOS pair (Apple's page listing the latest versions)",
    english: ["https://support.apple.com/en-us/109033"],
    chinese: ["https://support.apple.com/en-us/109033"],
  },
  {
    section: "macOS",
    what: "that the app is dragged into Applications first, and why (read-only image, ejected)",
    english: ["Applications", "read-only", /eject/],
    chinese: ["应用程序", "只读", "eject"],
  },
  {
    section: "macOS",
    what: "that the step is needed once per download and that Homebrew avoids it",
    english: ["once per download"],
    chinese: ["每次下载"],
  },
  {
    section: "macOS",
    what: "the Homebrew tap command",
    english: [TAP_COMMAND],
    chinese: [TAP_COMMAND],
  },
  {
    section: "macOS",
    what: "the quarantine alternative",
    english: [QUARANTINE_COMMAND],
    chinese: [QUARANTINE_COMMAND],
  },
  {
    section: "Where your data lives, and how to remove it",
    what: "the macOS data directory",
    english: [MACOS_DATA_DIRECTORY],
    chinese: [MACOS_DATA_DIRECTORY],
  },
  {
    section: "Upgrade",
    what: "the Updates row and its check button under the names the app shows",
    english: ["**Updates**", "**Check for updates**"],
    chinese: ["**更新**（Updates）", "**检查更新**（Check for updates）"],
  },
];

/**
 * A string the app itself shows and a guide quotes. Each quote is checked twice:
 * the file the app ships it from must still hold the literal — an app that
 * reworded a message fails here — and the section that quotes it must carry the
 * app's own text, so a paraphrase of something a learner never sees fails too.
 * `%s` is where the app formats its own name in; a guide spells that name out.
 */
interface AppQuote {
  what: string;
  section: (typeof GUIDE_SECTIONS)[number];
  source: (typeof APP_SOURCES)[number];
  literal: string;
  /** The Chinese string, where the app ships one; absent means it shows the literal in either language. */
  chinese?: string;
}

const APP_QUOTES: readonly AppQuote[] = [
  {
    what: "the message shown when the WebView2 runtime is missing",
    section: "Windows",
    source: "internal/desktop/webview.go",
    literal:
      "%s needs the Microsoft Edge WebView2 Runtime, which this PC does not have. Install it from https://developer.microsoft.com/microsoft-edge/webview2/ (the Evergreen Standalone Installer), then start %s again.",
  },
  {
    what: "the Updates row's name",
    section: "Upgrade",
    source: "desktop/web/src/views/settings.ts",
    literal: "Updates",
    chinese: "更新",
  },
  {
    what: "the check-for-updates button's name",
    section: "Upgrade",
    source: "desktop/web/src/views/settings.ts",
    literal: "Check for updates",
    chinese: "检查更新",
  },
  {
    what: "the notice for a database a newer build wrote",
    section: "Upgrade",
    source: "desktop/web/src/core/i18n.ts",
    literal: "This data is from a newer Pass Quest",
    chinese: "数据来自更新的 Pass Quest",
  },
  {
    what: "the next step for a database a newer build wrote",
    section: "Upgrade",
    source: "desktop/web/src/core/i18n.ts",
    literal: "Install the newer Pass Quest that wrote this data; this version will not change it.",
    chinese: "安装写入这些数据的更新版本；当前版本不会改动它。",
  },
  {
    what: "the notice for a repository with no published release",
    section: "Upgrade",
    source: "desktop/web/src/views/settings.ts",
    literal: "No release has been published for this repository yet, so there is nothing newer to install.",
    chinese: "这个仓库还没有发布任何版本，所以现在没有可安装的新版本。",
  },
];

/** The sections the owner captures; each must keep at least one of its screenshot places. */
const SCREENSHOT_SECTIONS = ["Windows", "macOS"] as const;
/** The macOS 26/27 regression this gate exists to prevent: the removed right-click → Open path offered as a step. */
const RIGHT_CLICK_PATH = /right-click|control-click|右键|辅助点按/i;
const OPEN_STEP = /open|打开/i;
/** A guide may still say the path is gone; that sentence keeps "no longer" / "不再". */
const PATH_REMOVED = /no longer|不再/;

/** No guide may name a way to switch Gatekeeper off for the whole system. */
const BANNED_GATEKEEPER = ["spctl --master-disable", "--global-disable"];
/** A macOS release below the supported pair: the guides and the release notes may not still name one. */
const STALE_MAJOR = /\bmacOS +(?:\d+\.)?(?:1[0-9]|2[0-5])(?:\.\d+)?\b/;
const STALE_NAMES = ["Ventura", "Sonoma", "Sequoia"];

const FENCE = /^```(\S*)\s*$/;
const CLOSE_FENCE = /^```\s*$/;
const HEADING = /^(#{1,3}) +(.*)$/;
const SECTION_HEADING = /^##(?!#) +(.*)$/;
const MARKER = /^<!--\s*screenshot:\s*([a-z0-9][a-z0-9-]*)\s*-->$/;
const ITALIC = /^\*[^*].*\*$/;
const LINK = /\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g;
const SCHEME = /^[a-z][a-z0-9+.-]*:/i;
const MAJOR_MENTION = /\bmacOS +(\d+)/g;

function lines(text: string): string[] {
  return text.replace(/\r\n/g, "\n").split("\n");
}

interface Section {
  heading: string;
  body: string;
}

/** The `##` sections of a guide in order; a fenced block is body, never headings. */
function sections(text: string): Section[] {
  const found: Array<{ heading: string; body: string[] }> = [];
  let fenced = false;
  for (const line of lines(text)) {
    if (FENCE.test(line)) fenced = !fenced;
    const heading = fenced ? null : SECTION_HEADING.exec(line);
    if (heading) {
      found.push({ heading: heading[1].trim(), body: [] });
      continue;
    }
    found.at(-1)?.body.push(line);
  }
  return found.map((section) => ({ heading: section.heading, body: section.body.join("\n") }));
}

/** The level of every `#`/`##`/`###` heading in order, fences excluded. */
function headingLevels(text: string): number[] {
  const levels: number[] = [];
  let fenced = false;
  for (const line of lines(text)) {
    if (FENCE.test(line)) {
      fenced = !fenced;
      continue;
    }
    if (fenced) continue;
    const heading = HEADING.exec(line);
    if (heading) levels.push(heading[1].length);
  }
  return levels;
}

/** The screenshot marker slugs of a text in order. */
function screenshotMarkers(text: string): string[] {
  return lines(text)
    .map((line) => MARKER.exec(line)?.[1])
    .filter((slug): slug is string => slug !== undefined);
}

/** Every marker must be followed by the italic caption line the owner's screenshot replaces. */
function markerProblems(text: string, guide: string): string[] {
  const all = lines(text);
  const problems: string[] = [];
  for (let index = 0; index < all.length; index++) {
    const marker = MARKER.exec(all[index]);
    if (!marker) continue;
    let next = index + 1;
    while (next < all.length && all[next].trim() === "") next++;
    if (!ITALIC.test(all[next] ?? "")) {
      problems.push(`${guide}:${index + 1}: the screenshot marker "${marker[1]}" must be followed by an italic caption line`);
    }
  }
  return problems;
}

/** Each fenced block as its info string plus its body, so two sides can be compared byte for byte. */
function codeBlocks(text: string): string[] {
  const blocks: string[] = [];
  let open: string[] | null = null;
  for (const line of lines(text)) {
    if (open === null) {
      const fence = FENCE.exec(line);
      if (fence) open = [fence[1]];
      continue;
    }
    if (CLOSE_FENCE.test(line)) {
      blocks.push(open.join("\n"));
      open = null;
      continue;
    }
    open.push(line);
  }
  // An unterminated fence is kept, so a missing closing line shows up as a differing block.
  if (open !== null) blocks.push(open.join("\n"));
  return blocks;
}

function linkTargets(text: string): { relative: string[]; external: string[] } {
  const relative: string[] = [];
  const external: string[] = [];
  for (const match of text.matchAll(LINK)) {
    const target = (match[1] ?? "").split("#", 1)[0];
    if (!target) continue;
    if (SCHEME.test(target)) external.push(target);
    else relative.push(target);
  }
  return { relative, external };
}

/** A guide link's zh-CN counterpart when the tree has one: `../README.md` -> `../README.zh-CN.md`. */
function zhCounterpart(root: string, target: string): string {
  if (!target.endsWith(".md") || target.endsWith(".zh-CN.md")) return target;
  const variant = `${target.slice(0, -".md".length)}.zh-CN.md`;
  // The link is written relative to the guide, so the counterpart is looked up there too.
  return existsSync(resolve(root, `${GUIDE_DIR}${variant}`)) ? variant : target;
}

function preview(value: string): string {
  const one = value.replace(/\n/g, "\\n");
  return one.length > 160 ? `${one.slice(0, 157)}...` : one;
}

/** The first line on which two texts differ, for the pinned render's failure message. */
function firstDifference(rendered: string, golden: string): string {
  const renderedLines = rendered.split("\n");
  const goldenLines = golden.split("\n");
  for (let index = 0; index < Math.max(renderedLines.length, goldenLines.length); index++) {
    if (renderedLines[index] !== goldenLines[index]) {
      const left = JSON.stringify(renderedLines[index] ?? "(end of file)");
      const right = JSON.stringify(goldenLines[index] ?? "(end of file)");
      return `first differing line ${index + 1}: ${left} against the golden's ${right}`;
    }
  }
  return "the two texts hold the same lines and differ in encoding only";
}

export const installGuide: Gate = {
  id: "install-guide",
  title: "install guide",
  async run(ctx) {
    const documents = await readEach(ctx.root, [
      ENGLISH_GUIDE,
      CHINESE_GUIDE,
      "README.md",
      "README.zh-CN.md",
      RELEASE_NOTES,
      MANIFEST,
      CASK_TEMPLATE_PATH,
      CASK_GOLDEN_PATH,
    ]);
    const paths = [ENGLISH_GUIDE, CHINESE_GUIDE, "README.md", "README.zh-CN.md", RELEASE_NOTES, MANIFEST, CASK_TEMPLATE_PATH, CASK_GOLDEN_PATH];
    const unreadable = paths.filter((_, index) => documents[index] === null);
    if (unreadable.length > 0) return fail(unreadable.map((path) => `${path}: missing or unreadable`));
    const [english, chinese, readme, readmeChinese, notes, manifest, caskTemplate, caskGolden] = paths.map((_, index) =>
      documents[index]!.toString("utf8"),
    ) as string[];
    const sourceDocuments = await readEach(ctx.root, [...APP_SOURCES]);
    const unreadableSources = APP_SOURCES.filter((_, index) => sourceDocuments[index] === null);
    if (unreadableSources.length > 0) return fail(unreadableSources.map((path) => `${path}: missing or unreadable`));
    const sourceOf = (path: string): string => {
      const index = (APP_SOURCES as readonly string[]).indexOf(path);
      return index < 0 ? "" : sourceDocuments[index]!.toString("utf8");
    };

    const problems: string[] = [];
    const englishSections = sections(english);
    const chineseSections = sections(chinese);

    // The English guide's section skeleton is the placement contract; the Chinese
    // guide is a translation, so only its structure (not its heading text) must match.
    const foundSections = englishSections.map((section) => section.heading);
    if (foundSections.join(" | ") !== GUIDE_SECTIONS.join(" | ")) {
      problems.push(`the "##" sections of ${ENGLISH_GUIDE} must be exactly [${GUIDE_SECTIONS.join(" | ")}]; they are [${foundSections.join(" | ")}]`);
    }
    const englishLevels = headingLevels(english).join(",");
    const chineseLevels = headingLevels(chinese).join(",");
    if (chineseLevels !== englishLevels) {
      problems.push(`${CHINESE_GUIDE}: the headings must be the same in the same order as ${ENGLISH_GUIDE}: its levels are [${chineseLevels}] against [${englishLevels}]`);
    }

    // The floor: mygo.json is the one declaration, and a guide that names another
    // pair is either ahead of the package or still documenting a release it left.
    const manifestDocument: unknown = (() => {
      try {
        return JSON.parse(manifest);
      } catch {
        return null;
      }
    })();
    const floorText = stringField(field(manifestDocument, "macos"), "minimumSystemVersion");
    const floor = floorText && /^\d+(\.\d+)*$/.test(floorText) ? Number(floorText.split(".")[0]) : null;
    if (floor === null) {
      problems.push(`${MANIFEST}: macos.minimumSystemVersion must be a version like "26.0"; it is ${floorText === undefined ? "missing" : JSON.stringify(floorText)}`);
    }
    const identifier = stringField(manifestDocument, "identifier");
    if (identifier !== BUNDLE_ID) {
      problems.push(`${MANIFEST}: identifier must be ${BUNDLE_ID}, the bundle id the app registers under and the cask's uninstall quit: names; it is ${identifier === undefined ? "missing" : JSON.stringify(identifier)}`);
    }

    for (const [guide, text, language, guideSections] of [
      [ENGLISH_GUIDE, english, "english", englishSections],
      [CHINESE_GUIDE, chinese, "chinese", chineseSections],
    ] as const) {
      for (const fact of FACTS) {
        const index = GUIDE_SECTIONS.indexOf(fact.section);
        const body = guideSections[index]?.body;
        if (body === undefined) {
          problems.push(`${guide}: no "${fact.section}" section (section ${index + 1} of ${GUIDE_SECTIONS.length}), so ${fact.what} cannot be there`);
          continue;
        }
        for (const pin of language === "english" ? fact.english : fact.chinese) {
          const present = typeof pin === "string" ? body.includes(pin) : pin.test(body);
          if (!present) {
            problems.push(
              `${guide}: the "${fact.section}" section must carry ${fact.what}: missing ${typeof pin === "string" ? JSON.stringify(pin) : String(pin)}`,
            );
          }
        }
      }
      if (floor !== null) {
        const supported = guideSections[GUIDE_SECTIONS.indexOf("Supported systems")]?.body ?? "";
        const majors = [...new Set([...supported.matchAll(MAJOR_MENTION)].map((match) => Number(match[1])))].sort(
          (left, right) => left - right,
        );
        if (majors.join(",") !== `${floor},${floor + 1}`) {
          problems.push(`${guide}: the "Supported systems" section names macOS majors [${majors.join(", ") || "none"}]; ${MANIFEST} declares ${floorText}, so the supported pair is ${floor} and ${floor + 1}`);
        }
      }
      for (const banned of BANNED_GATEKEEPER) {
        if (text.includes(banned)) {
          problems.push(`${guide}: names ${JSON.stringify(banned)}, which disables Gatekeeper system-wide; the guide must not suggest that`);
        }
      }
      const stale = STALE_MAJOR.exec(text);
      if (stale) problems.push(`${guide}: names ${stale[0].trim()}, below the supported floor of macOS ${floor ?? 26}`);
      for (const name of STALE_NAMES) {
        if (text.includes(name)) problems.push(`${guide}: names macOS ${name}, below the supported floor`);
      }
      problems.push(...markerProblems(text, guide));
      // The regression this goal exists to prevent: a guide that still offers the
      // right-click → Open path macOS 26 and 27 removed. The sentence saying the
      // path is gone stays allowed, so whole paragraphs are read, not lines.
      const macos = guideSections[GUIDE_SECTIONS.indexOf("macOS")]?.body ?? "";
      for (const paragraph of macos.split(/\n\s*\n/)) {
        if (RIGHT_CLICK_PATH.test(paragraph) && OPEN_STEP.test(paragraph) && !PATH_REMOVED.test(paragraph)) {
          problems.push(
            `${guide}: the "macOS" section still offers the removed right-click → Open path (macOS 26 and 27 do not have it): ${preview(paragraph.trim())}`,
          );
        }
      }
      for (const name of SCREENSHOT_SECTIONS) {
        const places = screenshotMarkers(guideSections[GUIDE_SECTIONS.indexOf(name)]?.body ?? "");
        if (places.length === 0) problems.push(`${guide}: the "${name}" section must keep at least one screenshot place; it has none`);
      }
    }

    // Every string a guide quotes from the app has to exist twice: in the file the
    // app ships it from, and in the section that quotes it. An app that reworded a
    // message fails on the first; a paraphrase of text a learner never sees — the
    // defect this check exists for — fails on the second.
    const appName = APP_NAME_DECLARATION.exec(sourceOf(APP_NAME_SOURCE))?.[1];
    if (appName === undefined) {
      problems.push(`${APP_NAME_SOURCE}: no AppName = "..." declaration, so the strings the guides quote cannot be checked against the app`);
    }
    for (const quote of APP_QUOTES) {
      const source = sourceOf(quote.source);
      for (const text of [quote.literal, quote.chinese]) {
        if (text !== undefined && !source.includes(text)) {
          problems.push(`${quote.source}: no longer holds ${quote.what}: ${JSON.stringify(text)}`);
        }
      }
      if (appName === undefined) continue;
      const englishQuote = quote.literal.replaceAll("%s", appName);
      for (const [guide, guideSections, quoted] of [
        [ENGLISH_GUIDE, englishSections, englishQuote],
        [CHINESE_GUIDE, chineseSections, quote.chinese ?? englishQuote],
      ] as const) {
        const body = guideSections[GUIDE_SECTIONS.indexOf(quote.section)]?.body ?? "";
        // The English guide hard-wraps its prose; the section is compared flattened.
        if (!body.replace(/\s+/g, " ").includes(quoted)) {
          problems.push(`${guide}: the "${quote.section}" section must quote ${quote.what} as the app shows it: missing ${JSON.stringify(quoted)}`);
        }
      }
    }

    // Parity: the same commands byte for byte, and the same screenshot places per section.
    const englishBlocks = codeBlocks(english);
    const chineseBlocks = codeBlocks(chinese);
    if (englishBlocks.length !== chineseBlocks.length) {
      problems.push(`${CHINESE_GUIDE}: ${chineseBlocks.length} fenced code blocks against ${ENGLISH_GUIDE}'s ${englishBlocks.length}; the guides must run the same commands`);
    }
    for (let index = 0; index < Math.max(englishBlocks.length, chineseBlocks.length); index++) {
      const left = englishBlocks[index];
      const right = chineseBlocks[index];
      if (left === undefined || right === undefined || left === right) continue;
      problems.push(`${CHINESE_GUIDE}: code block ${index + 1} differs from ${ENGLISH_GUIDE}'s: ${preview(right)} against ${preview(left)}`);
    }
    for (let index = 0; index < Math.max(englishSections.length, chineseSections.length); index++) {
      const name = GUIDE_SECTIONS[index] ?? `section ${index + 1}`;
      const left = screenshotMarkers(englishSections[index]?.body ?? "");
      const right = screenshotMarkers(chineseSections[index]?.body ?? "");
      if (left.join(",") !== right.join(",")) {
        problems.push(`${CHINESE_GUIDE}: the "${name}" section's screenshot places [${right.join(", ")}] must match ${ENGLISH_GUIDE}'s [${left.join(", ")}]`);
      }
    }

    // Parity: the same links, with a zh-CN document linked where one exists, and the language cross-link.
    const chineseFile = CHINESE_GUIDE.split("/").pop() ?? CHINESE_GUIDE;
    const englishFile = ENGLISH_GUIDE.split("/").pop() ?? ENGLISH_GUIDE;
    const englishLinks = linkTargets(english);
    const chineseLinks = linkTargets(chinese);
    if (!englishLinks.relative.includes(chineseFile)) problems.push(`${ENGLISH_GUIDE}: must link its Chinese counterpart ${chineseFile}`);
    if (!chineseLinks.relative.includes(englishFile)) problems.push(`${CHINESE_GUIDE}: must link its English counterpart ${englishFile}`);
    const englishSide = englishLinks.relative.filter((target) => target !== chineseFile);
    const chineseSide = chineseLinks.relative.filter((target) => target !== englishFile);
    const expectedChinese = englishSide.map((target) => zhCounterpart(ctx.root, target));
    for (const target of expectedChinese) {
      if (!chineseSide.includes(target)) problems.push(`${CHINESE_GUIDE}: must link ${target}, which ${ENGLISH_GUIDE} links`);
    }
    for (const target of chineseSide) {
      if (!expectedChinese.includes(target)) problems.push(`${CHINESE_GUIDE}: links ${target}; ${ENGLISH_GUIDE} does not, so the link exists on one side only`);
    }
    for (const target of [...new Set([...englishLinks.external, ...chineseLinks.external])]) {
      if (!englishLinks.external.includes(target) || !chineseLinks.external.includes(target)) {
        problems.push(`the guides must carry the same external links: ${target} is in ${englishLinks.external.includes(target) ? ENGLISH_GUIDE : CHINESE_GUIDE} only`);
      }
    }

    // Each README links the guide in its own language; the release notes name the
    // tap command and both guides, so a macOS reader meets the Homebrew path early.
    for (const [readmePath, readmeText, guide] of [
      ["README.md", readme, ENGLISH_GUIDE],
      ["README.zh-CN.md", readmeChinese, CHINESE_GUIDE],
    ] as const) {
      const linked = linkTargets(readmeText).relative.some((target) => target.replace(/^\.\//, "") === guide);
      if (!linked) problems.push(`${readmePath}: must link ${guide}`);
    }
    if (!notes.includes(TAP_COMMAND)) problems.push(`${RELEASE_NOTES}: must name the Homebrew command ${JSON.stringify(TAP_COMMAND)}`);
    for (const guide of [ENGLISH_GUIDE, CHINESE_GUIDE]) {
      if (!notes.includes(guide)) problems.push(`${RELEASE_NOTES}: must link ${guide}`);
    }
    const staleNotes = STALE_MAJOR.exec(notes);
    if (staleNotes) problems.push(`${RELEASE_NOTES}: names ${staleNotes[0].trim()}, below the supported floor`);
    for (const name of STALE_NAMES) {
      if (notes.includes(name)) problems.push(`${RELEASE_NOTES}: names macOS ${name}, below the supported floor`);
    }

    // The cask the guides' one Homebrew command installs: the template rendered
    // with the published v0.15.0 values (the release's own asset and SHA-256,
    // plan fact 6) must carry every stanza the tap needs and no placeholder, and
    // must equal the pinned render byte for byte. The floor and the identifier
    // are injected from mygo.json, and the data directory is the guides' own
    // MACOS_DATA_DIRECTORY, so a cask that disagrees with the guides or the
    // manifest fails here, before any push. A second render with a synthetic
    // version and digest proves the two substituted values still travel through
    // the template instead of being hard-coded.
    const caskFacts: CaskFacts = {
      ...CASK_FACTS,
      minimumMajor: floor ?? CASK_FACTS.minimumMajor,
      identifier: identifier ?? CASK_FACTS.identifier,
    };
    const rendered = renderCask(caskTemplate, CASK_VERSION, CASK_SHA256);
    for (const problem of caskProblems(rendered, caskFacts)) problems.push(`${CASK_TEMPLATE_PATH}: ${problem}`);
    for (const problem of placeholderProblems(caskTemplate)) problems.push(`${CASK_TEMPLATE_PATH}: ${problem}`);
    // Line endings are normalized before the comparison: the tree pins LF, and a
    // CRLF checkout would otherwise fail as a phantom template edit.
    const renderedLf = rendered.replaceAll("\r\n", "\n");
    const goldenLf = caskGolden.replaceAll("\r\n", "\n");
    if (renderedLf !== goldenLf) {
      problems.push(
        `${CASK_GOLDEN_PATH}: the template no longer renders the pinned v${CASK_VERSION} cask (${firstDifference(renderedLf, goldenLf)}); re-render the golden and review the diff`,
      );
    }

    if (problems.length > 0) return fail(problems);
    const markerTotal = screenshotMarkers(english).length;
    return pass(
      `${GUIDE_SECTIONS.length} sections, ${englishBlocks.length} code blocks and ${markerTotal} screenshot places agree; ` +
        `${englishSide.length} links and ${englishLinks.external.length} URLs agree; ${APP_QUOTES.length} quoted app strings match their sources; ` +
        `the release notes name the tap command and both guides; the cask template renders v${CASK_VERSION} with the release's SHA-256, and a synthetic pair, into ${CASK_TAP_PATH}; ` +
        `floor ${floorText} from ${MANIFEST}`,
    );
  },
};
