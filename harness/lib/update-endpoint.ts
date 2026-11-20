/**
 * The constant endpoint of goal 0015's release check and the prefix every
 * release link is built from. One definition, two readers: the local-only
 * gate pins the http(s) literals of `internal/update` against these exact
 * strings, and the update stub (lib/update-stub.ts) refuses to stand in for
 * anything else. Both comparisons are exact, so a drifted host or path is a
 * named failure, never a silently accepted request.
 */
export const UPDATE_ENDPOINT = "https://api.github.com/repos/wxxb789/passquest/releases/latest";
export const UPDATE_RELEASES_PREFIX = "https://github.com/wxxb789/passquest/releases/";
