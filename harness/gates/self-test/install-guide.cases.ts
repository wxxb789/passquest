import { readFileSync, rmSync, writeFileSync } from "node:fs";

import { CASK_SHA256, CASK_VERSION, renderCask } from "../../lib/cask.ts";
import { resolve } from "../../lib/repo.ts";

import {
  APP_SOURCES,
  CASK_GOLDEN_PATH,
  CASK_TEMPLATE_PATH,
  CHINESE_GUIDE,
  ENGLISH_GUIDE,
  installGuide,
  RELEASE_NOTES,
  TAP_COMMAND,
} from "../install-guide.ts";

import { copyProduct, patchCopy } from "./fixture.ts";
import type { BrokenCase, PassCase } from "./fixture.ts";

/**
 * The documents the gate reads, copied from the tree so each case starts from the
 * real pair and then breaks exactly one thing. `README.zh-CN.md` matters twice: it
 * is the README the Chinese guide must link, and it makes `../README.zh-CN.md` the
 * live zh-CN counterpart the link parity resolves.
 */
const GUIDE_DOCUMENTS = [
  ENGLISH_GUIDE,
  CHINESE_GUIDE,
  "README.md",
  "README.zh-CN.md",
  RELEASE_NOTES,
  "mygo.json",
  CASK_TEMPLATE_PATH,
  CASK_GOLDEN_PATH,
  ...APP_SOURCES,
];

/** A desc above the Cask-Cookbook's 80-character limit, for the case that must fail on it. */
const LONG_DESC = "Local-first AI exam coach for one written exam, with recall practice, essay review and pronunciation drills";

/**
 * The template's own `zap trash:` block, so a case can replace exactly it: the
 * three paths the macOS install job measured, in the indentation and order the
 * template carries.
 */
const ZAP_BLOCK = [
  "  zap trash: [",
  '    "~/Library/Application Support/Pass Quest",',
  '    "~/Library/Caches/com.github.wxxb789.passquest",',
  '    "~/Library/WebKit/com.github.wxxb789.passquest",',
  "  ]",
].join("\n");

/** The first expected fact that the gate's failure lines do not name. */
function expectedLines(failures: string[], expected: string[]): string | null {
  for (const line of expected) {
    if (!failures.some((failure) => failure.includes(line))) {
      return `expected the failure to name ${JSON.stringify(line)}; got: ${failures.join("; ") || "none"}`;
    }
  }
  return null;
}

/**
 * A cask case: the real documents plus one edit to the template, and the failure
 * lines the gate must print for it. Every check in `caskProblems` and the pinned
 * render have one, so none of them can go quiet.
 */
function caskCase(label: string, anchor: string, replacement: string, expected: string[]): BrokenCase {
  return {
    gate: installGuide,
    label,
    files: {},
    prepare: async (root) => {
      copyProduct(root, GUIDE_DOCUMENTS);
      return patchCopy(root, CASK_TEMPLATE_PATH, anchor, replacement);
    },
    postFail: (failures) => expectedLines(failures, expected),
  };
}

/**
 * A guide case: the real documents plus whatever `patch` edits, and the
 * failure lines the gate must print for it. `patch` receives the fixture root
 * with the documents already copied and returns the first `patchCopy` error
 * (null when every anchor was found).
 */
function guideCase(label: string, patch: (root: string) => string | null, expected: string[]): BrokenCase {
  return {
    gate: installGuide,
    label,
    files: {},
    prepare: async (root) => {
      copyProduct(root, GUIDE_DOCUMENTS);
      return patch(root);
    },
    postFail: (failures) => expectedLines(failures, expected),
  };
}

/** A guide pass case: the real documents plus `patch`'s edits, which the gate must accept. */
function guidePassCase(label: string, patch: (root: string) => string | null, postPass?: PassCase["postPass"]): PassCase {
  return {
    gate: installGuide,
    label,
    files: {},
    prepare: async (root) => {
      copyProduct(root, GUIDE_DOCUMENTS);
      return patch(root);
    },
    ...(postPass === undefined ? {} : { postPass }),
  };
}

export function installGuideCases(): BrokenCase[] {
  return [
    guideCase(
      "the Open Anyway step moved out of the macOS section",
      (root) =>
        // The wording stays in the file, in the wrong section, and no longer names
        // the button: only a fact pinned inside its own section can see that.
        patchCopy(
          root,
          ENGLISH_GUIDE,
          "choose **Open Anyway** — macOS offers the button for about an hour after the attempt.",
          "choose the button macOS offers.",
        ) ??
        patchCopy(root, ENGLISH_GUIDE, "the blocked-app notice and the Open Anyway button", "the blocked-app notice and the button beside it") ??
        patchCopy(root, ENGLISH_GUIDE, "## Linux\n", "## Linux\n\n**Open Anyway** is the button in System Settings on macOS.\n"),
      [
        `${ENGLISH_GUIDE}: the "macOS" section must carry the Open Anyway flow under Privacy & Security: missing "Open Anyway"`,
      ],
    ),
    guideCase(
      "the guide naming a macOS pair below the declared floor",
      (root) =>
        patchCopy(root, ENGLISH_GUIDE, "macOS 26 and macOS 27, the two latest majors", "macOS 24 and macOS 25, the two latest majors") ??
        // The rest of the section is brought to the same (wrong) floor, so the fixture is one guide written for macOS 24.
        patchCopy(root, ENGLISH_GUIDE, "the packages declare macOS 26 as their minimum", "the packages declare macOS 24 as their minimum") ??
        patchCopy(
          root,
          ENGLISH_GUIDE,
          "lists macOS 27 Golden Gate and macOS\nTahoe 26 as the latest versions",
          "lists macOS 24 and macOS 25 as the latest versions",
        ),
      [
        `${ENGLISH_GUIDE}: names macOS 24, below the supported floor of macOS 26`,
        `${ENGLISH_GUIDE}: the "Supported systems" section names macOS majors [24, 25]; mygo.json declares 26.0, so the supported pair is 26 and 27`,
      ],
    ),
    guideCase(
      "release notes without the Homebrew line",
      (root) => patchCopy(root, RELEASE_NOTES, TAP_COMMAND, "brew install --cask pass-quest"),
      [`${RELEASE_NOTES}: must name the Homebrew command ${JSON.stringify(TAP_COMMAND)}`],
    ),
    guideCase(
      "the Chinese guide dropping a screenshot place",
      (root) => patchCopy(root, CHINESE_GUIDE, "<!-- screenshot: macos-open-anyway-confirm -->\n", ""),
      [
        `${CHINESE_GUIDE}: the "macOS" section's screenshot places [macos-blocked-dialog, macos-privacy-security, macos-first-launch, macos-homebrew-install, macos-xattr-terminal] must match ${ENGLISH_GUIDE}'s [macos-blocked-dialog, macos-privacy-security, macos-open-anyway-confirm, macos-first-launch, macos-homebrew-install, macos-xattr-terminal]`,
      ],
    ),
    guideCase(
      "the Chinese guide running a different command",
      (root) =>
        patchCopy(
          root,
          CHINESE_GUIDE,
          'xattr -dr com.apple.quarantine "/Applications/Pass Quest.app"',
          "xattr -dr com.apple.quarantine /Applications/Pass Quest.app",
        ),
      [
        `${CHINESE_GUIDE}: code block 2 differs from ${ENGLISH_GUIDE}'s`,
        `${CHINESE_GUIDE}: the "macOS" section must carry the quarantine alternative: missing "xattr -dr com.apple.quarantine`,
      ],
    ),
    guideCase(
      "the guide naming a system-wide Gatekeeper disabling",
      (root) =>
        patchCopy(
          root,
          ENGLISH_GUIDE,
          "Do not disable Gatekeeper system-wide to get past the warning.",
          "If the warning persists, run sudo spctl --master-disable and try again.",
        ),
      [`${ENGLISH_GUIDE}: names "spctl --master-disable", which disables Gatekeeper system-wide; the guide must not suggest that`],
    ),
    guideCase(
      "a README that does not link its own guide",
      (root) => patchCopy(root, "README.zh-CN.md", "[安装指南](docs/install.zh-CN.md)", "[安装指南](docs/install.md)"),
      [`README.zh-CN.md: must link ${CHINESE_GUIDE}`],
    ),
    guideCase(
      "the Chinese guide dropping the zh-CN README link",
      (root) => patchCopy(root, CHINESE_GUIDE, "](../README.zh-CN.md)", "](../README.md)"),
      [
        `${CHINESE_GUIDE}: must link ../README.zh-CN.md, which ${ENGLISH_GUIDE} links`,
        `${CHINESE_GUIDE}: links ../README.md; ${ENGLISH_GUIDE} does not, so the link exists on one side only`,
      ],
    ),
    guideCase(
      "the English guide's section skeleton changed",
      (root) => patchCopy(root, ENGLISH_GUIDE, "\n## Upgrade\n", "\n## Upgrading\n"),
      [
        `the "##" sections of ${ENGLISH_GUIDE} must be exactly [Supported systems | Windows | macOS | Linux | Where your data lives, and how to remove it | Upgrade | What CI proves, and what is untested]; they are [Supported systems | Windows | macOS | Linux | Where your data lives, and how to remove it | Upgrading | What CI proves, and what is untested]`,
      ],
    ),
    guideCase(
      "the Chinese guide's heading levels no longer match",
      (root) => patchCopy(root, CHINESE_GUIDE, "\n## Linux\n", "\n### Linux\n"),
      [
        `${CHINESE_GUIDE}: the headings must be the same in the same order as ${ENGLISH_GUIDE}: its levels are [1,2,2,2,3,2,2,2] against [1,2,2,2,2,2,2,2]`,
      ],
    ),
    guideCase(
      "the Chinese guide missing from the tree",
      (root) => {
        rmSync(resolve(root, CHINESE_GUIDE));
        return null;
      },
      [`${CHINESE_GUIDE}: missing or unreadable`],
    ),
    guideCase(
      "a screenshot marker without its italic caption",
      (root) =>
        patchCopy(
          root,
          ENGLISH_GUIDE,
          '<!-- screenshot: windows-smartscreen -->\n*Screenshot: the SmartScreen "Windows protected your PC" dialog and its **More info** link (owner).*\n',
          "<!-- screenshot: windows-smartscreen -->\n",
        ),
      [`${ENGLISH_GUIDE}:53: the screenshot marker "windows-smartscreen" must be followed by an italic caption line`],
    ),
    guideCase(
      "the English guide gaining a code block the Chinese guide lacks",
      (root) =>
        patchCopy(
          root,
          ENGLISH_GUIDE,
          "CI itself runs the AppImage that way and never mounts it with FUSE.",
          "```bash\nls\n```\n\nCI itself runs the AppImage that way and never mounts it with FUSE.",
        ),
      [`${CHINESE_GUIDE}: 4 fenced code blocks against ${ENGLISH_GUIDE}'s 5; the guides must run the same commands`],
    ),
    guideCase(
      "an external URL on the English side only",
      (root) =>
        patchCopy(
          root,
          ENGLISH_GUIDE,
          "installing the packaged app; what the app does is described in the [project README](../README.md).",
          "installing the packaged app, or through [the walkthrough](https://example.com/only-english); what the app does is described in the [project README](../README.md).",
        ),
      [`the guides must carry the same external links: https://example.com/only-english is in ${ENGLISH_GUIDE} only`],
    ),
    guideCase(
      "the English guide not linking its Chinese counterpart",
      (root) => patchCopy(root, ENGLISH_GUIDE, "[简体中文](install.zh-CN.md)", "[简体中文](install.md)"),
      [`${ENGLISH_GUIDE}: must link its Chinese counterpart install.zh-CN.md`],
    ),
    guideCase(
      "the release notes dropping the Chinese guide link",
      (root) => patchCopy(root, RELEASE_NOTES, " ([简体中文](https://github.com/wxxb789/passquest/blob/{{TAG}}/docs/install.zh-CN.md))", ""),
      [`${RELEASE_NOTES}: must link ${CHINESE_GUIDE}`],
    ),
    guideCase(
      "a guide still naming macOS Sonoma",
      (root) => patchCopy(root, ENGLISH_GUIDE, "## Supported systems\n", "## Supported systems\n\nmacOS Sonoma is not supported.\n"),
      [`${ENGLISH_GUIDE}: names macOS Sonoma, below the supported floor`],
    ),
    guideCase(
      "an unparsable mygo.json floor",
      (root) => patchCopy(root, "mygo.json", '"minimumSystemVersion": "26.0"', '"minimumSystemVersion": "tahoe"'),
      [`mygo.json: macos.minimumSystemVersion must be a version like "26.0"; it is "tahoe"`],
    ),
    guideCase(
      "the manifest declaring another bundle identifier",
      (root) => patchCopy(root, "mygo.json", '"identifier": "com.github.wxxb789.passquest"', '"identifier": "com.github.wxxb789.pass-quest"'),
      [
        `mygo.json: identifier must be com.github.wxxb789.passquest, the bundle id the app registers under and the cask's uninstall quit: names; it is "com.github.wxxb789.pass-quest"`,
      ],
    ),
    guideCase(
      "the macOS section back to the removed right-click → Open step",
      (root) =>
        patchCopy(
          root,
          ENGLISH_GUIDE,
          "1. Open **Pass Quest** from **Applications** and let macOS refuse it.",
          "1. Right-click **Pass Quest** in **Applications**, choose **Open**, and confirm the dialog.",
        ),
      [`${ENGLISH_GUIDE}: the "macOS" section still offers the removed right-click → Open path (macOS 26 and 27 do not have it)`],
    ),
    guideCase(
      "the macOS section losing every screenshot place",
      (root) => {
        // Both guides lose every marker, so the per-section minimum is the only check that can see it.
        for (const guide of [ENGLISH_GUIDE, CHINESE_GUIDE]) {
          const file = resolve(root, guide);
          const kept = readFileSync(file, "utf8").split("\n").filter((line) => !line.startsWith("<!-- screenshot:"));
          writeFileSync(file, kept.join("\n"));
        }
        return null;
      },
      [
        `${ENGLISH_GUIDE}: the "macOS" section must keep at least one screenshot place; it has none`,
        `${CHINESE_GUIDE}: the "macOS" section must keep at least one screenshot place; it has none`,
      ],
    ),
    guideCase(
      "the Chinese guide paraphrasing an app string",
      (root) => patchCopy(root, CHINESE_GUIDE, "数据来自更新的 Pass Quest", "数据来自更新版本的 Pass Quest"),
      [
        `${CHINESE_GUIDE}: the "Upgrade" section must quote the notice for a database a newer build wrote as the app shows it: missing "数据来自更新的 Pass Quest"`,
      ],
    ),
    guideCase(
      "the app rewording a string the guides quote",
      (root) => patchCopy(root, "desktop/web/src/core/i18n.ts", "'数据来自更新的 Pass Quest'", "'数据来自更新版本的 Pass Quest'"),
      [`desktop/web/src/core/i18n.ts: no longer holds the notice for a database a newer build wrote: "数据来自更新的 Pass Quest"`],
    ),
    guideCase(
      "the guide naming macOS 13",
      (root) => patchCopy(root, ENGLISH_GUIDE, "Any macOS older than 26\nis unsupported", "Any macOS 13 or older\nis unsupported"),
      [
        `${ENGLISH_GUIDE}: names macOS 13, below the supported floor of macOS 26`,
        `${ENGLISH_GUIDE}: the "Supported systems" section names macOS majors [13, 26, 27]; mygo.json declares 26.0, so the supported pair is 26 and 27`,
      ],
    ),
    guideCase(
      "the Chinese guide not linking its English counterpart",
      (root) => patchCopy(root, CHINESE_GUIDE, "[English](install.md)", "[English](install.zh-CN.md)"),
      [
        `${CHINESE_GUIDE}: must link its English counterpart install.md`,
        `${CHINESE_GUIDE}: links install.zh-CN.md; ${ENGLISH_GUIDE} does not, so the link exists on one side only`,
      ],
    ),
    guideCase(
      "the release notes naming a macOS release below the floor",
      (root) => patchCopy(root, RELEASE_NOTES, "`SHA256SUMS.txt` lists each file's SHA-256.", "`SHA256SUMS.txt` lists each file's SHA-256. macOS Sonoma is not supported."),
      [`${RELEASE_NOTES}: names macOS Sonoma, below the supported floor`],
    ),
    guideCase(
      "the app's name declaration gone",
      (root) => patchCopy(root, "internal/desktop/paths.go", 'AppName       = "Pass Quest"', "AppName       = appDisplayName"),
      [`internal/desktop/paths.go: no AppName = "..." declaration, so the strings the guides quote cannot be checked against the app`],
    ),
    guideCase(
      "an app source the guides quote from missing",
      (root) => {
        rmSync(resolve(root, "desktop/web/src/views/settings.ts"));
        return null;
      },
      [`desktop/web/src/views/settings.ts: missing or unreadable`],
    ),
    guideCase(
      "the macOS section dropping the Keychain prompt's answer and why it appears",
      (root) => {
        const edits: [string, string, string][] = [
          [ENGLISH_GUIDE, "and choose **Always Allow**", "and choose the button macOS offers"],
          [CHINESE_GUIDE, "选择**始终允许**", "选择系统提供的按钮"],
          [ENGLISH_GUIDE, "not signed by an Apple-issued identity", "not signed by a certificate Apple issued"],
          [CHINESE_GUIDE, "没有 Apple 签发的签名标识", "没有 Apple 开发者签名"],
        ];
        for (const [file, anchor, replacement] of edits) {
          const problem = patchCopy(root, file, anchor, replacement);
          if (problem) return problem;
        }
        return null;
      },
      [
        `${ENGLISH_GUIDE}: the "macOS" section must carry the Keychain prompt one upgrade brings, the answer to it and why it appears: missing "Always Allow"`,
        `${CHINESE_GUIDE}: the "macOS" section must carry the Keychain prompt one upgrade brings, the answer to it and why it appears: missing "始终允许"`,
        `${ENGLISH_GUIDE}: the "macOS" section must carry the Keychain prompt one upgrade brings, the answer to it and why it appears: missing "not signed by an Apple-issued identity"`,
        `${CHINESE_GUIDE}: the "macOS" section must carry the Keychain prompt one upgrade brings, the answer to it and why it appears: missing "没有 Apple 签发的签名标识"`,
      ],
    ),
    guideCase(
      "the What-CI-proves section dropping the two endpoint formats",
      (root) => {
        const edits: [string, string][] = [
          [
            ENGLISH_GUIDE,
            "The model path is proved over both endpoint formats: an\nendpoint whose path ends in `/chat/completions` and one ending in `/responses` are both driven, and\nthe path alone selects the wire format (the tutor smoke, the provider smoke, the agent smoke's four\npasses and the package smoke's `brief-responses` case). ",
          ],
          [
            CHINESE_GUIDE,
            "模型路径在两种端点格式上都有验证：以 `/chat/completions` 结尾和以 `/responses` 结尾的端点都会被驱动，线格式只由路径决定（tutor smoke、provider smoke、agent smoke 的四轮通过和 package smoke 的 `brief-responses` 用例）。",
          ],
        ];
        for (const [file, sentence] of edits) {
          const problem = patchCopy(root, file, sentence, "");
          if (problem) return problem;
        }
        return null;
      },
      [
        `${ENGLISH_GUIDE}: the "What CI proves, and what is untested" section must carry both endpoint formats work and the path selects the wire format: missing "both endpoint formats"`,
        `${ENGLISH_GUIDE}: the "What CI proves, and what is untested" section must carry both endpoint formats work and the path selects the wire format: missing "/chat/completions"`,
        `${ENGLISH_GUIDE}: the "What CI proves, and what is untested" section must carry both endpoint formats work and the path selects the wire format: missing "/responses"`,
        `${CHINESE_GUIDE}: the "What CI proves, and what is untested" section must carry both endpoint formats work and the path selects the wire format: missing "两种端点格式"`,
        `${CHINESE_GUIDE}: the "What CI proves, and what is untested" section must carry both endpoint formats work and the path selects the wire format: missing "/chat/completions"`,
        `${CHINESE_GUIDE}: the "What CI proves, and what is untested" section must carry both endpoint formats work and the path selects the wire format: missing "/responses"`,
      ],
    ),
    // The cask half: every check in `caskProblems`, `placeholderProblems` and
    // the pinned render have a case here. A template edit fails its semantic
    // check and the golden line together, so each case names the check it
    // exists for.
    caskCase(
      "a cask template nobody rendered",
      'version "@VERSION@"',
      'version "@VERS1ON@"',
      [`${CASK_TEMPLATE_PATH}: the rendered cask still carries @VERS1ON@; a placeholder nobody substituted would reach the tap verbatim`],
    ),
    caskCase(
      "a cask carrying a lower-case leftover",
      'version "@VERSION@"',
      'version "@version@"',
      [`${CASK_TEMPLATE_PATH}: the rendered cask still carries @version@; a placeholder nobody substituted would reach the tap verbatim`],
    ),
    caskCase(
      "a cask that lost the version line",
      '  version "@VERSION@"\n',
      "",
      [`${CASK_TEMPLATE_PATH}: version must be 0.15.0, the release this cask was rendered from; it is absent`],
    ),
    caskCase(
      "a cask whose version was written as another release",
      'version "@VERSION@"',
      'version "0.14.0"',
      [`${CASK_TEMPLATE_PATH}: version must be 0.15.0, the release this cask was rendered from; it is "0.14.0"`],
    ),
    caskCase(
      "a cask whose sha256 is not a digest",
      'sha256 "@SHA256@"',
      'sha256 "0123456789abcdef"',
      [`${CASK_TEMPLATE_PATH}: sha256 must be one 64-character lowercase hex string; the release's own SHA256SUMS.txt is the source`],
    ),
    caskCase(
      "a cask whose sha256 is another 64-hex value",
      'sha256 "@SHA256@"',
      `sha256 "${"0".repeat(64)}"`,
      [`${CASK_TEMPLATE_PATH}: sha256 must be the release asset's own digest ${CASK_SHA256}`],
    ),
    caskCase(
      "a cask whose header token does not match its file",
      'cask "pass-quest" do',
      'cask "passquest" do',
      [`${CASK_TEMPLATE_PATH}: the header must be cask "pass-quest" do`, "cask/cask_loader.rb"],
    ),
    caskCase(
      "a cask whose url is not the release asset",
      "pass-quest-#{version}-arm64.dmg",
      "pass-quest-#{version}.dmg",
      [`${CASK_TEMPLATE_PATH}: url must be https://github.com/wxxb789/passquest/releases/download/v#{version}/pass-quest-#{version}-arm64.dmg`],
    ),
    caskCase(
      "a cask installing another app name",
      'app "Pass Quest.app"',
      'app "PassQuest.app"',
      [`${CASK_TEMPLATE_PATH}: app must be "Pass Quest.app"`],
    ),
    caskCase(
      "a cask pointing at another homepage",
      'homepage "https://github.com/wxxb789/passquest"',
      'homepage "https://github.com/wxxb789/passquest-app"',
      [`${CASK_TEMPLATE_PATH}: homepage must be https://github.com/wxxb789/passquest`],
    ),
    caskCase(
      "a cask without a desc",
      '  desc "Local-first AI exam coach for one written exam"\n',
      "",
      [`${CASK_TEMPLATE_PATH}: no desc stanza; the cask must state what the app is`],
    ),
    caskCase(
      "a cask whose desc is above 80 characters",
      'desc "Local-first AI exam coach for one written exam"',
      `desc "${LONG_DESC}"`,
      [`${CASK_TEMPLATE_PATH}: desc is ${LONG_DESC.length} characters, above the 80-character limit`],
    ),
    caskCase(
      "a cask whose desc starts with the app's name",
      'desc "Local-first AI exam coach for one written exam"',
      'desc "Pass Quest is a local-first AI exam coach"',
      [`${CASK_TEMPLATE_PATH}: desc starts with the app's own name, which the cask title already carries: "Pass Quest is a local-first AI exam coach"`],
    ),
    caskCase(
      "a cask without a macOS floor",
      "  depends_on macos: :tahoe\n",
      "",
      [`${CASK_TEMPLATE_PATH}: depends_on macos: must state the minimum as a code symbol; it is absent or not one symbol`],
    ),
    caskCase(
      "a cask whose macOS floor is below the declared one",
      "depends_on macos: :tahoe",
      "depends_on macos: :sonoma",
      [`${CASK_TEMPLATE_PATH}: depends_on macos: :sonoma against :tahoe (macOS 26, the floor mygo.json declares)`],
    ),
    caskCase(
      "a cask whose macOS floor is a codename the table does not know",
      "depends_on macos: :tahoe",
      "depends_on macos: :el_capitan",
      [`${CASK_TEMPLATE_PATH}: depends_on macos: :el_capitan against :tahoe (macOS 26, the floor mygo.json declares)`],
    ),
    caskCase(
      "a cask built for another architecture",
      "depends_on arch: :arm64",
      "depends_on arch: :x86_64",
      [`${CASK_TEMPLATE_PATH}: depends_on arch: must be :arm64; the release carries one arm64 dmg`],
    ),
    caskCase(
      "a cask whose livecheck names url :stable",
      "url :url",
      "url :stable",
      [`${CASK_TEMPLATE_PATH}: livecheck must be a block holding url :url and strategy :github_latest; url :stable is Formula-only and GithubLatest must be named`],
    ),
    caskCase(
      "a cask whose livecheck drops the strategy",
      "    strategy :github_latest\n",
      "",
      [`${CASK_TEMPLATE_PATH}: livecheck must be a block holding url :url and strategy :github_latest`],
    ),
    caskCase(
      "a cask whose zap is missing the data directory",
      ZAP_BLOCK,
      [
        "  zap trash: [",
        '    "~/Library/Caches/com.github.wxxb789.passquest",',
        '    "~/Library/WebKit/com.github.wxxb789.passquest",',
        "  ]",
      ].join("\n"),
      [`${CASK_TEMPLATE_PATH}: zap trash: must carry the data directory the install guides name: "~/Library/Application Support/Pass Quest"`],
    ),
    caskCase(
      "a cask whose zap holds two paths in one string",
      '    "~/Library/Application Support/Pass Quest",',
      '    "~/Library/Application Support/Pass Quest,~/Library/Caches/com.github.wxxb789.passquest",',
      [
        `${CASK_TEMPLATE_PATH}: zap trash: must carry the data directory the install guides name`,
        "holds several paths in one string; Homebrew resolves each directive value as one literal path, so the paths must be separate strings",
      ],
    ),
    caskCase(
      "a cask whose xattr runs in a step that is not postflight_steps",
      "postflight_steps do",
      "postinstall_steps do",
      [`${CASK_TEMPLATE_PATH}: postflight_steps must clear com.apple.quarantine; Homebrew quarantines the download, so without it the first launch meets Gatekeeper`],
    ),
    caskCase(
      "a cask whose xattr clears the wrong attribute",
      '"com.apple.quarantine"',
      '"com.apple.quarantined"',
      [`${CASK_TEMPLATE_PATH}: postflight_steps must name com.apple.quarantine`],
    ),
    caskCase(
      "a cask whose xattr step lost the install-time token",
      'args: ["-rd", "com.apple.quarantine", "{{appdir}}/Pass Quest.app"]',
      'args: ["-rd", "com.apple.quarantine", "/Applications/Pass Quest.app"]',
      [`${CASK_TEMPLATE_PATH}: the xattr run's args must be ["-rd", "com.apple.quarantine", "{{appdir}}/Pass Quest.app"]`],
    ),
    caskCase(
      "a cask whose xattr failure aborts the install",
      "        must_succeed: false",
      "        must_succeed: true",
      [`${CASK_TEMPLATE_PATH}: run must pass must_succeed: false: xattr exits non-zero when the attribute is already absent, the state this step wants`],
    ),
    caskCase(
      "a cask that does not quit the app on uninstall",
      '  uninstall quit: "com.github.wxxb789.passquest"\n',
      "",
      [`${CASK_TEMPLATE_PATH}: uninstall quit: must be com.github.wxxb789.passquest, the identifier the app registers under`],
    ),
    caskCase(
      "a template edit the stanza checks cannot see",
      "  caveats <<~EOS\n",
      "  # A stub line the stanza checks cannot see.\n  caveats <<~EOS\n",
      [`${CASK_GOLDEN_PATH}: the template no longer renders the pinned v0.15.0 cask`, "re-render the golden and review the diff"],
    ),
    caskCase(
      "a cask that lost the app's name line",
      '  name "Pass Quest"\n',
      "",
      [`${CASK_GOLDEN_PATH}: the template no longer renders the pinned v0.15.0 cask`],
    ),
    {
      gate: installGuide,
      label: "a cask whose xattr step no longer deletes the attribute",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        const patched = patchCopy(root, CASK_TEMPLATE_PATH, 'args: ["-rd", "com.apple.quarantine', 'args: ["-r", "com.apple.quarantine');
        if (patched) return patched;
        // The golden is re-rendered from the edit, so only the semantic check
        // can see the dropped d: -r lists the attribute instead of deleting it.
        const template = readFileSync(resolve(root, CASK_TEMPLATE_PATH), "utf8");
        writeFileSync(resolve(root, CASK_GOLDEN_PATH), renderCask(template, CASK_VERSION, CASK_SHA256));
        return null;
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${CASK_TEMPLATE_PATH}: the xattr run's args must be ["-rd", "com.apple.quarantine", "{{appdir}}/Pass Quest.app"]`,
        ]),
    },
    {
      gate: installGuide,
      label: "a cask whose version is hard-coded and whose golden was re-rendered",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        const patched = patchCopy(root, CASK_TEMPLATE_PATH, 'version "@VERSION@"', 'version "0.15.0"');
        if (patched) return patched;
        // The pinned render still matches the facts (0.15.0 is the release this
        // cask was built from) and the golden is re-rendered from the edit, so
        // only the synthetic render can see the version no longer travels.
        const template = readFileSync(resolve(root, CASK_TEMPLATE_PATH), "utf8");
        writeFileSync(resolve(root, CASK_GOLDEN_PATH), renderCask(template, CASK_VERSION, CASK_SHA256));
        return null;
      },
      postFail: (failures) => expectedLines(failures, [`${CASK_TEMPLATE_PATH}: the version line must carry @VERSION@`]),
    },
  ];
}

export function installGuidePassCases(): PassCase[] {
  return [
    guidePassCase("the real install guide pair", () => null, (detail) =>
      detail.includes("7 screenshot places") &&
      detail.includes("the cask template renders v0.15.0 with the release's SHA-256") &&
      detail.includes("floor 26.0 from mygo.json")
        ? null
        : `the pass line must count the screenshot places, name the cask render and the floor; got: ${detail}`),
    guidePassCase("the tap cask with the zap paths in the array form brew generate-zap emits", (root) => {
      // `brew generate-zap` emits `format_directive`'s own indentation
      // (dev-cmd/generate-zap.rb: seven spaces per item, five for the closing
      // bracket), which is not the tree's — the parser reads entries, not a
      // layout, so a pasted stanza passes.
      const generated = [
        "  zap trash: [",
        '       "~/Library/Application Support/Pass Quest",',
        '       "~/Library/Caches/com.github.wxxb789.passquest",',
        '       "~/Library/WebKit/com.github.wxxb789.passquest",',
        "     ]",
      ].join("\n");
      const patched = patchCopy(root, CASK_TEMPLATE_PATH, ZAP_BLOCK, generated);
      if (patched) return patched;
      // The edited template is a new render, so the golden moves with it — the
      // workflow the gate's mismatch message names for any accepted edit.
      const template = readFileSync(resolve(root, CASK_TEMPLATE_PATH), "utf8");
      writeFileSync(resolve(root, CASK_GOLDEN_PATH), renderCask(template, CASK_VERSION, CASK_SHA256));
      return null;
    }),
    guidePassCase("the tap cask with the zap paths in a one-line array", (root) => {
      const oneLine = [
        '  zap trash: ["~/Library/Application Support/Pass Quest", "~/Library/Caches/com.github.wxxb789.passquest", "~/Library/WebKit/com.github.wxxb789.passquest"]',
        '  # "a, b" after the array stays a comment, not a second zap entry.',
      ].join("\n");
      const patched = patchCopy(root, CASK_TEMPLATE_PATH, ZAP_BLOCK, oneLine);
      if (patched) return patched;
      const template = readFileSync(resolve(root, CASK_TEMPLATE_PATH), "utf8");
      writeFileSync(resolve(root, CASK_GOLDEN_PATH), renderCask(template, CASK_VERSION, CASK_SHA256));
      return null;
    }),
    guidePassCase("the tap cask with the xattr flags written -dr", (root) => {
      const patched = patchCopy(root, CASK_TEMPLATE_PATH, 'args: ["-rd", "com.apple.quarantine', 'args: ["-dr", "com.apple.quarantine');
      if (patched) return patched;
      const template = readFileSync(resolve(root, CASK_TEMPLATE_PATH), "utf8");
      writeFileSync(resolve(root, CASK_GOLDEN_PATH), renderCask(template, CASK_VERSION, CASK_SHA256));
      return null;
    }),
  ];
}
