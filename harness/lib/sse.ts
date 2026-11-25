/**
 * One OpenAI-compatible SSE stream for the loopback stubs. Both the agent
 * and provider smokes receive the same wire shape, so a change to it is made
 * once here, and the durable study brief's three-answer handshake lives here
 * for the same reason: the agent smoke's matrix and the package smoke's brief
 * case drive it against one stub apiece. The Responses form is the same
 * handshake over the events pi-ai's own adapter requires: response.created,
 * the output item's added/delta/done events and a terminal response.completed
 * that carries the usage counts.
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

/** One data-only SSE frame of a Responses stream, as the OpenAI SDK parses them. */
function responsesFrame(type: string, payload: Record<string, unknown>): string {
  return `data: ${JSON.stringify({ type, ...payload })}\n\n`;
}

/** The usage a stubbed Responses answer reports: the same counts `chatReply` answers with. */
const RESPONSES_USAGE = { input_tokens: 10, output_tokens: 5, total_tokens: 15 };

/** One Responses text answer: the message item with its deltas, then the terminal event with usage. */
export function sseResponsesText(text: string, model: string, id = "resp-smoke"): Response {
  const itemId = "msg-smoke";
  const item = { type: "message", id: itemId, role: "assistant", status: "completed", content: [{ type: "output_text", text, annotations: [] }] };
  const frames = [
    responsesFrame("response.created", { response: { id, model, status: "in_progress" } }),
    responsesFrame("response.output_item.added", { output_index: 0, item: { type: "message", id: itemId, role: "assistant", content: [] } }),
    responsesFrame("response.output_text.delta", { output_index: 0, item_id: itemId, delta: text }),
    responsesFrame("response.output_text.done", { output_index: 0, item_id: itemId, text }),
    responsesFrame("response.output_item.done", { output_index: 0, item }),
    responsesFrame("response.completed", { response: { id, model, status: "completed", output: [item], usage: RESPONSES_USAGE } }),
  ];
  return new Response(frames.join(""), { headers: { "content-type": "text/event-stream" } });
}

/** One Responses tool call, the same `record_study_step` call the chat stub asks for. */
export function sseResponsesTool(
  index: number,
  kind: string,
  title: string,
  body: string,
  model: string,
  id = "resp-smoke",
): Response {
  const callId = `call_${index}`;
  const itemId = `fc_${index}`;
  const args = JSON.stringify({ index, kind, title, body });
  const item = { type: "function_call", id: itemId, call_id: callId, name: "record_study_step", arguments: args, status: "completed" };
  const frames = [
    responsesFrame("response.created", { response: { id, model, status: "in_progress" } }),
    responsesFrame("response.output_item.added", { output_index: 0, item: { type: "function_call", id: itemId, call_id: callId, name: "record_study_step", arguments: "" } }),
    responsesFrame("response.function_call_arguments.delta", { output_index: 0, item_id: itemId, delta: args }),
    responsesFrame("response.function_call_arguments.done", { output_index: 0, item_id: itemId, arguments: args }),
    responsesFrame("response.output_item.done", { output_index: 0, item }),
    responsesFrame("response.completed", { response: { id, model, status: "completed", output: [item], usage: RESPONSES_USAGE } }),
  ];
  return new Response(frames.join(""), { headers: { "content-type": "text/event-stream" } });
}

/** One chat message of an OpenAI-compatible request body, as far as a stub reads it. */
interface StubMessage {
  role?: string;
  content?: unknown;
}

/** The messages array of a parsed chat-completions body; empty when the body is malformed. */
export function stubMessages(body: unknown): StubMessage[] {
  const messages = (body as { messages?: unknown } | null | undefined)?.messages;
  return Array.isArray(messages) ? (messages as StubMessage[]) : [];
}

/** One input item of a Responses request body, as far as a stub reads it. */
interface StubResponsesItem {
  type?: string;
  content?: unknown;
  output?: unknown;
}

/** The input array of a parsed Responses body; empty when the body is malformed. */
export function stubResponsesInput(body: unknown): StubResponsesItem[] {
  const input = (body as { input?: unknown } | null | undefined)?.input;
  return Array.isArray(input) ? (input as StubResponsesItem[]) : [];
}

function contentText(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content.map((part) => String((part as { text?: unknown } | null | undefined)?.text ?? "")).join("");
}

/**
 * Every text one Responses request carries — instruction and message content
 * (a string or `input_text`/`output_text` parts, the shapes the adapter
 * sends) and tool output — so a stub can read a prompt the way
 * `stubMessages` lets the chat stubs read one.
 */
export function stubResponsesText(body: unknown): string {
  return stubResponsesInput(body)
    .map((item) => (item.type === "function_call_output" ? String(item.output ?? "") : contentText(item.content)))
    .join("\n");
}

/** The tool results a Responses request already carries, the twin of the chat stub's `role: "tool"` messages. */
export function stubResponsesToolResults(body: unknown): string[] {
  return stubResponsesInput(body)
    .filter((item) => item.type === "function_call_output")
    .map((item) => String(item.output ?? ""));
}

/** One step of the brief handshake the stub asks for next, or null once two are recorded. */
function briefStep(toolText: string, label: string): { index: number; kind: string; title: string; body: string } | null {
  if (!/step 1/i.test(toolText)) {
    return {
      index: 1,
      kind: "diagnosis",
      title: `Step 1 diagnosis ${label}`,
      body: "The recorded outcome suggests marking the negation before comparing options.",
    };
  }
  if (!/step 2/i.test(toolText)) {
    return {
      index: 2,
      kind: "tactic",
      title: `Step 2 tactic ${label}`,
      body: "Mark NOT or EXCEPT first; a word match alone is not evidence.",
    };
  }
  return null;
}

const BRIEF_FINAL_TEXT = "Two advisory steps recorded; practise the focused tactic next.";

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
  const step = briefStep(toolText, label);
  return step ? sseTool(step.index, step.kind, step.title, step.body, model) : sseText(BRIEF_FINAL_TEXT, model);
}

/** `briefAnswer`'s Responses twin: the same handshake over the `function_call_output` items. */
export function briefResponsesAnswer(body: unknown, label: string, model: string): Response {
  const step = briefStep(stubResponsesToolResults(body).join("\n"), label);
  return step
    ? sseResponsesTool(step.index, step.kind, step.title, step.body, model)
    : sseResponsesText(BRIEF_FINAL_TEXT, model);
}
