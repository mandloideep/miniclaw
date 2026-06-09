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

### Gmail OAuth (deferred — wired second)

When this lands you'll need to:

1. Create a project at https://console.cloud.google.com.
2. Enable the **Gmail API**.
3. Create OAuth client credentials, type **Desktop app**.
4. Add `http://localhost:<port>/oauth/callback` as a redirect URI (port
   chosen at runtime).
5. Drop the downloaded JSON at `~/.miniclaw/google_oauth_client.json` (or
   point the app at it via the settings UI).
6. First connect kicks off browser consent; refresh tokens persist in the
   OS keychain.

This path is not implemented yet. Tracked in `docs/decisions.md` §1.

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

## 7. What's not done yet

This list shrinks as features land. Maintained here so the handoff is
explicit.

- [ ] IMAP/SMTP account connect + secret storage
- [ ] Gmail OAuth path
- [ ] Ollama client + per-workspace model selection
- [ ] Email ingest scheduler + sync window controls
- [ ] Summarization + needs-attention scoring
- [ ] Telegram bot wiring + per-workspace/account recipient routing
- [ ] Daily digest scheduler
- [ ] Hey-style triage: put-aside, screener, spam/filter list
- [ ] Categories tab (OAuth native + IMAP filter approximation)
- [ ] SQLite migrations (goose) + sqlc-generated queries
- [ ] OS keychain service

## 8. What can't be done autonomously (human-only)

These steps require *you* — the assistant cannot perform them.

- Generating app passwords on each provider.
- Creating the Google Cloud OAuth client and downloading the JSON.
- Creating the Telegram bot via @BotFather and capturing the token.
- Installing Ollama and `ollama pull`ing the model you want.
- Installing system-level tooling (`brew install`, `nvm install`).
- Approving keychain prompts the first time a secret is stored.
