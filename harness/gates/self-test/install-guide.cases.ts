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

export function installGuideCases(): BrokenCase[] {
  return [
    {
      gate: installGuide,
      label: "the Open Anyway step moved out of the macOS section",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        // The wording stays in the file, in the wrong section, and no longer names
        // the button: only a fact pinned inside its own section can see that.
        const step = patchCopy(
          root,
          ENGLISH_GUIDE,
          "choose **Open Anyway** — macOS offers the button for about an hour after the attempt.",
          "choose the button macOS offers.",
        );
        if (step) return step;
        const caption = patchCopy(root, ENGLISH_GUIDE, "the blocked-app notice and the Open Anyway button", "the blocked-app notice and the button beside it");
        if (caption) return caption;
        return patchCopy(root, ENGLISH_GUIDE, "## Linux\n", "## Linux\n\n**Open Anyway** is the button in System Settings on macOS.\n");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${ENGLISH_GUIDE}: the "macOS" section must carry the Open Anyway flow under Privacy & Security: missing "Open Anyway"`,
        ]),
    },
    {
      gate: installGuide,
      label: "the guide naming a macOS pair below the declared floor",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        const pair = patchCopy(root, ENGLISH_GUIDE, "macOS 26 and macOS 27, the two latest majors", "macOS 24 and macOS 25, the two latest majors");
        if (pair) return pair;
        // The rest of the section is brought to the same (wrong) floor, so the fixture is one guide written for macOS 24.
        const floor = patchCopy(root, ENGLISH_GUIDE, "the packages declare macOS 26 as their minimum", "the packages declare macOS 24 as their minimum");
        if (floor) return floor;
        return patchCopy(
          root,
          ENGLISH_GUIDE,
          "lists macOS 27 Golden Gate and macOS\nTahoe 26 as the latest versions",
          "lists macOS 24 and macOS 25 as the latest versions",
        );
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${ENGLISH_GUIDE}: names macOS 24, below the supported floor of macOS 26`,
          `${ENGLISH_GUIDE}: the "Supported systems" section names macOS majors [24, 25]; mygo.json declares 26.0, so the supported pair is 26 and 27`,
        ]),
    },
    {
      gate: installGuide,
      label: "release notes without the Homebrew line",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, RELEASE_NOTES, TAP_COMMAND, "brew install --cask pass-quest");
      },
      postFail: (failures) => expectedLines(failures, [`${RELEASE_NOTES}: must name the Homebrew command ${JSON.stringify(TAP_COMMAND)}`]),
    },
    {
      gate: installGuide,
      label: "the Chinese guide dropping a screenshot place",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, CHINESE_GUIDE, "<!-- screenshot: macos-open-anyway-confirm -->\n", "");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${CHINESE_GUIDE}: the "macOS" section's screenshot places [macos-blocked-dialog, macos-privacy-security, macos-first-launch, macos-homebrew-install, macos-xattr-terminal] must match ${ENGLISH_GUIDE}'s [macos-blocked-dialog, macos-privacy-security, macos-open-anyway-confirm, macos-first-launch, macos-homebrew-install, macos-xattr-terminal]`,
        ]),
    },
    {
      gate: installGuide,
      label: "the Chinese guide running a different command",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(
          root,
          CHINESE_GUIDE,
          'xattr -dr com.apple.quarantine "/Applications/Pass Quest.app"',
          "xattr -dr com.apple.quarantine /Applications/Pass Quest.app",
        );
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${CHINESE_GUIDE}: code block 2 differs from ${ENGLISH_GUIDE}'s`,
          `${CHINESE_GUIDE}: the "macOS" section must carry the quarantine alternative: missing "xattr -dr com.apple.quarantine`,
        ]),
    },
    {
      gate: installGuide,
      label: "the guide naming a system-wide Gatekeeper disabling",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(
          root,
          ENGLISH_GUIDE,
          "Do not disable Gatekeeper system-wide to get past the warning.",
          "If the warning persists, run sudo spctl --master-disable and try again.",
        );
      },
      postFail: (failures) =>
        expectedLines(failures, [`${ENGLISH_GUIDE}: names "spctl --master-disable", which disables Gatekeeper system-wide; the guide must not suggest that`]),
    },
    {
      gate: installGuide,
      label: "a README that does not link its own guide",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, "README.zh-CN.md", "[安装指南](docs/install.zh-CN.md)", "[安装指南](docs/install.md)");
      },
      postFail: (failures) => expectedLines(failures, [`README.zh-CN.md: must link ${CHINESE_GUIDE}`]),
    },
    {
      gate: installGuide,
      label: "the Chinese guide dropping the zh-CN README link",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, CHINESE_GUIDE, "](../README.zh-CN.md)", "](../README.md)");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${CHINESE_GUIDE}: must link ../README.zh-CN.md, which ${ENGLISH_GUIDE} links`,
          `${CHINESE_GUIDE}: links ../README.md; ${ENGLISH_GUIDE} does not, so the link exists on one side only`,
        ]),
    },
    {
      gate: installGuide,
      label: "the English guide's section skeleton changed",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, ENGLISH_GUIDE, "\n## Upgrade\n", "\n## Upgrading\n");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `the "##" sections of ${ENGLISH_GUIDE} must be exactly [Supported systems | Windows | macOS | Linux | Where your data lives, and how to remove it | Upgrade | What CI proves, and what is untested]; they are [Supported systems | Windows | macOS | Linux | Where your data lives, and how to remove it | Upgrading | What CI proves, and what is untested]`,
        ]),
    },
    {
      gate: installGuide,
      label: "the Chinese guide's heading levels no longer match",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, CHINESE_GUIDE, "\n## Linux\n", "\n### Linux\n");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${CHINESE_GUIDE}: the headings must be the same in the same order as ${ENGLISH_GUIDE}: its levels are [1,2,2,2,3,2,2,2] against [1,2,2,2,2,2,2,2]`,
        ]),
    },
    {
      gate: installGuide,
      label: "the Chinese guide missing from the tree",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        rmSync(resolve(root, CHINESE_GUIDE));
        return null;
      },
      postFail: (failures) => expectedLines(failures, [`${CHINESE_GUIDE}: missing or unreadable`]),
    },
    {
      gate: installGuide,
      label: "a screenshot marker without its italic caption",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(
          root,
          ENGLISH_GUIDE,
          '<!-- screenshot: windows-smartscreen -->\n*Screenshot: the SmartScreen "Windows protected your PC" dialog and its **More info** link (owner).*\n',
          "<!-- screenshot: windows-smartscreen -->\n",
        );
      },
      postFail: (failures) =>
        expectedLines(failures, [`${ENGLISH_GUIDE}:53: the screenshot marker "windows-smartscreen" must be followed by an italic caption line`]),
    },
    {
      gate: installGuide,
      label: "the English guide gaining a code block the Chinese guide lacks",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(
          root,
          ENGLISH_GUIDE,
          "CI itself runs the AppImage that way and never mounts it with FUSE.",
          "```bash\nls\n```\n\nCI itself runs the AppImage that way and never mounts it with FUSE.",
        );
      },
      postFail: (failures) =>
        expectedLines(failures, [`${CHINESE_GUIDE}: 4 fenced code blocks against ${ENGLISH_GUIDE}'s 5; the guides must run the same commands`]),
    },
    {
      gate: installGuide,
      label: "an external URL on the English side only",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(
          root,
          ENGLISH_GUIDE,
          "installing the packaged app; what the app does is described in the [project README](../README.md).",
          "installing the packaged app, or through [the walkthrough](https://example.com/only-english); what the app does is described in the [project README](../README.md).",
        );
      },
      postFail: (failures) =>
        expectedLines(failures, [`the guides must carry the same external links: https://example.com/only-english is in ${ENGLISH_GUIDE} only`]),
    },
    {
      gate: installGuide,
      label: "the English guide not linking its Chinese counterpart",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, ENGLISH_GUIDE, "[简体中文](install.zh-CN.md)", "[简体中文](install.md)");
      },
      postFail: (failures) => expectedLines(failures, [`${ENGLISH_GUIDE}: must link its Chinese counterpart install.zh-CN.md`]),
    },
    {
      gate: installGuide,
      label: "the release notes dropping the Chinese guide link",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, RELEASE_NOTES, " ([简体中文](https://github.com/wxxb789/passquest/blob/{{TAG}}/docs/install.zh-CN.md))", "");
      },
      postFail: (failures) => expectedLines(failures, [`${RELEASE_NOTES}: must link ${CHINESE_GUIDE}`]),
    },
    {
      gate: installGuide,
      label: "a guide still naming macOS Sonoma",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, ENGLISH_GUIDE, "## Supported systems\n", "## Supported systems\n\nmacOS Sonoma is not supported.\n");
      },
      postFail: (failures) => expectedLines(failures, [`${ENGLISH_GUIDE}: names macOS Sonoma, below the supported floor`]),
    },
    {
      gate: installGuide,
      label: "an unparsable mygo.json floor",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, "mygo.json", '"minimumSystemVersion": "26.0"', '"minimumSystemVersion": "tahoe"');
      },
      postFail: (failures) =>
        expectedLines(failures, [`mygo.json: macos.minimumSystemVersion must be a version like "26.0"; it is "tahoe"`]),
    },
    {
      gate: installGuide,
      label: "the manifest declaring another bundle identifier",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, "mygo.json", '"identifier": "com.github.wxxb789.passquest"', '"identifier": "com.github.wxxb789.pass-quest"');
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `mygo.json: identifier must be com.github.wxxb789.passquest, the bundle id the app registers under and the cask's uninstall quit: names; it is "com.github.wxxb789.pass-quest"`,
        ]),
    },
    {
      gate: installGuide,
      label: "the macOS section back to the removed right-click → Open step",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(
          root,
          ENGLISH_GUIDE,
          "1. Open **Pass Quest** from **Applications** and let macOS refuse it.",
          "1. Right-click **Pass Quest** in **Applications**, choose **Open**, and confirm the dialog.",
        );
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${ENGLISH_GUIDE}: the "macOS" section still offers the removed right-click → Open path (macOS 26 and 27 do not have it)`,
        ]),
    },
    {
      gate: installGuide,
      label: "the macOS section losing every screenshot place",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        // Both guides lose every marker, so the per-section minimum is the only check that can see it.
        for (const guide of [ENGLISH_GUIDE, CHINESE_GUIDE]) {
          const file = resolve(root, guide);
          const kept = readFileSync(file, "utf8").split("\n").filter((line) => !line.startsWith("<!-- screenshot:"));
          writeFileSync(file, kept.join("\n"));
        }
        return null;
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${ENGLISH_GUIDE}: the "macOS" section must keep at least one screenshot place; it has none`,
          `${CHINESE_GUIDE}: the "macOS" section must keep at least one screenshot place; it has none`,
        ]),
    },
    {
      gate: installGuide,
      label: "the Chinese guide paraphrasing an app string",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, CHINESE_GUIDE, "数据来自更新的 Pass Quest", "数据来自更新版本的 Pass Quest");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${CHINESE_GUIDE}: the "Upgrade" section must quote the notice for a database a newer build wrote as the app shows it: missing "数据来自更新的 Pass Quest"`,
        ]),
    },
    {
      gate: installGuide,
      label: "the app rewording a string the guides quote",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, "desktop/web/src/core/i18n.ts", "'数据来自更新的 Pass Quest'", "'数据来自更新版本的 Pass Quest'");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `desktop/web/src/core/i18n.ts: no longer holds the notice for a database a newer build wrote: "数据来自更新的 Pass Quest"`,
        ]),
    },
    {
      gate: installGuide,
      label: "the guide naming macOS 13",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, ENGLISH_GUIDE, "Any macOS older than 26\nis unsupported", "Any macOS 13 or older\nis unsupported");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${ENGLISH_GUIDE}: names macOS 13, below the supported floor of macOS 26`,
          `${ENGLISH_GUIDE}: the "Supported systems" section names macOS majors [13, 26, 27]; mygo.json declares 26.0, so the supported pair is 26 and 27`,
        ]),
    },
    {
      gate: installGuide,
      label: "the Chinese guide not linking its English counterpart",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, CHINESE_GUIDE, "[English](install.md)", "[English](install.zh-CN.md)");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `${CHINESE_GUIDE}: must link its English counterpart install.md`,
          `${CHINESE_GUIDE}: links install.zh-CN.md; ${ENGLISH_GUIDE} does not, so the link exists on one side only`,
        ]),
    },
    {
      gate: installGuide,
      label: "the release notes naming a macOS release below the floor",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, RELEASE_NOTES, "`SHA256SUMS.txt` lists each file's SHA-256.", "`SHA256SUMS.txt` lists each file's SHA-256. macOS Sonoma is not supported.");
      },
      postFail: (failures) =>
        expectedLines(failures, [`${RELEASE_NOTES}: names macOS Sonoma, below the supported floor`]),
    },
    {
      gate: installGuide,
      label: "the app's name declaration gone",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return patchCopy(root, "internal/desktop/paths.go", 'AppName       = "Pass Quest"', "AppName       = appDisplayName");
      },
      postFail: (failures) =>
        expectedLines(failures, [
          `internal/desktop/paths.go: no AppName = "..." declaration, so the strings the guides quote cannot be checked against the app`,
        ]),
    },
    {
      gate: installGuide,
      label: "an app source the guides quote from missing",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        rmSync(resolve(root, "desktop/web/src/views/settings.ts"));
        return null;
      },
      postFail: (failures) => expectedLines(failures, [`desktop/web/src/views/settings.ts: missing or unreadable`]),
    },
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
      'zap trash: "~/Library/Application Support/Pass Quest"',
      'zap trash: "~/Library/Caches/Pass Quest"',
      [`${CASK_TEMPLATE_PATH}: zap trash: must carry the data directory the install guides name: "~/Library/Application Support/Pass Quest"`],
    ),
    caskCase(
      "a cask whose zap holds two paths in one string",
      'zap trash: "~/Library/Application Support/Pass Quest"',
      'zap trash: "~/Library/Application Support/Pass Quest,~/Library/Caches/com.github.wxxb789.passquest"',
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
    {
      gate: installGuide,
      label: "the real install guide pair",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        return null;
      },
      postPass: (detail) =>
        detail.includes("7 screenshot places") &&
        detail.includes("the cask template renders v0.15.0 with the release's SHA-256") &&
        detail.includes("floor 26.0 from mygo.json")
          ? null
          : `the pass line must count the screenshot places, name the cask render and the floor; got: ${detail}`,
    },
    {
      gate: installGuide,
      label: "the tap cask with the zap paths in the array form brew generate-zap emits",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        const array = [
          "  zap trash: [",
          '    "~/Library/Application Support/Pass Quest",',
          '    "~/Library/Caches/com.github.wxxb789.passquest",',
          "  ]",
        ].join("\n");
        const patched = patchCopy(root, CASK_TEMPLATE_PATH, '  zap trash: "~/Library/Application Support/Pass Quest"', array);
        if (patched) return patched;
        // The edited template is a new render, so the golden moves with it — the
        // workflow the gate's mismatch message names, and the shape P4 appends
        // its measured paths in.
        const template = readFileSync(resolve(root, CASK_TEMPLATE_PATH), "utf8");
        writeFileSync(resolve(root, CASK_GOLDEN_PATH), renderCask(template, CASK_VERSION, CASK_SHA256));
        return null;
      },
    },
    {
      gate: installGuide,
      label: "the tap cask with the zap paths in a one-line array",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        const oneLine = [
          '  zap trash: ["~/Library/Application Support/Pass Quest", "~/Library/Caches/com.github.wxxb789.passquest"]',
          '  # "a, b" after the array stays a comment, not a second zap entry.',
        ].join("\n");
        const patched = patchCopy(root, CASK_TEMPLATE_PATH, '  zap trash: "~/Library/Application Support/Pass Quest"', oneLine);
        if (patched) return patched;
        const template = readFileSync(resolve(root, CASK_TEMPLATE_PATH), "utf8");
        writeFileSync(resolve(root, CASK_GOLDEN_PATH), renderCask(template, CASK_VERSION, CASK_SHA256));
        return null;
      },
    },
    {
      gate: installGuide,
      label: "the tap cask with the xattr flags written -dr",
      files: {},
      prepare: async (root) => {
        copyProduct(root, GUIDE_DOCUMENTS);
        const patched = patchCopy(root, CASK_TEMPLATE_PATH, 'args: ["-rd", "com.apple.quarantine', 'args: ["-dr", "com.apple.quarantine');
        if (patched) return patched;
        const template = readFileSync(resolve(root, CASK_TEMPLATE_PATH), "utf8");
        writeFileSync(resolve(root, CASK_GOLDEN_PATH), renderCask(template, CASK_VERSION, CASK_SHA256));
        return null;
      },
    },
  ];
}
