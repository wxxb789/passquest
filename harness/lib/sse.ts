/**
 * One OpenAI-compatible SSE chat stream for the loopback stubs. Both the agent
 * and provider smokes receive the same wire shape, so a change to it is made
 * once here, and the durable study brief's three-answer handshake lives here
 * for the same reason: the agent smoke's matrix and the package smoke's brief
 * case drive it against one stub apiece.
 */

export function sseText(text: string, model: string, id = "chatcmpl-smoke"): Response {
  const base = { id, object: "chat.completion.chunk", created: 1, model };
  const body = [
    { ...base, choices: [{ index: 0, delta: { role: "assistant", content: text }, finish_reason: null }] },
    { ...base, choices: [{ index: 0, delta: {}, finish_reason: "stop" }] },
  ]
    .map((chunk) => `data: ${JSON.stringify(chunk)}\n\n`)
    .join("");
  return new Response(`${body}data: [DONE]\n\n`, { headers: { "content-type": "text/event-stream" } });
}

export function sseTool(
  index: number,
  kind: string,
  title: string,
  body: string,
  model: string,
  id = "chatcmpl-smoke",
): Response {
  const base = { id, object: "chat.completion.chunk", created: 1, model };
  const call = {
    index: 0,
    id: `call_${index}`,
    type: "function",
    function: { name: "record_study_step", arguments: JSON.stringify({ index, kind, title, body }) },
  };
  const chunks = [
    { ...base, choices: [{ index: 0, delta: { role: "assistant", tool_calls: [call] }, finish_reason: null }] },
    { ...base, choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }] },
  ];
  return new Response(`${chunks.map((chunk) => `data: ${JSON.stringify(chunk)}\n\n`).join("")}data: [DONE]\n\n`, {
    headers: { "content-type": "text/event-stream" },
  });
}

/** One chat message of an OpenAI-compatible request body, as far as a stub reads it. */
export interface StubMessage {
  role?: string;
  content?: unknown;
}

/** The messages array of a parsed chat-completions body; empty when the body is malformed. */
export function stubMessages(body: unknown): StubMessage[] {
  const messages = (body as { messages?: unknown } | null | undefined)?.messages;
  return Array.isArray(messages) ? (messages as StubMessage[]) : [];
}

/**
 * The three answers the durable study brief's loopback stub gives, keyed by the
 * tool results already in the conversation: step 1, step 2, then the final text.
 * The agent smoke's matrix and the package smoke's brief case drive the same
 * handshake, so it lives once. `label` names the run in the recorded titles —
 * the run id's tail — so two runs on one stub stay tellable apart.
 */
export function briefAnswer(messages: readonly StubMessage[], label: string, model: string): Response {
  const toolText = messages
    .filter((message) => message.role === "tool")
    .map((message) => String(message.content ?? ""))
    .join("\n");
  if (!/step 1/i.test(toolText)) {
    return sseTool(1, "diagnosis", `Step 1 diagnosis ${label}`, "The recorded outcome suggests marking the negation before comparing options.", model);
  }
  if (!/step 2/i.test(toolText)) {
    return sseTool(2, "tactic", `Step 2 tactic ${label}`, "Mark NOT or EXCEPT first; a word match alone is not evidence.", model);
  }
  return sseText("Two advisory steps recorded; practise the focused tactic next.", model);
}
