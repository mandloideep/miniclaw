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

### 1.2 Gmail OAuth (S — needs parity)
- REST sync via `gmailoauth.Syncer.Sync` using `messages.list` + `messages.get?format=full`.
- Gotchas / open work:
  - Only fetches `text/plain` + `text/html`; inline images (CID parts), attachments, and remote-image data are dropped → frontend can't render rich mail.
  - No "fetch older" path — the watermark is `after:<date>`, never `before:`. Need a `Backfill(accountID, beforeDate)` that walks `before:` pages until quota or user stop.
  - `LabelIds` only mapped to four categories — Gmail's native labels (CATEGORY_PERSONAL, IMPORTANT, STARRED, user labels) are discarded.
  - No incremental `historyId` cursor — every pass re-lists. Cheap at small scale, will burn quota at large.
  - Gmail returns `Message-ID` with angle brackets; we store as-is and rely on the UNIQUE index to dedupe — verify on accounts that auto-rewrite Message-ID.
- Build steps for parity:
  - Walk `payload.parts` for `image/*` parts with `Content-ID` and store as blobs (or a side table `email_attachments`) keyed by `(email_id, cid)`.
  - Add `gmailoauth.Syncer.Backfill(ctx, accountID, before time.Time, max int)`.
  - Add `gmailoauth.Syncer.SyncNow(ctx, accountID)` as a thin frontend wrapper around Sync that returns `{written, since}` to give the UI a status toast.
  - Persist Gmail labels in a `email_labels(email_id, label)` table; mirror IMAP's `\Flagged`/`\Seen` paths.
  - Implement `users.history.list` once initial sync is done to swap polling for delta sync.

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

### 1.6 Sync controls UI (T)
- Need: "Sync now" button per account, "Fetch older — last 30 days" button, sync-status indicator in the status bar.
- Build: thin wrappers in `gmailoauth.Service` / `msoauth.Service` / `email.IMAPSyncer` callable from JS; emit `application.RegisterEvent[SyncProgress]` so the UI can subscribe.

---

## 2. Email model

### 2.1 Storage schema (S)
- `emails`, `summaries`, `senders`, `accounts`, `workspaces` in SQLite. Schema in `internal/db/migrations/0001_init.sql`.
- FTS5 virtual table over `subject + body_plain`.
- Gotcha: `body_html` is already stored but not exposed by `inbox.Service` — frontend only ever sees `body_plain`, which is why images and formatting are invisible.

### 2.2 Inbox read API (P)
- `inbox.Service.ListByWorkspace`, `ListByAccount`, `MarkRead`, `MarkUnread` ship.
- Missing:
  - `GetEmail(id)` returning the full record incl. `body_html`, `to`, `cc`, headers, attachments.
  - `ListSince(accountID, beforeReceivedAt, limit)` for infinite-scroll back in time.
  - `Search(q, workspaceID)` against `emails_fts`.
  - `ListByCategory(workspaceID, category)`.

### 2.3 Threading (T)
- We index by `Message-ID` only. No `In-Reply-To`/`References` walk.
- Build: add columns `in_reply_to`, `references` and a derived `thread_id` (`COALESCE(References.split[0], Message-ID)`), with an index. Frontend collapses replies under the root.
- Gotcha: Gmail's `threadId` ≠ RFC References — use ours, treat Gmail's as a sync hint only.

### 2.4 Attachments + inline images (T)
- Schema: `email_attachments(id, email_id, content_id, filename, mime, size, blob | path)`.
- Frontend rendering: rewrite `<img src="cid:abc">` → `wails:///attachments/<id>` served via a Wails asset handler.
- Remote images: gate behind a user setting ("load remote images: never / per-sender / always"); inject a placeholder otherwise. Strips most tracking pixels for free.

### 2.5 HTML sanitization (T)
- Sanitize server-side with `bluemonday` (Go) — strip `<script>`, `<iframe>`, JS event attrs, `javascript:` URLs.
- Render inside a sandboxed `<iframe srcdoc>` to keep email CSS from leaking into the app's Tailwind tokens.
- Gotcha: Outlook conditional comments and `mso-` styles look broken but are intentional — don't filter them, they're scoped.

---

## 3. Triage & summarization

### 3.1 Per-email summary (S)
- `internal/services/summary` calls Ollama `/api/generate`. Stores summary + `needs_attention` flag.
- Gotcha: model picks attention via a JSON-shaped prompt — defensive parse, log the raw response on parse failure.

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
- IMAP sync consults `MatchesBlock` before upsert; **Gmail sync doesn't yet** — wire the same check in `gmailoauth/sync.go` before parity ships.

### 3.6 Put aside (S)
- One-bit `is_put_aside` flag. Toggled from inbox row. UI: `views/PutAsideView.jsx`.

### 3.7 Snooze (T)
- Extend put-aside with `snoozed_until TEXT`. A daily ticker un-snoozes; emit a Telegram nudge if `needs_attention`.

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

## 5. Calendar (T)

- Local time-blocking only; Google sync only for explicitly promoted blocks.
- Schema: `calendar_blocks(id, workspace_id, title, start, end, kind, google_event_id NULLABLE)`.
- Promotion path: write block → call Google Calendar API → store `google_event_id` → 2-way sync only for that row.
- Gotchas:
  - Time zones are the entire problem. Store UTC, render in user-local, never split.
  - Google's `recurringEventId` is its own world — start single-event only, add recurrence later.

---

## 6. Todos + notes (T)

- Lightweight per-workspace. Tables: `todos(id, workspace_id, text, done, due, sort)`, `notes(id, workspace_id, title, body_md, updated_at)`.
- Frontend: a single right-side panel toggle on the inbox view, not its own tab.

---

## 7. Frontend shell

### 7.1 Design system (P)
- Tokens from `DESIGN.md` (Linear-style dark canvas) wired into `frontend/src/index.css` via Tailwind v4 `@theme`.
- Primitives sourced from shadcn (`components.json` configured) — don't hand-roll buttons, dialogs, sidebars, etc.
- Lucide for icons; Linear-style monochrome only.

### 7.2 App shell (P)
- Three pane: workspace strip → folder/account sidebar → message list → reader.
- Command palette (⌘K) via `shadcn/command` for sender search, message search, "go to put-aside", "sync now".
- Status bar (bottom) shows Ollama health + last sync per account.

### 7.3 Inbox view (P)
- Virtualized list when count > 200 (use `@tanstack/react-virtual`; not yet added — add when message count outgrows the naive render).
- HTML body rendered inside a sandboxed `<iframe srcdoc>` with `style` overrides for color-scheme: dark.
- Fetch-older button at the bottom of the list — calls `Backfill` and re-queries.

### 7.4 Settings (P)
- Workspaces CRUD, Accounts CRUD, Ollama health, Telegram setup ship.
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
