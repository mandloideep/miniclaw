# miniclaw

A Wails v3 desktop app that uses a local Ollama model to summarize email across multiple accounts, surface what actually needs attention, and let the user act on it from one place. SQLite is the local store.

## Product scope

- **Email aggregation**: connect N email accounts. Organize them into user-created workspaces (defaults: Family, Work, Personal, Other; user can add more).
- **Summarization & triage**: Ollama runs locally and produces per-email summaries plus a periodic rundown of the inbox. Emails that need attention (replies, deadlines, decisions) are prioritized.
- **Two delivery surfaces**:
  - The Wails web UI for reading, replying, and managing.
  - Telegram for periodic rundowns and important pings.
- **Calendar**: in-app calendar with time-blocking. Time blocks stay local; only the meetings the user explicitly promotes sync to the live calendar (Google/etc.).
- **Todo + Notes**: lightweight per-workspace todos and notes.
- **Storage**: SQLite for everything (accounts, emails cache, summaries, workspaces, todos, notes, calendar blocks).

## Hard requirements (non-negotiable)

### Tooling
- **Node**: always pinned via `frontend/.nvmrc`. Never install or run frontend tooling outside the pinned version.
- **Package manager**: **npm only**. No yarn, no pnpm, no bun. Lockfile is `package-lock.json`.
- **Format before commit**: run the frontend formatter (Prettier via the project's configured script) and `gofmt`/`goimports` on Go before every commit. No unformatted code lands.

### Commit hygiene
- **Commit cadence**: after each small completed feature. Not as-you-go, not one giant end-of-day dump.
- **Messages**: plain, authentic, human-sounding. Imperative mood, lowercase or sentence case, no marketing language.
- **Forbidden in commits, PRs, or any artifact**:
  - `Co-Authored-By: Claude` / any Claude / Anthropic attribution
  - "Authored by Claude Code", "Generated with Claude", or similar
  - Emojis in commit messages
  - AI-disclosure trailers of any kind
- Author must look like a normal human developer's commit. If a commit message would not pass as written by the user, rewrite it.

### Wails docs
- This is **Wails v3**. Always consult v3 docs, never v2.
- Canonical entry point: https://v3.wails.io/concepts/architecture/
- Fetch additional v3 pages as needed (bindings, services, events, build system, mobile targets). Do not guess v3 APIs from v2 memory.

## Skills to use (when relevant)

Pick from these based on what's being built. Decide which ones apply *before* writing the plan; don't blanket-load all of them every turn.

### Frontend
- `/frontend-design` — for creating distinctive, production-grade UI
- `/web-design-guidelines` — accessibility and UX review of UI code
- `/vercel-composition-patterns` — React composition / API design
- `/vercel-react-best-practices` — React + Next-era performance patterns
- `/shadcn` — when adding or composing shadcn/ui components
- `/tailwind-design-system` — design tokens, component library structure
- `/typescript-advanced-types` — when writing nontrivial type logic

### Go (Wails backend / services)
- `cc-skills-golang` plugin — pick the right sub-skill for the task:
  - `golang-project-layout`, `golang-naming`, `golang-code-style` when starting or restructuring
  - `golang-error-handling`, `golang-safety`, `golang-concurrency`, `golang-context` when writing service logic
  - `golang-database` for SQLite access patterns
  - `golang-testing`, `golang-stretchr-testify` for tests
  - `golang-dependency-injection`, `golang-structs-interfaces`, `golang-design-patterns` for architecture
  - `golang-observability`, `golang-troubleshooting` for runtime issues
  - `golang-security` for anything touching credentials, OAuth tokens, IMAP/SMTP, file I/O
  - Others as the situation calls for them

### After implementation
- `/code-simplifier` — pass over the diff once the feature works
- LSP tools for diagnostics
- `/golangci-lint` (via `cc-skills-golang:golang-linter`) before commit on Go changes
- **Fallow** (TS/JS static analysis) for the frontend — use the `fallow` MCP tools to find dead code, duplication, complexity hotspots, unused deps, and unresolved imports before finishing a feature.
  - Install: `npm install -g fallow` (provides both `fallow` CLI and `fallow-mcp`). Re-install under every Node version `.nvmrc` switches to — npm globals are per-Node-version under nvm.
  - Config lives at `frontend/.fallowrc.json`. Generated dirs (`bindings/`, `dist/`, `src/components/ui/`) are excluded — keep that list in sync when new generated paths appear.
  - When the MCP server is loaded, prefer the typed tools (`analyze`, `find_dupes`, `check_health`, etc.) over running the CLI directly.

## Architecture notes (working assumptions, refine as we go)

- **Process model**: Wails v3 services for backend logic (email sync, Ollama client, Telegram bot, calendar). UI calls them via Wails bindings.
- **Email fetch**: IMAP for reads, SMTP for sends; OAuth where the provider requires it (Gmail). Store tokens encrypted at rest.
- **Ollama**: local HTTP client to `http://localhost:11434`. Model choice configurable per workspace; default to a small instruct model.
- **Scheduler**: in-process ticker for periodic sync + summarization passes. Configurable cadence per workspace.
- **DB migrations**: versioned SQL files, applied on startup.
- **Secrets**: never commit `.env`, OAuth client secrets, or API tokens. Use OS keychain via a Go service for runtime storage.

## Out of scope (for now)

- Multi-user / multi-device sync
- Mobile builds (the scaffolding is there but not a priority)
- Calendar providers beyond Google
