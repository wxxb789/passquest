/**
 * What a self-test case is, and the fixture helpers more than one gate's cases share. Each gate's cases live
 * beside this file in <gate module>.cases.ts; self-test.ts runs them in a fixed order.
 */
import { mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";

import { resolve } from "../../lib/repo.ts";
import type { Gate } from "../../lib/types.ts";

export const GO_MOD = "module example.com/selftest\n\ngo 1.27.3\n";

export interface BrokenCase {
  gate: Gate;
  label: string;
  files: Record<string, string>;
  tracked?: string[];
  /** Runs after the fixture files exist and before the gate (usually git state). */
  prepare?: (root: string) => Promise<string | null>;
  postFail?: (failures: string[]) => string | null;
}

/** A fixture the gate must accept; proves a check is not unconditionally red. */
export interface PassCase {
  gate: Gate;
  label: string;
  files: Record<string, string>;
  tracked?: string[];
  prepare?: (root: string) => Promise<string | null>;
  postPass?: (detail: string) => string | null;
}

/** Product sources a smoke gate builds and drives from the repository root. */
export const NOTE_SMOKE_SOURCES = ["go.mod", "go.sum", "cmd", "internal", "agent"];
/** desktop-ux also bundles the frontend, reads its allowlist and compares the app version the row shows with mygo.json. */
export const DESKTOP_UX_SOURCES = [...NOTE_SMOKE_SOURCES, "desktop", "harness/ux-allow.json", "mygo.json"];

function copyTree(from: string, to: string): number {
  let copied = 0;
  for (const entry of readdirSync(from, { withFileTypes: true })) {
    const source = path.join(from, entry.name);
    const target = path.join(to, entry.name);
    if (entry.isDirectory()) {
      mkdirSync(target, { recursive: true });
      copied += copyTree(source, target);
      continue;
    }
    if (!entry.isFile()) continue;
    writeFileSync(target, readFileSync(source));
    copied++;
  }
  return copied;
}

/** Copies working-tree product paths into the fixture root; returns the number of files. */
export function copyProduct(root: string, relatives: string[]): number {
  const repo = path.resolve(import.meta.dir, "..", "..", "..");
  let copied = 0;
  for (const relative of relatives) {
    const source = resolve(repo, relative);
    const target = resolve(root, relative);
    if (statSync(source).isDirectory()) {
      mkdirSync(target, { recursive: true });
      copied += copyTree(source, target);
      continue;
    }
    mkdirSync(path.dirname(target), { recursive: true });
    writeFileSync(target, readFileSync(source));
    copied++;
  }
  return copied;
}

/** Replaces the one occurrence of `anchor` in a copied product file. */
export function patchCopy(root: string, relative: string, anchor: string, replacement: string): string | null {
  const file = resolve(root, relative);
  const source = readFileSync(file, "utf8");
  const count = source.split(anchor).length - 1;
  if (count !== 1) return `the anchor appears ${count} time(s) in ${relative}`;
  writeFileSync(file, source.replace(anchor, () => replacement));
  return null;
}
