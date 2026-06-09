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
- [x] **Calendar promote → Google.** `CalendarService.Promote(blockId,
      accountId)` posts to Google Calendar's events.insert and persists the
      real event ID. `PullFromGoogle(workspaceId, accountId)` imports
      upcoming events as kind="meeting" blocks. Push and pull share the
      Gmail-OAuth account's token (calendar.events scope added in
      `gmailoauth.Scopes`). Frontend has account picker, Pull button, and
      friendly error mapping for insufficient_scope / expired token.

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

## Done — all gaps closed

Everything originally tracked here is now wired through. The Google Calendar
work added a `calendar.events` scope to `gmailoauth.Scopes`, so existing
Gmail-OAuth accounts must re-authorise once before push/pull will succeed
(the UI surfaces a friendly hint when it sees the `insufficient` error).

Known small follow-ups not on the original list:
- Local block edits don't propagate to a previously-pushed Google event;
  only the initial push is implemented (no `events.patch` call yet).
- Calendar pull doesn't store a watermark, so re-running it re-walks the
  same 31-day window each time (cheap, but not incremental).
