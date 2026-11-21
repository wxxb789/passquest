import { createHash } from "node:crypto";

/**
 * The SHA-256 of a string's UTF-8 bytes or of a byte array, as lowercase hex.
 * One implementation for the three digests the harness prints or verifies: the
 * cask tools' render line, the provider stub's request record and the pinned
 * runtime download.
 */
export function sha256Hex(data: string | Uint8Array): string {
  return createHash("sha256").update(data).digest("hex");
}
