import { readFileSync, rmSync, writeFileSync } from "node:fs";

import { resolve } from "../../lib/repo.ts";

import { APP_SOURCES, CHINESE_GUIDE, ENGLISH_GUIDE, installGuide, RELEASE_NOTES, TAP_COMMAND } from "../install-guide.ts";

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
  ...APP_SOURCES,
];

/** The first expected fact that the gate's failure lines do not name. */
function expectedLines(failures: string[], expected: string[]): string | null {
  for (const line of expected) {
    if (!failures.some((failure) => failure.includes(line))) {
      return `expected the failure to name ${JSON.stringify(line)}; got: ${failures.join("; ") || "none"}`;
    }
  }
  return null;
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
        return patchCopy(root, ENGLISH_GUIDE, "the packages declare macOS 26 as their minimum", "the packages declare macOS 24 as their minimum");
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
        expectedLines(failures, [`${ENGLISH_GUIDE}:51: the screenshot marker "windows-smartscreen" must be followed by an italic caption line`]),
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
        detail.includes("7 screenshot places") && detail.includes("floor 26.0 from mygo.json")
          ? null
          : `the pass line must count the screenshot places and name the floor; got: ${detail}`,
    },
  ];
}
