# Setup

Everything a human has to do to get miniclaw running locally. Skip the
sections for paths you don't use (e.g. if you only want IMAP, ignore the
Gmail OAuth section for now).

This file gets appended to as decisions land. See `docs/decisions.md` for
the *why*.

---

## 1. Tooling

These have to exist on your machine before any of the below works.

| Tool | Version | How |
|---|---|---|
| Go | matches `go.mod` (currently 1.25.x) | https://go.dev/dl |
| Node | pinned in `frontend/.nvmrc` | `nvm install` then `nvm use` inside `frontend/` |
| npm | ships with Node | — |
| Wails v3 CLI | latest alpha | `go install github.com/wailsapp/wails/v3/cmd/wails3@latest` |
| Task | https://taskfile.dev (Wails v3 uses it) | `brew install go-task` |
| Ollama | latest | https://ollama.com/download |
| SQLite CLI | optional | `brew install sqlite` |
| `golangci-lint` | latest | `brew install golangci-lint` |
| `fallow` | latest | `npm install -g fallow` (per-Node-version under nvm) |

Verify:

```bash
go version
node --version          # must match frontend/.nvmrc
wails3 doctor
ollama --version
```

---

## 2. Ollama

The app expects Ollama running at `http://localhost:11434` (its default).

```bash
ollama serve &           # or run the desktop app
ollama pull llama3.2:3b  # small instruct model, good default
ollama list              # confirm it's there
```

Any installed instruct model will show up in the onboarding model picker.

---

## 3. Email accounts

### IMAP / SMTP (recommended first path)

For each account you want to connect:

1. **Enable IMAP** in the provider's webmail settings if it's off by default.
2. **Generate an app password** — most providers don't accept your real
   password from third-party clients with 2FA on.
3. Note your provider's **IMAP host + port** and **SMTP host + port**.
4. Paste host, port, email address, app password into the account form
   in-app.

Provider-specific notes:

- **Gmail** — Settings → Forwarding and POP/IMAP → enable IMAP. App
  password needs 2-Step Verification enabled first; generate at
  https://myaccount.google.com/apppasswords. Hosts: `imap.gmail.com:993`,
  `smtp.gmail.com:587` (STARTTLS) or `:465` (TLS).
- **Yahoo** — Account Info → Account Security → Generate app password.
  Hosts: `imap.mail.yahoo.com:993`, `smtp.mail.yahoo.com:465`.
- **Fastmail / iCloud / Outlook personal** — each has an app-password
  flow under their security settings.
- **Self-hosted / corporate** — get host/port from your admin.

Secrets are stored in the OS keychain via a Wails service. The SQLite row
holds only host, port, username, and a keychain reference — never the
password itself.

### Gmail OAuth

1. Create a project at https://console.cloud.google.com.
2. Enable the **Gmail API**.
3. Create OAuth client credentials, type **Desktop app**.
4. The loopback redirect (`http://127.0.0.1:<random-port>/callback`) is
   registered at runtime — no need to pre-register URIs.
5. Drop the downloaded JSON at `~/.miniclaw/google_oauth_client.json`.
6. In Settings → Add account → **Gmail OAuth** → Sign in with Google.
   Refresh token lives in the OS keychain under
   `gmail_oauth:<email>`. miniclaw also reads `labelIds` on each message
   so Gmail's native CATEGORY_PROMOTIONS/SOCIAL/UPDATES/FORUMS labels
   feed the Categories tab directly.

### Microsoft OAuth (Outlook, Hotmail, Live, M365)

1. Go to https://portal.azure.com → Azure Active Directory →
   **App registrations** → New registration.
2. Pick **Accounts in any organizational directory + personal Microsoft
   accounts** (multi-tenant + consumers) so personal Outlook/Hotmail
   accounts work alongside work/school.
3. Redirect URI: add a **Public client/native** URI of
   `http://localhost`. miniclaw will use `http://127.0.0.1:<random-port>`
   at runtime; Azure treats loopback as a wildcard for public clients.
4. API permissions → Add → **Microsoft Graph → Delegated** →
   `Mail.Read`, `User.Read`, `offline_access`. Grant admin consent if
   you need a work tenant.
5. From the app's **Overview** page, copy the **Application (client) ID**.
6. Save to `~/.miniclaw/ms_oauth_client.json` as:
   ```json
   { "client_id": "YOUR-CLIENT-ID-GUID", "tenant": "common" }
   ```
   Use `"tenant": "<your-tenant-guid>"` for single-tenant apps.
7. Settings → Add account → **Microsoft OAuth** → Sign in. The refresh
   token lands in the keychain under `ms_oauth:<email>`. Sync uses
   Microsoft Graph `/me/mailFolders/Inbox/messages`.

### Yahoo

Yahoo's OAuth program is partner-only — there is no public developer
registration that grants Mail API access. **Use IMAP/SMTP instead:**

1. Yahoo Account → **Account Info → Account Security**.
2. Turn on 2-Step Verification.
3. Generate an **App password** (label it "miniclaw").
4. Add account in Settings as IMAP/SMTP with
   `imap.mail.yahoo.com:993` and `smtp.mail.yahoo.com:465`.

---

## 4. Telegram

The morning digest and "needs attention" pings go through a Telegram bot
you own.

1. DM [@BotFather](https://t.me/BotFather), `/newbot`, follow prompts,
   keep the **bot token**.
2. Start a chat with your new bot (or add it to a group).
3. Get your **chat ID**: send any message, then
   `curl https://api.telegram.org/bot<TOKEN>/getUpdates` and read
   `result[].message.chat.id`.
4. In-app: Settings → Telegram → paste bot token, then add recipients by
   name + chat ID.
5. Assign recipients to workspaces (or to individual accounts as
   overrides). Per `docs/decisions.md` §5.

---

## 5. Workspaces

Defaults seeded on first launch: **Family, Work, Personal, Other**. You
can rename, add, delete, and emoji each one. Every account is assigned to
exactly one workspace.

---

## 6. Running the project

```bash
# from repo root
make deps                # tidy Go modules + install npm in root + frontend
make dev                 # wails3 dev — opens window with hot-reload
```

Other useful targets:

```bash
make build               # production binary
make bindings            # regenerate Go ↔ JS bindings
make fmt                 # gofmt + goimports + biome format
make lint                # golangci-lint + biome lint
make test                # go test -race -coverprofile
make ollama.up           # docker-compose Ollama if you don't want native
```

---

## 7. Status

All §7 items from the original goal are landed:

- [x] IMAP/SMTP account connect + secret storage (`internal/services/email`, `internal/services/keychain`)
- [x] Gmail OAuth path (`internal/services/gmailoauth`)
- [x] Ollama client + per-workspace model selection (`internal/services/ollama`, per-account `ollama_model`)
- [x] Email ingest scheduler + sync window controls (`internal/scheduler`, per-account `sync_cadence_secs`)
- [x] Summarization + needs-attention scoring (`internal/services/summary`)
- [x] Telegram bot wiring + per-workspace/account recipient routing (`internal/services/telegram`)
- [x] Daily digest scheduler (`internal/services/digest`)
- [x] Hey-style triage: put-aside, screener, spam/filter list (`internal/services/triage`)
- [x] Categories tab — IMAP filter approximation (`internal/services/categories`); OAuth-native labels deferred until user demand
- [x] SQLite migrations (goose) + sqlc-generated queries (`internal/db/*`)
- [x] OS keychain service (`internal/services/keychain`)

Frontend: tabs for Inbox, Put Aside, Screener, Filters, Categories,
Settings. Settings holds workspace CRUD, account add (IMAP form + Gmail
OAuth button), Ollama status + model list, Telegram bot token + digest
time + recipient management.

Address-of-limitations log (all previously-known items resolved):
- IMAP multi-folder walk: per-account `folder_allowlist` (CSV) drives a
  per-folder sync loop on one connection; INBOX is the default when
  empty.
- Gmail native labels: `gmailoauth` reads `labelIds` from
  `messages.get` and maps CATEGORY_PROMOTIONS/SOCIAL/UPDATES/FORUMS to
  the Categories tab; local rules engine is the fallback.
- Reply from app: inbox reading pane has a Reply button that opens a
  composer and calls `SMTPSender.Send`.
- IMAP push: `email.IDLE` keeps one IMAP IDLE connection per IMAP
  account, rotates every 25 minutes, debounces bursts, and triggers
  ingest+summarise within hundreds of ms of a new message landing.
- Microsoft OAuth: full PKCE loopback flow against Microsoft identity,
  Graph-API sync from `/me/mailFolders/Inbox/messages`.

Still optional, not yet wired (low-priority, will land on demand):
- Gmail Pub/Sub watch (IDLE-equivalent for OAuth Gmail) — uses GCP
  Pub/Sub and a public webhook; IMAP IDLE covers most users.
- Per-account UI for `folder_allowlist` editing (the backend column +
  service method exist; add an input to Settings → Accounts when a
  user asks).

## 8. What can't be done autonomously (human-only)

These steps require *you* — the assistant cannot perform them.

- Generating app passwords on each provider.
- Creating the Google Cloud OAuth client and downloading the JSON.
- Creating the Telegram bot via @BotFather and capturing the token.
- Installing Ollama and `ollama pull`ing the model you want.
- Installing system-level tooling (`brew install`, `nvm install`).
- Approving keychain prompts the first time a secret is stored.
