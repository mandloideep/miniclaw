import { useCallback, useEffect, useState } from "react";
import { Accounts, Digest, GmailOAuth, MSOAuth, Ollama, Telegram, Workspaces } from "../api";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "../components/ui/alert-dialog";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "../components/ui/tabs";

// DestructiveConfirm wraps the standard "are you sure" + cancel/confirm
// flow so call sites stay tiny. Renders the trigger inline (it's a slot)
// and only calls onConfirm when the action button is clicked.
function DestructiveConfirm({ trigger, title, description, confirmLabel = "Remove", onConfirm }) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && <AlertDialogDescription>{description}</AlertDialogDescription>}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel asChild>
            <Button variant="ghost" size="sm">
              Cancel
            </Button>
          </AlertDialogCancel>
          <AlertDialogAction asChild>
            <Button
              size="sm"
              onClick={onConfirm}
              className="bg-danger/15 text-danger hover:bg-danger/25"
            >
              {confirmLabel}
            </Button>
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export default function SettingsView({
  workspaces,
  accounts,
  ollamaStatus,
  onWorkspacesChanged,
  onAccountsChanged,
}) {
  // Single Ollama.ListModels fetch, shared by AccountsSection and
  // OllamaSection so opening Settings doesn't fan out to two /api/tags hits.
  const [models, setModels] = useState([]);
  useEffect(() => {
    Ollama.ListModels()
      .then((rows) => setModels(rows || []))
      .catch(() => setModels([]));
  }, []);
  return (
    <div className="max-w-3xl space-y-8">
      <WorkspacesSection workspaces={workspaces} onChange={onWorkspacesChanged} />
      <AccountsSection
        workspaces={workspaces}
        accounts={accounts}
        models={models}
        onChange={onAccountsChanged}
      />
      <OllamaSection status={ollamaStatus} models={models} />
      <TelegramSection accounts={accounts} workspaces={workspaces} />
    </div>
  );
}

function Section({ title, description, action, children }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div>
          <CardTitle>{title}</CardTitle>
          {description && <CardDescription>{description}</CardDescription>}
        </div>
        {action}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

function WorkspacesSection({ workspaces, onChange }) {
  const [draft, setDraft] = useState({ name: "", emoji: "" });
  return (
    <Section
      title="Workspaces"
      description="Group accounts into themed inboxes. Each workspace gets its own list, summary cadence, and Telegram destination."
    >
      <ul className="space-y-1.5 mb-4">
        {workspaces.map((w) => (
          <li
            key={w.id}
            className="flex items-center gap-3 px-3 py-2 rounded-md border border-hairline bg-surface-2"
          >
            <span className="text-base">{w.emoji}</span>
            <span className="flex-1 text-[13px] text-ink">{w.name}</span>
            <Badge variant="muted">
              {/* eslint-disable-next-line react/no-array-index-key */}
              {workspaces.length === 1 ? "default" : "workspace"}
            </Badge>
            <DestructiveConfirm
              title={`Delete workspace "${w.name}"?`}
              description="Accounts inside it and their cached email also go. This can't be undone."
              confirmLabel="Delete workspace"
              onConfirm={async () => {
                await Workspaces.Delete(w.id);
                onChange();
              }}
              trigger={
                <Button variant="ghost" size="xs" className="text-ink-subtle hover:text-danger">
                  Remove
                </Button>
              }
            />
          </li>
        ))}
      </ul>
      <form
        className="flex gap-2 items-end"
        onSubmit={async (e) => {
          e.preventDefault();
          if (!draft.name) return;
          await Workspaces.Create(draft.name, draft.emoji);
          setDraft({ name: "", emoji: "" });
          onChange();
        }}
      >
        <div>
          <Label htmlFor="ws-emoji">Emoji</Label>
          <Input
            id="ws-emoji"
            value={draft.emoji}
            onChange={(e) => setDraft({ ...draft, emoji: e.target.value })}
            placeholder="✨"
            className="w-16 mt-1"
          />
        </div>
        <div className="flex-1">
          <Label htmlFor="ws-name">Name</Label>
          <Input
            id="ws-name"
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder="Workspace name"
            className="mt-1"
          />
        </div>
        <Button type="submit">Add</Button>
      </form>
    </Section>
  );
}

function AccountRow({ account, workspace, models, onChange }) {
  const [open, setOpen] = useState(false);
  const [cadence, setCadence] = useState(account.syncCadenceSecs || 300);
  const [allowlist, setAllowlist] = useState(account.folderAllowlist || "");
  const [model, setModel] = useState(account.ollamaModel || "");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  const isIMAP = account.authKind === "imap";

  const save = async () => {
    setSaving(true);
    try {
      const work = [];
      if (Number(cadence) !== account.syncCadenceSecs) {
        work.push(Accounts.SetCadence(account.id, Number(cadence)));
      }
      if (model !== (account.ollamaModel || "")) {
        work.push(Accounts.SetModel(account.id, model));
      }
      if (isIMAP && allowlist !== (account.folderAllowlist || "")) {
        work.push(Accounts.SetFolderAllowlist(account.id, allowlist));
      }
      await Promise.all(work);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
      onChange();
    } finally {
      setSaving(false);
    }
  };

  return (
    <li className="border border-hairline rounded">
      <div className="px-3 py-2 flex justify-between items-start">
        <button
          type="button"
          className="text-left flex-1 min-w-0"
          onClick={() => setOpen((v) => !v)}
        >
          <div className="text-sm">
            {account.emailAddress}{" "}
            <span className="text-xs text-ink-subtle">({account.authKind})</span>
          </div>
          <div className="text-xs text-ink-subtle">
            {workspace ? `${workspace.emoji} ${workspace.name}` : "—"} ·{" "}
            {account.lastSyncedAt ? `synced ${account.lastSyncedAt}` : "never synced"} · every{" "}
            {Math.round((account.syncCadenceSecs || 300) / 60)} min · model:{" "}
            {account.ollamaModel || "(default)"}
          </div>
        </button>
        <div className="flex items-center gap-2 shrink-0">
          <button
            type="button"
            className="text-xs text-ink-subtle hover:text-ink"
            onClick={() => setOpen((v) => !v)}
          >
            {open ? "close" : "edit"}
          </button>
          <DestructiveConfirm
            title={`Remove ${account.emailAddress}?`}
            description="Cached emails and the keychain secret are removed locally. The mailbox on the provider stays untouched."
            confirmLabel="Remove account"
            onConfirm={async () => {
              await Accounts.Delete(account.id);
              onChange();
            }}
            trigger={
              <button type="button" className="text-xs text-ink-subtle hover:text-danger">
                remove
              </button>
            }
          />
        </div>
      </div>
      {open && (
        <div className="px-3 pb-3 pt-1 border-t border-hairline space-y-2.5">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor={`acc-${account.id}-cadence`} className="text-xs">
                Sync every (seconds, min 30)
              </Label>
              <Input
                id={`acc-${account.id}-cadence`}
                type="number"
                min={30}
                value={cadence}
                onChange={(e) => setCadence(e.target.value)}
                className="mt-1"
              />
            </div>
            <div>
              <Label htmlFor={`acc-${account.id}-model`} className="text-xs">
                Ollama model
              </Label>
              <select
                id={`acc-${account.id}-model`}
                value={model}
                onChange={(e) => setModel(e.target.value)}
                className="mt-1 w-full px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
              >
                <option value="">(workspace default)</option>
                {(models || []).map((m) => {
                  // Ollama.ListModels returns Model objects; the old code
                  // assumed a list of strings, which crashed the row open.
                  const name = typeof m === "string" ? m : m?.name;
                  if (!name) return null;
                  return (
                    <option key={name} value={name}>
                      {name}
                    </option>
                  );
                })}
              </select>
            </div>
          </div>
          {isIMAP && (
            <div>
              <Label htmlFor={`acc-${account.id}-folders`} className="text-xs">
                Folder allowlist (comma-separated; empty = INBOX only)
              </Label>
              <Input
                id={`acc-${account.id}-folders`}
                placeholder="INBOX, [Gmail]/All Mail"
                value={allowlist}
                onChange={(e) => setAllowlist(e.target.value)}
                className="mt-1"
              />
            </div>
          )}
          <div className="flex items-center gap-3">
            <Button size="sm" onClick={save} disabled={saving}>
              {saving ? "Saving…" : "Save"}
            </Button>
            {saved && <span className="text-xs text-ink-subtle">Saved.</span>}
          </div>
        </div>
      )}
    </li>
  );
}

function AccountsSection({ workspaces, accounts, models, onChange }) {
  const [showAdd, setShowAdd] = useState(false);
  return (
    <Section title="Accounts">
      <ul className="space-y-1.5 mb-3">
        {accounts.map((a) => (
          <AccountRow
            key={a.id}
            account={a}
            workspace={workspaces.find((w) => w.id === a.workspaceId)}
            models={models}
            onChange={onChange}
          />
        ))}
      </ul>
      {showAdd ? (
        <AddAccountForm
          workspaces={workspaces}
          models={models}
          onClose={() => setShowAdd(false)}
          onAdded={() => {
            setShowAdd(false);
            onChange();
          }}
        />
      ) : (
        <button
          type="button"
          onClick={() => setShowAdd(true)}
          className="px-3 py-1.5 rounded bg-surface-2 text-sm hover:bg-surface-3"
        >
          + Add account
        </button>
      )}
    </Section>
  );
}

function AddAccountForm({ workspaces, models, onClose, onAdded }) {
  const [kind, setKind] = useState("imap");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [form, setForm] = useState({
    workspaceId: workspaces[0]?.id ?? 0,
    displayName: "",
    emailAddress: "",
    imapHost: "",
    imapPort: 993,
    smtpHost: "",
    smtpPort: 465,
    password: "",
    fetchSince: "",
    ollamaModel: models[0]?.name ?? "",
  });

  async function submit(e) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      if (kind === "imap") {
        await Accounts.AddIMAP({
          ...form,
          imapPort: Number(form.imapPort),
          smtpPort: Number(form.smtpPort),
        });
      } else if (kind === "gmail_oauth") {
        const res = await GmailOAuth.StartAuthorize();
        await Accounts.AddGmailOAuth({
          workspaceId: Number(form.workspaceId),
          displayName: form.displayName || res.EmailAddress,
          emailAddress: res.EmailAddress,
          refreshToken: res.RefreshToken,
          fetchSince: form.fetchSince,
          ollamaModel: form.ollamaModel,
        });
      } else if (kind === "ms_oauth") {
        const res = await MSOAuth.StartAuthorize();
        await Accounts.AddMSOAuth({
          workspaceId: Number(form.workspaceId),
          displayName: form.displayName || res.EmailAddress,
          emailAddress: res.EmailAddress,
          refreshToken: res.RefreshToken,
          fetchSince: form.fetchSince,
          ollamaModel: form.ollamaModel,
        });
      }
      onAdded();
    } catch (e) {
      setErr(String(e?.message ?? e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      onSubmit={submit}
      className="space-y-3 border border-hairline rounded-md p-3 bg-surface-2"
    >
      <Tabs value={kind} onValueChange={setKind}>
        <TabsList>
          <TabsTrigger value="imap">IMAP / SMTP</TabsTrigger>
          <TabsTrigger value="gmail_oauth">Gmail OAuth</TabsTrigger>
          <TabsTrigger value="ms_oauth">Microsoft OAuth</TabsTrigger>
        </TabsList>
      </Tabs>

      <select
        value={form.workspaceId}
        onChange={(e) => setForm({ ...form, workspaceId: Number(e.target.value) })}
        className="w-full px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
      >
        {workspaces.map((w) => (
          <option key={w.id} value={w.id}>
            {w.emoji} {w.name}
          </option>
        ))}
      </select>

      <input
        placeholder="display name"
        value={form.displayName}
        onChange={(e) => setForm({ ...form, displayName: e.target.value })}
        className="w-full px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
      />

      {kind === "imap" && (
        <>
          <input
            placeholder="email address"
            value={form.emailAddress}
            onChange={(e) => setForm({ ...form, emailAddress: e.target.value })}
            className="w-full px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
          />
          <div className="grid grid-cols-2 gap-2">
            <input
              placeholder="imap host"
              value={form.imapHost}
              onChange={(e) => setForm({ ...form, imapHost: e.target.value })}
              className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
            />
            <input
              placeholder="imap port"
              type="number"
              value={form.imapPort}
              onChange={(e) => setForm({ ...form, imapPort: e.target.value })}
              className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
            />
            <input
              placeholder="smtp host"
              value={form.smtpHost}
              onChange={(e) => setForm({ ...form, smtpHost: e.target.value })}
              className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
            />
            <input
              placeholder="smtp port"
              type="number"
              value={form.smtpPort}
              onChange={(e) => setForm({ ...form, smtpPort: e.target.value })}
              className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
            />
          </div>
          <input
            type="password"
            placeholder="app password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            className="w-full px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
          />
        </>
      )}

      <div className="grid grid-cols-2 gap-2">
        <input
          placeholder="fetch since (YYYY-MM-DD, optional)"
          value={form.fetchSince}
          onChange={(e) => setForm({ ...form, fetchSince: e.target.value })}
          className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        />
        <select
          value={form.ollamaModel}
          onChange={(e) => setForm({ ...form, ollamaModel: e.target.value })}
          className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        >
          <option value="">(default model)</option>
          {models.map((m) => (
            <option key={m.name} value={m.name}>
              {m.name}
            </option>
          ))}
        </select>
      </div>

      {err && <p className="text-xs text-danger">{err}</p>}
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={busy}
          className="px-3 py-1.5 rounded bg-brand text-sm disabled:opacity-50"
        >
          {busy
            ? "Working…"
            : kind === "imap"
              ? "Add"
              : kind === "gmail_oauth"
                ? "Sign in with Google"
                : "Sign in with Microsoft"}
        </button>
        <button
          type="button"
          onClick={onClose}
          className="px-3 py-1.5 rounded bg-surface-2 text-sm"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

function OllamaSection({ status, models }) {
  return (
    <Section title="Ollama">
      {!status?.running ? (
        <p className="text-sm text-danger">
          Ollama is not reachable at http://localhost:11434 — start it (`ollama serve`) and reload.
        </p>
      ) : (
        <>
          <p className="text-xs text-ink-subtle mb-2">
            Running v{status.version}. Available models:
          </p>
          <ul className="text-sm">
            {models.length === 0 && (
              <li className="text-xs text-ink-subtle">
                No models installed yet. `ollama pull llama3.2:3b` is a good first pick.
              </li>
            )}
            {models.map((m) => (
              <li key={m.name} className="py-0.5">
                {m.name} <span className="text-xs text-ink-subtle">{m.parameterSize}</span>
              </li>
            ))}
          </ul>
        </>
      )}
    </Section>
  );
}

function DigestRunButton() {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);
  return (
    <div className="ml-2 flex items-center gap-2">
      <Button
        variant="secondary"
        size="sm"
        disabled={busy}
        onClick={async () => {
          setBusy(true);
          setResult(null);
          try {
            await Digest.RunNow();
            setResult({ ok: true, msg: "Digest sent." });
          } catch (err) {
            setResult({ ok: false, msg: String(err?.message || err) });
          } finally {
            setBusy(false);
            setTimeout(() => setResult(null), 4000);
          }
        }}
      >
        {busy ? "Sending…" : "Send test digest now"}
      </Button>
      {result && (
        <span className={`text-[11px] ${result.ok ? "text-ink-subtle" : "text-rose-400"}`}>
          {result.msg}
        </span>
      )}
    </div>
  );
}

function TelegramSection({ accounts, workspaces }) {
  const [settings, setSettings] = useState({
    botToken: "",
    digestTime: "08:00",
  });
  const [recipients, setRecipients] = useState([]);
  const [draft, setDraft] = useState({ name: "", chatId: "" });

  const refresh = useCallback(async () => {
    setSettings(await Telegram.GetSettings());
    setRecipients(await Telegram.ListRecipients());
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <Section title="Telegram digest">
      <div className="space-y-2 mb-3">
        <label className="text-xs text-ink-subtle block">
          Bot token (from @BotFather)
          <input
            type="password"
            value={settings.botToken}
            onChange={(e) => setSettings({ ...settings, botToken: e.target.value })}
            onBlur={() => Telegram.SetBotToken(settings.botToken)}
            className="mt-1 w-full px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
          />
        </label>
        <label className="text-xs text-ink-subtle block">
          Digest time (HH:MM)
          <input
            value={settings.digestTime}
            onChange={(e) => setSettings({ ...settings, digestTime: e.target.value })}
            onBlur={() => Telegram.SetDigestTime(settings.digestTime).catch(() => {})}
            className="mt-1 w-32 px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
          />
        </label>
        <DigestRunButton />
      </div>

      <h3 className="text-xs text-ink-subtle uppercase mt-4 mb-1">Recipients</h3>
      <ul className="space-y-1.5 mb-3">
        {recipients.map((r) => (
          <RecipientRow
            key={r.id}
            recipient={r}
            workspaces={workspaces}
            accounts={accounts}
            onChanged={refresh}
          />
        ))}
      </ul>
      <form
        className="flex gap-2"
        onSubmit={async (e) => {
          e.preventDefault();
          if (!draft.name || !draft.chatId) return;
          await Telegram.AddRecipient(draft.name, draft.chatId);
          setDraft({ name: "", chatId: "" });
          refresh();
        }}
      >
        <input
          placeholder="name"
          value={draft.name}
          onChange={(e) => setDraft({ ...draft, name: e.target.value })}
          className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        />
        <input
          placeholder="telegram chat id"
          value={draft.chatId}
          onChange={(e) => setDraft({ ...draft, chatId: e.target.value })}
          className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        />
        <button type="submit" className="px-3 py-1.5 rounded bg-brand text-sm">
          Add recipient
        </button>
      </form>
    </Section>
  );
}

function RecipientRow({ recipient, workspaces, accounts, onChanged }) {
  const [pick, setPick] = useState("workspace");
  const [targetId, setTargetId] = useState(workspaces[0]?.id ?? accounts[0]?.id ?? 0);
  return (
    <li className="border border-hairline rounded p-2.5 space-y-2">
      <div className="flex justify-between items-center">
        <span className="text-sm">
          <span className="text-ink">{recipient.name}</span>{" "}
          <span className="text-ink-subtle text-xs">({recipient.chatId})</span>
        </span>
        <button
          type="button"
          onClick={async () => {
            await Telegram.DeleteRecipient(recipient.id);
            onChanged();
          }}
          className="text-xs text-ink-subtle hover:text-danger"
        >
          delete
        </button>
      </div>
      <div className="flex gap-1.5 text-xs">
        <select
          value={pick}
          onChange={(e) => setPick(e.target.value)}
          className="px-2 py-1 bg-surface-2 border border-hairline-strong rounded"
        >
          <option value="workspace">workspace</option>
          <option value="account">account</option>
        </select>
        <select
          value={targetId}
          onChange={(e) => setTargetId(Number(e.target.value))}
          className="px-2 py-1 bg-surface-2 border border-hairline-strong rounded"
        >
          {(pick === "workspace" ? workspaces : accounts).map((x) => (
            <option key={x.id} value={x.id}>
              {pick === "workspace" ? `${x.emoji} ${x.name}` : x.emailAddress}
            </option>
          ))}
        </select>
        <button
          type="button"
          onClick={async () => {
            if (pick === "workspace") {
              await Telegram.AssignToWorkspace(targetId, recipient.id);
            } else {
              await Telegram.AssignToAccount(targetId, recipient.id);
            }
          }}
          className="px-2 py-1 rounded bg-brand-hover"
        >
          Assign
        </button>
      </div>
    </li>
  );
}
