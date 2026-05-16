<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.

<!-- END:nextjs-agent-rules -->

# Working in this project

## Package manager: pnpm

This project uses **pnpm**, not npm or yarn. Use `pnpm <script>` and `pnpm add` / `pnpm add -D`. Don't commit `package-lock.json` or `yarn.lock`.

## Scripts

| Command             | Purpose                                                                                                                                                                              |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `pnpm dev`          | Start the Next dev server with HMR (default port 3000). Check `lsof -iTCP:3000` before starting a new one — HMR picks up changes automatically, so a restart is almost never needed. |
| `pnpm build`        | Production build. Run this to catch type errors before pushing if your change is non-trivial.                                                                                        |
| `pnpm lint`         | Run ESLint.                                                                                                                                                                          |
| `pnpm format`       | Prettier write across the repo.                                                                                                                                                      |
| `pnpm format:check` | Prettier check (CI-style). Use this in scripts.                                                                                                                                      |

## Format after every change

**After any code edit (TS/TSX/JS/JSON/CSS/MD), run `pnpm format` before committing.** This keeps diffs free of cosmetic noise and matches the `.prettierrc` style used by the rest of the repo. If you're touching one file, you can scope it: `pnpm exec prettier --write path/to/file.tsx`.

## Firestore indexes

The production Go code (in the parent `oncetask` package) relies on a set of composite indexes already provisioned for its query shapes. The admin UI has two views:

- **Browse** (`OnceTaskList`): free-form filtering, may prompt new indexes when filter combinations require them. Indexes are surfaced via clickable links in the error banner.
- **Index-aware** (`IndexedView`): every query matches a shape the production Go code already indexes. Should never prompt a new index.

When adding new server-side queries, prefer matching an existing Go query shape (`oncetask/firestore_queries.go`) so no new indexes are required.
