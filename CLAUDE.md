- **CRITICAL**: At the start of every session working with local code, call `mcp__plugin_serena_serena__activate_project` with the project root before doing any other work.
- **CRITICAL**: Prefer Serena's JetBrains-powered semantic tools (`jet_brains_find_symbol`, `jet_brains_get_symbols_overview`, `jet_brains_find_referencing_symbols`, `jet_brains_type_hierarchy`) for navigating code. Use Serena's mutation tools (`replace_symbol_body`, `insert_after_symbol`, `rename_symbol`) for refactoring. Prefer these over reading entire files when possible.

# Code
- Adapter pattern for prod 3rd-party APIs. I/O translation only — no business logic in adapters. Prototypes exempt.
- **IMPORTANT**: Be idiomatic to the language and framework

# Tests
- No tautological tests. Assert non-trivial outcomes on real logic paths. If deleting the implementation doesn't fail the test, delete the test.

# Git
- Do NOT batch multiple unrelated changes into one commit. Prefer small, frequent commits.
- When working in parallel
- with other sessions, use a git worktree to avoid conflicts.
- When git committing or creating a PR, never add Co-Authored-By or generated with

# Info
- When reporting information to me be extremely concise and sacrifice grammar for concision
