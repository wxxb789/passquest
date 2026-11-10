# Agent host

`tutor-host.mjs` is the first Node component in the product. It exists for one
reason: the Go application core must not import a network client, while a BYOK
text provider requires HTTPS. The host is a single-shot process, not an agent
loop yet.

## Contract

- One JSON request on stdin, one JSON result on stdout, protocol version `1`.
- The credential is read only from `PASSQUEST_TUTOR_API_KEY`. It is placed in the
  `Authorization: Bearer …` header and nowhere else. The process logs nothing.
- Redirects are refused; `https` is required except for loopback endpoints.
- Responses are capped at 1 MiB and the returned text at the request bound.
  A provider failure is a structured `status: "error"` document with a stable
  code; the host does not throw or retry.
- The host has no database, filesystem or tool access. It cannot change an
  attempt, grade or answer key.

`docs/tutor.md` documents the CLI, the consent model and the exact request
fields. Goal 0006 will replace this per-call process with the Pi Durable
runtime behind the same versioned boundary; this file is intentionally small
and dependency-free so it runs on the pinned Node version without a build step.
