# oncetask-admin

A standalone Next.js admin UI for the [`oncetask`](https://github.com/anish749/oncetask) Go library. Points at a Firestore database, lists tasks from the `onceTasks` collection, and exposes Reset / Cancel / Delete mutations that mirror the library's `Manager` semantics.

## Architecture

```
Browser ─HTTP→ Next.js API routes ─@google-cloud/firestore→ Firestore
                       ↑
                 ADC / service account
```

No Firebase SDK on the client. The Next.js server holds the Firestore credential and performs all reads/writes. The browser only talks to local API routes.

## Quick start

```bash
cp .env.example .env.local
# edit GOOGLE_CLOUD_PROJECT, ONCE_TASK_ENV, ONCETASK_LINKS_CONFIG
gcloud auth application-default login
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Configuration

| Env var | Purpose |
|---|---|
| `GOOGLE_CLOUD_PROJECT` | Firestore project ID (required) |
| `ONCE_TASK_ENV` | Task environment this instance is scoped to (default `DEFAULT`) |
| `ONCETASK_LINKS_CONFIG` | Path to YAML file declaring outbound links (optional) |

## Outbound links

The admin has no hardcoded "View Logs" button — outbound links are declared in a YAML file and discovered at runtime. See `links.example.yaml` for the schema. A link renders when:

1. Every entry in its `requires` list resolves to a non-empty value on the task.
2. Every `{placeholder}` in its `url` resolves to a non-empty value (auto URL-encoded).

This covers View Logs, Gmail thread links, conversation deep-links, and similar deep-link patterns without code changes.

## Authentication

This app ships with **no built-in authentication**. Deploy it behind a reverse proxy (Caddy `forward_auth`, nginx `auth_request` + oauth2-proxy, Cloudflare Access, Google IAP, Tailscale, etc.) or on a private network.

Mutation endpoints (`POST /api/tasks/[id]/reset`, `cancel`, `DELETE /api/tasks/[id]`) all validate that the target task belongs to `ONCE_TASK_ENV` before writing — accidentally pointing at the wrong env is caught server-side.

## API surface

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/tasks` | List tasks with `status`, `type`, `env`, `resourceKey`, `limit` query params |
| `GET` | `/api/tasks/[id]` | Fetch one task |
| `POST` | `/api/tasks/[id]/reset` | Reset a terminal-state task |
| `POST` | `/api/tasks/[id]/cancel` | Cancel a non-done task |
| `DELETE` | `/api/tasks/[id]` | Permanently delete a task |
| `GET` | `/api/metadata` | Discover task types + environments via skip-scan |
| `GET` | `/api/links` | Parsed link config (for client-side rendering) |

## Relationship to the library

Mutation logic in `src/lib/oncetask/mutations.ts` mirrors `pkg/oncetask/reset.go` and `pkg/oncetask/cancellation.go` field-for-field. If the Go library changes the wire schema, this app needs to track that — there is no automatic synchronization.
