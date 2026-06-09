# Backend ↔ Frontend gap audit

Snapshot of where the Wails backend and React frontend are out of sync. Ordered
by impact. File and line references are to the state of the repo when this was
written; verify before acting.

## Orphaned backend capability (data exists, UI doesn't surface it)

- **Email threading.** `emails.thread_id`, `in_reply_to`, `email_refs` columns
  plus the sqlc `ListEmailsByThread` query exist (migration 0005). Gmail sync
  derives thread IDs (`internal/services/gmailoauth/sync.go:84`). But
  `inbox.Service` does not expose a binding for it and no view renders threads.
  Biggest single gap.
- **Gmail labels.** `email_labels` table is populated by Gmail sync; sqlc has
  `AddEmailLabel` / `ListEmailLabels`. No retrieval method on `inbox.Service`,
  no UI. CATEGORY_*, IMPORTANT, and user labels are invisible.
- **Mark unread.** `Inbox.MarkUnread()` exists at
  `internal/services/inbox/inbox.go:244` and is never called from the
  frontend.
- **Digest "send now".** `Digest.RunNow()` at
  `internal/services/digest/digest.go:66` has no UI trigger in SettingsView.
- **Per-account sync cadence / folder allowlist.** `account.sync_cadence_secs`
  and `account.folderAllowlist` are in the model; no settings inputs to edit
  either.
- **Re-classify categories.** `Categories.ClassifyAccount()` exists at
  `internal/services/categories/categories.go:46`; `CategoriesView` is
  read-only.

## UI incomplete vs backend capability

- **Snooze.** Backend accepts arbitrary RFC3339; `InboxView.jsx:792-817`
  hardcodes four presets. No custom date picker.
- **Search.** `Inbox.Search()` works and CommandPalette uses it, but the inbox
  view itself has no search input. Users have to Cmd+K.
- **Reply / compose.** Reply works (`InboxView.jsx:706-773`). No new-mail
  compose, no draft persistence, no attachment upload — `Attachments` service
  has no `Upload` binding.
- **Calendar promote → Google.** `internal/services/planner/planner.go:100-108`
  writes `"pending:ID"` as a placeholder; real Google sync is unimplemented on
  both sides.

## Genuinely unbuilt (both sides)

- **Gmail incremental sync.** `accounts.gmail_history_id` column exists, but
  neither sync path uses the History API. Full re-sync every pass.
- **Todos.** No due-date filter or sort UI.
- **Calendar.** No conflict detection. Timezone handling assumes local; rows
  stored as UTC.
- **Notes.** No markdown preview, no FTS within workspace.

## Suggested order of attack

1. Expose threading + labels on `inbox.Service` and build the UI. Most backend
   work is already done; biggest user-visible return.
2. Small UI-only fills: mark-unread, custom-snooze date picker, inbox search
   input, digest "send now" button.
3. Settings: per-account cadence + folder allowlist editors.
4. Compose + attachments.
5. Gmail incremental sync. Invisible until mailboxes get large, but a
   scalability cliff.
