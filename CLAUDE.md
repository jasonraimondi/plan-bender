- **IMPORTANT**: At the start of every session working with local code, call `mcp__plugin_serena_serena__activate_project` with the project root before doing any other work.
- **IMPORTANT**: Prefer Serena's JetBrains-powered semantic tools (`jet_brains_find_symbol`, `jet_brains_get_symbols_overview`, `jet_brains_find_referencing_symbols`, `jet_brains_type_hierarchy`) for navigating code. Use Serena's mutation tools (`replace_symbol_body`, `insert_after_symbol`, `rename_symbol`) for refactoring. Prefer these over reading entire files when possible.

# Code
- Never use raw Date in business logic. Use Temporal API (TC39), or isolate Date behind a swappable abstraction — especially for formatting, arithmetic, and timezone handling.
- Functional core, imperative shell. Service functions are pure logic returning `Result<T, DomainError>`. Side effects (queue dispatch, session ops) happen in tRPC procedures and adapters. DomainError extends Error (instanceof for tRPC error mapping).

# Tests
- No tautological tests. Assert non-trivial outcomes on real logic paths. If deleting the implementation doesn't fail the test, delete the test.

# Git
- Do NOT batch multiple unrelated changes into one commit. Prefer small, frequent commits.
- Always use `--no-verify` when committing.
- When working in parallel with other sessions, use a git worktree to avoid conflicts.
- When git committing or creating a PR, never add Co-Authored-By or generated with

# Info
- When reporting information to me be extremely concise and sacrifice grammar for concision


- Adapter pattern for prod 3rd-party APIs. I/O translation only — no business logic in adapters. Prototypes exempt.
- Result type: `{ ok: true; data: T } | { ok: false; error: DomainError }`. Constructors: `ok(data)`, `err(error)`. tRPC procedures unwrap via `unwrapResult()` — returns data or throws mapped TRPCError. `safeDispatch` is the one intentional exception to "no swallowed errors" (logs at warn, never throws).
- Environment variables via [varlock](https://github.com/dmno-dev/varlock). Apps access env with `import { ENV } from "varlock/env"` — never raw `process.env`. `.env.schema` files (using `@env-spec` decorators: `@required`, `@sensitive`, `@type=`) are the single source of truth for variable documentation, defaults, and validation. Root `.env.schema` holds shared vars; per-app schemas `@import()` the root and add app-specific vars. Packages never import varlock — they receive config through factory function parameters. Mark secrets with `@sensitive` for automatic log redaction.
