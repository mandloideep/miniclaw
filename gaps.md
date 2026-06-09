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
- [x] **Attachments.** `OutgoingMessage.attachments` lands as
      `multipart/mixed` parts in the RFC822 envelope. Compose dialog has a
      file picker that base64-encodes each file before send.
- [ ] **Calendar promote → Google.** `planner.go` still writes
      `"pending:ID"`. See note below.

## Genuinely unbuilt (both sides)

- [x] **Gmail incremental sync.** Already wired in `gmailoauth/sync.go` —
      `listHistoryMessageIDs` consults `users.history.list` when a cursor is
      stored, falls back to the date-bounded `messages.list` if expired.
      Initial audit was wrong; nothing to do here.
- [x] **Todos.** Due-date input on the form, six filter buttons, and sorted
      by dueAt ascending. Overdue rows tinted red.
- [x] **Calendar conflict detection.** Overlapping blocks are tinted red
      and list the conflicting block titles. Timezone handling still
      local-only.
- [x] **Notes markdown preview.** Tab toggles between editor and a tiny
      inline renderer (headings, bold/italic, lists, fenced code, links).
- [x] **Notes search.** `NotesService.Search` runs a workspace-scoped LIKE
      query; NotesPane shows a search input above the list.

## Deferred — needs its own session

- [ ] **Google Calendar 2-way sync.** Genuinely substantial work and out of
      scope for a single sweep:
  - The current Gmail OAuth scope set doesn't include
    `https://www.googleapis.com/auth/calendar.events`. Adding it forces every
    Gmail-OAuth account through re-consent.
  - Need a calendar client (`events.list`, `events.insert`, `events.update`,
    `events.delete`) and a sync watermark separate from `gmail_history_id`.
  - Need to decide: pull-only (mirror Google → local), push-only (promote
    local → Google), or true 2-way with last-writer-wins.
  - `planner.go:100-108` should stay as a stub until those decisions land.
  Treat this as its own ticket, not a gap to chip at.
