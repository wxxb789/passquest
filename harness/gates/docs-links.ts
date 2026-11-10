import { checkMarkdownLinks } from "../lib/links.ts";
import { fail, pass } from "../lib/types.ts";
import type { Gate } from "../lib/types.ts";

export const docsLinks: Gate = {
  id: "docs-links",
  title: "docs links",
  async run(ctx) {
    const { checked, broken } = await checkMarkdownLinks(ctx.root, await ctx.workingTreeFiles());
    if (broken.length > 0) {
      return fail([...broken, `${broken.length} broken of ${checked} checked`]);
    }
    return pass(`${checked} checked, 0 broken`);
  },
};
