/** Shared baseline constants and text rules for the baseline gate, its self-test and the package content scan. */

export const BASELINE = "data/baselines/2026-crsc-english";
/**
 * The earlier 22-page native-text extraction of the same internal-only PDF:
 * the second private-text source the package content scan turns into
 * needles, its path in one place beside the 2026 baseline's.
 */
export const BASELINE_PARTIAL = "data/baselines/2025-crsc-english-partial.txt";
export const EXPECTED_PAGES = 165;
export const EXPECTED_NATIVE = 22;
export const EXPECTED_OCR = 143;

// source.CountNonSpace in Go skips unicode.IsSpace plus a literal form feed.
// JavaScript's \s is not the same set: it also treats U+FEFF as whitespace,
// while Go's unicode.White_Space property (and this pattern) does not.
const WHITE_SPACE = /^\p{White_Space}$/u;

export function nonSpaceChars(text: string): number {
  let count = 0;
  for (const char of text) {
    if (char === "\f" || WHITE_SPACE.test(char)) continue;
    count++;
  }
  return count;
}

/** Strict UTF-8 decode: invalid bytes are null, a literal U+FFFD is valid text. */
export function decodeUTF8(bytes: Buffer): string | null {
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}
