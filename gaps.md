# Backend ↔ Frontend gap audit

Living punch list. Items marked `[x]` are done; `[ ]` are still open.

## Orphaned backend capability

- [x] **Email threading.** `Inbox.ListByThread` exposed; reader shows a
      collapsible "Conversation (N)" panel with sibling navigation.
- [x] **Gmail labels.** `EmailDetail.labels` populated server-side and
      rendered as badges in the reader header.
- [x] **Mark unread.** Button next to "Put aside" calls `Inbox.MarkUnread`
      and returns to the inbox.
- [x] **Digest "send now".** Settings button shows busy + result state.
- [x] **Per-account sync cadence / folder allowlist.** Per-row edit panel in
      SettingsView (cadence, model, allowlist for IMAP).
- [x] **Re-classify categories.** Button in CategoriesView fans out across
      accounts and reports the count.

## UI incomplete vs backend capability

- [x] **Snooze.** Dropdown gained "Pick a time…" that opens a Dialog with a
      datetime-local input.
- [x] **Search.** Inbox-pane search input wired to `Inbox.Search`.
- [ ] **Reply / compose.** Reply works. Still missing: new-mail compose,
      draft persistence, attachment upload (`Attachments` service has no
      `Upload` binding yet).
- [ ] **Calendar promote → Google.** `planner.go` still writes
      `"pending:ID"`. Real Google sync is unimplemented on both sides.

## Genuinely unbuilt (both sides)

- [ ] **Gmail incremental sync.** `accounts.gmail_history_id` column exists,
      but neither sync path uses the History API. Full re-sync every pass.
- [ ] **Todos.** No due-date filter or sort UI.
- [ ] **Calendar.** No conflict detection. Timezone handling assumes local;
      rows stored as UTC.
- [ ] **Notes.** No markdown preview, no FTS within workspace.

## Suggested order of attack for the rest

1. Compose-new + attachment upload (largest user-visible reply-side gap).
2. Todos due-date sort + filter.
3. Notes markdown preview (cheap polish).
4. Gmail incremental sync (scalability cliff, but invisible until then).
5. Real Google Calendar sync (largest, OAuth-flow-heavy).
