/**
 * Narrow readers for untyped JSON documents. Gate assertions treat a protocol
 * document as untrusted input: a missing or wrongly typed field is an assertion
 * failure, not an exception.
 */
export function field(value: unknown, key: string): unknown {
  if (value === null || typeof value !== "object" || Array.isArray(value)) return undefined;
  return Reflect.get(value, key);
}

export function stringField(value: unknown, key: string): string | undefined {
  const child = field(value, key);
  return typeof child === "string" ? child : undefined;
}

export function numberField(value: unknown, key: string): number | undefined {
  const child = field(value, key);
  return typeof child === "number" ? child : undefined;
}

export function arrayField(value: unknown, key: string): unknown[] {
  const child = field(value, key);
  return Array.isArray(child) ? child : [];
}

/** Lists the keys that appear anywhere in a document, recursively. */
function collectKeys(value: unknown, keys: Set<string>): void {
  if (Array.isArray(value)) {
    for (const child of value) collectKeys(child, keys);
    return;
  }
  if (value !== null && typeof value === "object") {
    for (const [key, child] of Object.entries(value)) {
      keys.add(key);
      collectKeys(child, keys);
    }
  }
}

/** Returns the forbidden keys present anywhere in a document, in input order. */
export function exposedKeys(value: unknown, forbidden: readonly string[]): string[] {
  const keys = new Set<string>();
  collectKeys(value, keys);
  return forbidden.filter((key) => keys.has(key));
}

/** Protocol keys that must never appear before an answer is recorded. */
export const FORBIDDEN_ANSWER_KEYS = [
  "answer",
  "correct_label",
  "correct_text",
  "evidence",
  "reasoning",
  "status",
  "feedback",
] as const;

/**
 * Reports one assertion per forbidden key. Gates share the key list so their
 * key-hiding checks cannot drift apart.
 */
export function checkKeyHiding(
  check: (condition: boolean, message: string) => boolean,
  doc: unknown,
  where: string,
  forbidden: readonly string[] = FORBIDDEN_ANSWER_KEYS,
): void {
  const exposed = exposedKeys(doc, forbidden);
  for (const key of forbidden) {
    check(!exposed.includes(key), `${where} exposes ${key}`);
  }
}
