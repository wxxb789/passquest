import { existsSync } from "node:fs";
import path from "node:path";

import { readText, resolve } from "./repo.ts";

const LINK = /!?\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g;

function decode(target: string): string {
  try {
    return decodeURIComponent(target);
  } catch {
    return target;
  }
}

/**
 * Checks relative markdown links and images against the filesystem. External
 * URLs and anchors are ignored; the count is the number of local targets.
 */
export async function checkMarkdownLinks(root: string, tracked: string[]): Promise<{ checked: number; broken: string[] }> {
  const markdown = tracked.filter((file) => file.endsWith(".md"));
  let checked = 0;
  const broken: string[] = [];
  for (const file of markdown) {
    let text: string;
    try {
      text = readText(root, file);
    } catch {
      broken.push(`${file}: unreadable`);
      continue;
    }
    const directory = path.dirname(resolve(root, file));
    for (const match of text.matchAll(LINK)) {
      let target = decode(match[1]).split("#", 1)[0];
      if (!target || target.includes("://") || /^(mailto|tel|data):/.test(target)) continue;
      checked++;
      const absolute = target.startsWith("/")
        ? path.join(root, target.slice(1))
        : path.resolve(directory, target);
      if (!existsSync(absolute)) {
        broken.push(`${file}: broken link -> ${match[1]}`);
      }
    }
  }
  return { checked, broken };
}
