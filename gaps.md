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
- [x] **Compose new mail.** Pencil button in the inbox header opens a Dialog
      with from-account picker, To/Cc/Subject/Body. Sends via SMTPSender.Send.
- [ ] **Attachments.** Compose still has no attachment upload. Reply has none
      either. `Attachments` service exposes Get/GetInline but no Upload yet.
- [ ] **Calendar promote → Google.** `planner.go` still writes
      `"pending:ID"`. Real Google sync is unimplemented on both sides.

## Genuinely unbuilt (both sides)

- [ ] **Gmail incremental sync.** `accounts.gmail_history_id` column exists,
      but neither sync path uses the History API. Full re-sync every pass.
- [x] **Todos.** Due-date input on the form, six filter buttons, and sorted
      by dueAt ascending. Overdue rows tinted red.
- [ ] **Calendar.** No conflict detection. Timezone handling assumes local;
      rows stored as UTC.
- [x] **Notes.** Markdown preview tab (tiny inline renderer for headings,
      bold/italic, lists, fenced code, links). FTS within workspace still
      missing.

## Remaining order of attack

1. Calendar conflict detection (small lift, big UX win).
2. Notes FTS (search across notes via SQLite FTS5).
3. Attachment upload binding + UI (Compose/Reply file picker).
4. Gmail incremental sync (scalability cliff, but invisible until then).
5. Real Google Calendar 2-way sync (OAuth flow + provider work — biggest).
