/**
 * One OpenAI-compatible SSE chat stream for the loopback stubs. Both the agent
 * and provider smokes receive the same wire shape, so a change to it is made
 * once here.
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
