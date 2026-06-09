# miniclaw — feature inventory

A living list of every feature shipped, partially shipped, or planned. Each entry is intentionally short: what it does, where it lives, how we'd extend it, and the traps we've already found (or will). Update this when scope shifts — it's the source of truth for "what does this app do".

Legend: **S** = shipped · **P** = partial · **T** = to-do.

---

## 1. Accounts & ingest

### 1.1 IMAP/SMTP accounts (S)
- IMAP read pass + SMTP send. Code: `internal/services/email/imap.go`, `smtp.go`.
- Folder allowlist per account (CSV), default `INBOX`.
- Stores password in OS keychain (`internal/services/keychain`).
- Gotchas:
  - go-imap v2 `UIDSet.AddNum` writes one UID per range — fine for hundreds, audit before 10k+ msg pulls.
  - `parseMessage` falls back to dumping raw RFC822 as plaintext if `mail.CreateReader` fails (quirky servers). Don't pipe that into the summariser without truncating.
  - Search criteria with both `Since` and `UID` ranges narrow the intersection — be explicit about which one is the watermark on each pass.

### 1.2 Gmail OAuth (S)
- REST sync via `gmailoauth.Syncer.Sync` + `BackfillBefore` + `SyncNow`.
- Walks `payload.parts` for inline images (Content-ID) and attachments (Filename), hydrates oversize parts via `attachments.get`, stores via `internal/services/attachments`.
- Persists every `labelIds` entry into `email_labels` for parity with IMAP flags — Gmail's native labels (CATEGORY_*, IMPORTANT, STARRED, user labels) are no longer discarded.
- Incremental sync uses `users.history.list` once `accounts.gmail_history_id` is seeded; falls back to the date-bounded `in:inbox after:` query when the cursor is empty or has expired (404), then reseeds from `users.profile.historyId`.
- `MatchesBlock` runs on every Gmail message before upsert, matching the IMAP path; first-seen senders register with the screener through `Triage.RegisterSender`.
- Threading headers (`In-Reply-To`, `References`) are captured and pushed through the shared `email.DeriveThreadID` so Gmail-side threading agrees with IMAP rather than relying on Gmail's `threadId`.
- Still open:
  - Gmail returns `Message-ID` with angle brackets; we store as-is and rely on the UNIQUE index to dedupe — verify on accounts that auto-rewrite Message-ID.

### 1.3 Microsoft OAuth (P)
- `internal/services/msoauth/sync.go` exists and is wired but parity work mirrors Gmail above.
- Gotcha: Graph API paginates differently than Gmail; reuse the iterator pattern, not the watermark date.

### 1.4 IMAP IDLE push (S)
- One IDLE connection per IMAP account in `internal/services/email/idle.go`.
- Gotchas:
  - IDLE drops after ~29 min on most servers — keep the auto-reconnect backoff; never assume the connection survives a laptop sleep.
  - Don't share the IDLE client with the polling syncer; they conflict on `SELECT`.

### 1.5 Scheduler (S)
- `internal/scheduler` ticks per-account on `sync_cadence_secs`. Picks IMAP / Gmail / MS by `auth_kind`.
- Gotcha: cadence < 60s on Gmail will get you rate-limited; cap at 60s in UI.

### 1.6 Sync controls UI (S)
- `IMAPSync.SyncNow` / `IMAPSync.BackfillBefore` / `GmailOAuth.SyncNow` / `GmailOAuth.BackfillBefore` exposed to JS.
- `sync_progress` event registered in `main.go` fires per account (`start` / `done` / `error`); top bar shows a live pill.
- Still open: per-account progress (count of items remaining) inside a long backfill — today's event is per-account, not per-message.

---

## 2. Email model

### 2.1 Storage schema (S)
- `emails`, `summaries`, `senders`, `accounts`, `workspaces` in SQLite. Schema in `internal/db/migrations/0001_init.sql`.
- FTS5 virtual table over `subject + body_plain`.
- Gotcha: `body_html` is already stored but not exposed by `inbox.Service` — frontend only ever sees `body_plain`, which is why images and formatting are invisible.

### 2.2 Inbox read API (S)
- `Get`, `ListByWorkspace`, `ListByAccount`, `ListOlderByWorkspace`, `Search`, `MarkRead`, `MarkUnread` ship.
- Search hits `emails_fts` directly; user input is quoted as a phrase to keep multi-word queries working.
- Still open: `ListByCategory(workspaceID, category)` for the category drill-downs.

### 2.3 Threading (S)
- Migration 0005 adds `in_reply_to`, `email_refs`, `thread_id` to `emails` plus a partial index on `thread_id`.
- `email.DeriveThreadID` picks the first `References` token, then `In-Reply-To`, then the row's own `Message-ID`. Both the IMAP and Gmail syncers wire it; Gmail's syncer takes the deriver as a function so the package stays free of circular imports.
- `ListEmailsByThread` is exposed via sqlc for the reader pane's collapse-by-root flow.
- Gotcha: Gmail's `threadId` ≠ RFC References — we deliberately ignore it and treat the RFC chain as the source of truth.
- Still open: the inbox list itself doesn't collapse threads yet; rendering still happens row-per-message.

### 2.4 Attachments + inline images (S)
- Schema: `email_attachments(email_id, content_id, filename, mime_type, size_bytes, data BLOB, is_inline)` (migration 0004).
- Gmail sync walks parts, hydrates deferred bodies via `attachments.get`, writes via `attachments.Service.Store`.
- IMAP sync now does the same: `parseMessage` walks `mail.NextPart`, collects inline (Content-ID) and `AttachmentHeader` parts, and pushes them through the shared `AttachmentStore` interface. RFC 2047 encoded-word filenames are decoded.
- Reader splices bytes as `data:` URLs inside the sandboxed iframe srcdoc — same-origin policy + the Wails asset handler don't play well with `cid:` rewrites, so inlining sidesteps the whole problem.
- Still open:
  - File attachments (non-image, non-inline) are stored but no UI surfaces them as downloads yet.
  - Remote-image policy is per-message ("Show images" toggle); per-sender allowlist hasn't landed.

### 2.5 HTML sanitization (T)
- Sanitize server-side with `bluemonday` (Go) — strip `<script>`, `<iframe>`, JS event attrs, `javascript:` URLs.
- Render inside a sandboxed `<iframe srcdoc>` to keep email CSS from leaking into the app's Tailwind tokens.
- Gotcha: Outlook conditional comments and `mso-` styles look broken but are intentional — don't filter them, they're scoped.

---

## 3. Triage & summarization

### 3.1 Per-email summary (S)
- `internal/services/summary` calls Ollama `/api/generate`. Stores summary + `needs_attention` flag.
- Empty `response` from Ollama is now an explicit error (was silently breaking JSON decode). Summariser logs the first failure fully, swallows the rest into a count line, bails after 3 consecutive empties, and retries once without JSON mode for small instruct models that reject `format=json`.
- Last error is stored on `ollama.Service.LastError()` and surfaces in `Ollama.Status` for the UI banner.
- Gotcha: defensive JSON extraction — small models often wrap output in prose; `extractJSON` pulls the first balanced `{...}` block.

### 3.2 Periodic rundown (P)
- `internal/services/digest` emits a workspace-level summary on cadence; routes to Telegram.
- Missing: in-app "today's rundown" view (card on the dashboard).

### 3.3 Categories (S)
- Heuristic classifier in `internal/services/categories`: maps `List-Unsubscribe` + known sender domains → promotions/social/updates/newsletter.
- Gmail categories override (`gmailoauth/sync.go::categoryFromLabels`) when present.

### 3.4 Screener (S)
- `senders` table with `screener_state`. UI: `views/ScreenerView.jsx`.
- Gotcha: first-seen senders default `unscreened`; block decisions are remembered forever — expose an undo in the UI.

### 3.5 Filters / block rules (S)
- `internal/services/triage` + `views/FiltersView.jsx`.
- Both syncers consult `MatchesBlock` before upsert: IMAP in `imap.go`, Gmail via the `Triage` interface wired with `gmailSync.AttachTriage(triageSvc)` in `main.go`. `RegisterSender` runs on both paths so the screener catches first-seen senders from any provider.

### 3.6 Put aside (S)
- One-bit `is_put_aside` flag. Toggled from inbox row. UI: `views/PutAsideView.jsx`.

### 3.7 Snooze (S)
- Migration 0005 adds `emails.snoozed_until` plus a partial index. Inbox queries hide rows whose snooze hasn't fired.
- `internal/services/snooze` exposes `Snooze`/`Unsnooze`/`ListSnoozed`. A 60s ticker (`snoozeSvc.Start`) clears due rows, then fires `telegram.NotifySnoozeWake` for the ones that have a `needs_attention` summary so cold newsletters wake silently.
- Reader UI: shadcn `DropdownMenu` with Later today / Tomorrow / Next week / Next month / Wake now. The dedicated **Snoozed** nav view lists everything pending with a "wake now" button.

### 3.8 Compose / reply (P)
- `SMTPSender.Send` ships. Plain-text only, quoted body.
- Missing: rich-text composer, draft persistence, attachment upload, reply-all, forward.

---

## 4. Delivery surfaces

### 4.1 Telegram (S)
- `internal/services/telegram` posts the periodic rundown + attention pings.
- Gotcha: bot token in keychain; never log it. Multi-account: one bot, one chat per workspace.

### 4.2 In-app notifications (T)
- Wails `application.SystemTray` or native notification on `needs_attention` arrival.
- Gotcha: macOS will silently drop notifications if Focus mode is on; mirror to the status bar as a fallback.

---

## 5. Calendar (P)

- Schema and service shipped (migration 0005, `internal/services/planner/CalendarService`). Local-first time-blocking with `kind` enum (block / meeting / focus) and a nullable `google_event_id`.
- UI: `views/PlannerView.jsx` ▶ Calendar tab — create, list, delete, and "Promote" stub that sets `google_event_id = "pending:<id>"`.
- Promotion path against the real Google Calendar API is the remaining work; today Promote is a placeholder so we can iterate on the UI without touching OAuth scopes.
- Gotchas:
  - Time zones are the entire problem. Store UTC, render in user-local, never split.
  - Google's `recurringEventId` is its own world — start single-event only, add recurrence later.

---

## 6. Todos + notes (S)

- Tables: `todos(id, workspace_id, text, done, due_at, sort_order, …)` and `notes(id, workspace_id, title, body_md, …)` in migration 0005.
- Services: `planner.TodosService` and `planner.NotesService`. Standard CRUD plus a fast `SetDone` path for the row checkbox.
- UI: tabs alongside the calendar inside `PlannerView` — markdown notes use a list/editor split; todos sort open-first by `sort_order`.

---

## 7. Frontend shell

### 7.1 Design system (P)
- Tokens from `DESIGN.md` (Linear-style dark canvas) wired into `frontend/src/index.css` via Tailwind v4 `@theme`.
- Primitives sourced from shadcn (`components.json` configured) — don't hand-roll buttons, dialogs, sidebars, etc.
- Lucide for icons; Linear-style monochrome only.

### 7.2 App shell (S)
- Three pane: workspace strip → folder/account sidebar → message list → reader.
- Command palette (⌘K / Ctrl-K) opens a shadcn Dialog with quick actions + FTS5 results.
- Top bar shows Ollama health, keychain status, live sync pill, sync-now button, settings gear.
- Still open: account "last synced" surfacing in the side rail, system tray icon.

### 7.3 Inbox view (S)
- HTML body inside sandboxed `<iframe srcdoc>` (allow-same-origin only). Reader has a shadcn `Tabs` toggle for Rendered / Plain text. Inline images splice from the attachments table as data URLs.
- "Load older" walks the cache; "Fetch 200 older from server" calls `BackfillBefore` for both Gmail and IMAP.
- List swaps to `@tanstack/react-virtual` once row count crosses 200 (`VIRTUALIZE_AT` in `InboxView.jsx`); shorter lists stay on the plain mapped layout to keep small-list interactions snappy.
- Reader has a `DropdownMenu`-driven Snooze control (Later today / Tomorrow / Next week / Next month / Wake now).
- Still open: thread collapse in the list.

### 7.4 Settings (P)
- Sections rendered as shadcn `Card` + `Tabs` + `Input` + `Label`. Add-account flow uses `Tabs` to switch IMAP / Gmail / Microsoft.
- Destructive flows (delete workspace, remove account) gated by shadcn `AlertDialog` instead of `window.confirm`. The wrapper lives in `SettingsView.jsx` as `DestructiveConfirm`.
- Missing: image-loading policy, sync cadence picker per account, "default workspace for new account".

### 7.5 Onboarding (T)
- First-run: pick model from `Ollama.ListModels`, pick default workspace, add first account.

---

## 8. Cross-cutting

### 8.1 Migrations (S)
- Goose-style up files in `internal/db/migrations`. Applied on boot.
- Rule: never edit a shipped migration — add a new one.

### 8.2 Secrets (S)
- All secrets via `internal/services/keychain` → OS keychain. Nothing on disk in plaintext.
- Gotcha: the dev app and prod app bundle ID must match the keychain access group on macOS — use `make assets` to keep them in sync.

### 8.3 Logging / observability (T)
- Switch ad-hoc `fmt.Printf` to `slog` with structured fields (`account_id`, `folder`, `op`).
- Forward warnings (sync errors, IDLE drops) to the status bar event stream.

### 8.4 Tests (P)
- Unit coverage on triage, summary, categories. Gmail/MS sync covered by fakes in `*_test.go`.
- Missing: IMAP IDLE happy-path test; frontend snapshot or integration tests (none yet).

### 8.5 Build / dev (S)
- `make dev` rebuilds bindings and frees port 9245; `make assets` regenerates app metadata; `make stop` kills the dev process.
- Gotcha: `wails generate bindings` after every Go service change — frontend imports break silently otherwise.

---

## Out of scope (deliberate)

- Multi-device sync, server-side storage.
- Mobile targets (scaffolding only).
- Calendar providers beyond Google.
- Webhooks / external automation triggers.
