# Decisions

Open architectural questions and the option matrices used to resolve them.
Each section ends with a **Pick** line — that's the working assumption the code
proceeds on. Revisit when reality contradicts it.

---

## 1. Account auth: IMAP/SMTP vs OAuth

The app needs to read mailboxes and (eventually) send replies. Two
fundamentally different access paths.

| Axis | IMAP/SMTP (+ app password) | OAuth 2.0 (Gmail API, MS Graph) |
|---|---|---|
| Provider coverage | Universal — anything that speaks IMAP works | Per-provider; need to wire Gmail, then MS, then... |
| User setup burden | Generate app password, paste it; sometimes enable IMAP first | Browser consent flow, but no manual passwords |
| Token storage | Single secret per account (the app password) | Refresh tokens; need periodic rotation handling |
| Gmail viability | Still works for legacy with app passwords; Google has hinted at deprecation | First-class; only sane path for new Gmail accounts long-term |
| Yahoo viability | Works with app passwords | Yahoo OAuth exists, partner-only registration is painful |
| Categories / labels / folders | Just folders (`[Gmail]/...` hacks) | Native: Gmail labels, MS categories |
| Push / change notifications | IMAP IDLE (one connection per mailbox, fragile) | Watch endpoints + Pub/Sub (Gmail) or webhooks (MS) |
| Send | SMTP — universal but per-provider hostnames/ports | Provider send endpoint, no SMTP config |
| Local dev friction | Low — paste credentials and go | High — register an OAuth app per provider, redirect URI, secrets |
| Failure modes | Auth fails silently when provider disables app passwords | Refresh-token revocation, scope drift on provider changes |
| Threat surface | One long-lived password per account | Token theft is bounded by scopes + revocable |

**Pick:** support **both**, IMAP/SMTP first. IMAP is the only realistic path to
"works with any account today" and the local-only nature of miniclaw makes
storing a per-account secret acceptable. Add Gmail OAuth as a second path once
the IMAP path is shipping value, because Gmail's app-password story is the
weakest. Defer MS Graph / Yahoo OAuth until requested.

---

## 2. DB layer: GORM vs sqlc

SQLite is the store. Question is how Go code talks to it.

| Axis | GORM | sqlc | (alt: plain `database/sql` + a small helper) |
|---|---|---|---|
| Style | Struct-tag ORM, runtime reflection | Codegen — write SQL, get typed Go funcs | Hand-rolled `Query`/`Scan` |
| Schema migrations | Has `AutoMigrate` but it's a footgun on SQLite (column adds OK, type changes drop tables) | Schema-first via plain SQL files; pair with golang-migrate / goose | Same — schema in SQL files |
| Type safety | Weak: query strings + reflection; runtime errors for typos | Strong: queries fail to compile if SQL is wrong | Weak — same as GORM at query time |
| SQLite quirks (NULL, datetime, JSON1) | Mostly hidden, sometimes wrongly | Visible — you write the SQL, you control coercion | Visible |
| Generated code in tree | None | Yes — committed under `internal/db/` | None |
| Onboarding | Familiar pattern for many devs | New tool; one-time learning curve | Lowest abstraction |
| Reads vs writes | Easy CRUD, gets ugly for joins/CTEs | Same syntax for everything | Same |
| FTS5 / triggers / `WITHOUT ROWID` | Awkward; need raw SQL escapes | Native — you're writing SQL | Native |
| Migration coupling | GORM models drift from real schema unless `AutoMigrate` is used (and you really shouldn't on prod) | Migrations and queries both live in `.sql`, single source of truth | Same as sqlc |
| License | MIT | MIT | stdlib |

**Pick:** **sqlc**. The app's queries are not generic CRUD — there's full-text
search over email bodies, joins across `accounts × workspaces × emails × summaries`,
and triggers for sync-state. sqlc keeps SQL honest, plays well with FTS5, and
avoids GORM's SQLite footguns. Pair with **goose** for migrations
(plain `.sql` files, applied on startup).

---

## 3. Keep `build/` or remove

The Wails v3 scaffold ships a `build/` directory with platform Taskfiles,
icons, plists, NSIS scripts, and AppImage glue. Question is whether it stays.

| Axis | Keep `build/` as-is | Trim to needed platforms only | Remove entirely |
|---|---|---|---|
| `wails3 build` / `wails3 dev` work out of the box | Yes | Yes, on supported platforms | No — would need to rebuild a custom build system |
| Repo noise | High — iOS, Android, Linux, Windows configs for a Mac-first dev | Lower | Lowest |
| Recoverable from history if removed | n/a | Yes | Yes (just `git checkout 60c6243 -- build/`) |
| Out-of-scope platforms today | iOS, Android (CLAUDE.md says scaffolding stays but not priority) | Same | Same |
| Risk of breaking later when shipping for X | n/a | Trim mistakenly and have to restore | Have to restore |

**Pick:** **keep `build/` as-is for now**. Removing pieces saves a few KB and
costs the safety net of a working `wails3` toolchain. Revisit if/when we
actively decide we're shipping macOS-only. iOS/Android files stay per CLAUDE.md
("scaffolding stays, not a priority").

---

## 4. Provider categories: native API vs local filter approximation

"Categories tab" (Hey-style) means surfacing provider-side groupings —
Gmail labels / categories, Outlook categories. Two ways to get them.

| Axis | Native API (OAuth) | Local rules / filter approximation (IMAP) |
|---|---|---|
| Accuracy vs the user's actual provider view | 1:1 — same labels they see in Gmail | Best-effort; "Promotions" via header heuristics |
| Provider coverage | Only OAuth providers | Any IMAP account |
| Maintenance | Provider can rename / reshape categories — track via API | Heuristics rot as senders change patterns |
| User control | Read-only of provider state | Fully user-editable |
| Privacy | Hits provider API for label list (still local-only inference though) | Pure-local |
| Implementation cost | Per-provider integration, but data is "free" | One filter engine + a starter rule pack |

**Pick:** **native API for OAuth accounts; local filter for IMAP accounts.**
Both paths exist behind a single `Categories` interface in
`internal/services/email`. OAuth implementations call the provider; IMAP falls
back to a rules engine seeded with a "Promotions / Updates / Social" starter
pack. This avoids forcing IMAP users to lose the feature, and avoids
re-implementing what Gmail already computes for OAuth users.

---

## 5. Telegram fan-out: per-email recipient mapping

Goal: one inbound email may notify many Telegram recipients (e.g. "anything to
family@ pings all four family members"). Need a schema.

| Axis | A. Per-account → fixed recipient list | B. Per-workspace → recipient list | C. Per-email rule engine (sender/subject → recipient set) |
|---|---|---|---|
| Mental model | "Mail to dad@gmail goes to dad" | "Anything in the Family workspace pings family chat" | "If from school AND about Lily, ping Mom + Dad" |
| Setup cost (per recipient) | One config row per recipient per account | One config row per workspace | Many — needs a rule UI |
| Flexibility | Low | Medium | High |
| Wrong-target risk | Low — explicit mapping | Low — workspace assignment is already deliberate | Medium — rules can mis-fire |
| Daily-digest fit | Trivial — group by recipient over their accounts | Trivial — group by workspace | Have to evaluate rules per email per day |
| Storage | `recipient_account(account_id, recipient_id)` | `recipient_workspace(workspace_id, recipient_id)` | `notification_rule(id, predicate, recipient_id)` |

**Pick:** **start with B (per-workspace), allow A (per-account override) on top.**
Workspaces already exist in the data model and already capture the "family vs
work vs personal" axis the goal describes. Per-account override handles edge
cases ("only notify me for this specific account") without a rule engine.
Skip C until users actually ask for it.

Schema sketch:

```
telegram_recipient(id, name, chat_id, created_at)
workspace_recipient(workspace_id, recipient_id)   -- many-to-many, default fan-out
account_recipient(account_id, recipient_id)       -- many-to-many, overrides
                                                  -- presence in either set = notify
```

---

## Log

- 2026-06-08: initial draft of all 5 matrices.
