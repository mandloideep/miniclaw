import { useCallback, useEffect, useState } from "react";
import { Accounts, Digest, GmailOAuth, MSOAuth, Ollama, Telegram, Workspaces } from "../api";

export default function SettingsView({
  workspaces,
  accounts,
  ollamaStatus,
  onWorkspacesChanged,
  onAccountsChanged,
}) {
  return (
    <div className="max-w-3xl space-y-8">
      <WorkspacesSection workspaces={workspaces} onChange={onWorkspacesChanged} />
      <AccountsSection workspaces={workspaces} accounts={accounts} onChange={onAccountsChanged} />
      <OllamaSection status={ollamaStatus} />
      <TelegramSection accounts={accounts} workspaces={workspaces} />
    </div>
  );
}

function Section({ title, children }) {
  return (
    <section className="bg-surface-1 border border-hairline rounded-lg p-4">
      <h2 className="text-base font-medium mb-3">{title}</h2>
      {children}
    </section>
  );
}

function WorkspacesSection({ workspaces, onChange }) {
  const [draft, setDraft] = useState({ name: "", emoji: "" });
  return (
    <Section title="Workspaces">
      <ul className="space-y-1.5 mb-3">
        {workspaces.map((w) => (
          <li
            key={w.id}
            className="flex items-center gap-3 px-2 py-1.5 rounded border border-hairline"
          >
            <span className="text-lg">{w.emoji}</span>
            <span className="flex-1">{w.name}</span>
            <button
              type="button"
              onClick={async () => {
                if (!window.confirm(`Delete workspace "${w.name}"?`)) return;
                await Workspaces.Delete(w.id);
                onChange();
              }}
              className="text-xs text-ink-subtle hover:text-danger"
            >
              delete
            </button>
          </li>
        ))}
      </ul>
      <form
        className="flex gap-2"
        onSubmit={async (e) => {
          e.preventDefault();
          if (!draft.name) return;
          await Workspaces.Create(draft.name, draft.emoji);
          setDraft({ name: "", emoji: "" });
          onChange();
        }}
      >
        <input
          placeholder="emoji"
          value={draft.emoji}
          onChange={(e) => setDraft({ ...draft, emoji: e.target.value })}
          className="w-16 px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        />
        <input
          placeholder="workspace name"
          value={draft.name}
          onChange={(e) => setDraft({ ...draft, name: e.target.value })}
          className="flex-1 px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        />
        <button type="submit" className="px-3 py-1.5 rounded bg-brand text-sm">
          Add
        </button>
      </form>
    </Section>
  );
}

function AccountsSection({ workspaces, accounts, onChange }) {
  const [showAdd, setShowAdd] = useState(false);
  const [models, setModels] = useState([]);
  useEffect(() => {
    Ollama.ListModels()
      .then(setModels)
      .catch(() => setModels([]));
  }, []);
  return (
    <Section title="Accounts">
      <ul className="space-y-1.5 mb-3">
        {accounts.map((a) => {
          const ws = workspaces.find((w) => w.id === a.workspaceId);
          return (
            <li
              key={a.id}
              className="px-3 py-2 border border-hairline rounded flex justify-between items-start"
            >
              <div>
                <div className="text-sm">
                  {a.emailAddress} <span className="text-xs text-ink-subtle">({a.authKind})</span>
                </div>
                <div className="text-xs text-ink-subtle">
                  {ws ? `${ws.emoji} ${ws.name}` : "—"} ·{" "}
                  {a.lastSyncedAt ? `synced ${a.lastSyncedAt}` : "never synced"} · model:{" "}
                  {a.ollamaModel || "(default)"}
                </div>
              </div>
              <button
                type="button"
                onClick={async () => {
                  if (!window.confirm(`Remove ${a.emailAddress}?`)) return;
                  await Accounts.Delete(a.id);
                  onChange();
                }}
                className="text-xs text-ink-subtle hover:text-danger"
              >
                remove
              </button>
            </li>
          );
        })}
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
    <form onSubmit={submit} className="space-y-2 border border-hairline rounded p-3">
      <div className="flex gap-2">
        {["imap", "gmail_oauth", "ms_oauth"].map((k) => (
          <button
            key={k}
            type="button"
            onClick={() => setKind(k)}
            className={`px-3 py-1 text-xs rounded ${
              kind === k ? "bg-ink text-canvas" : "bg-surface-2"
            }`}
          >
            {k === "imap" ? "IMAP/SMTP" : k === "gmail_oauth" ? "Gmail OAuth" : "Microsoft OAuth"}
          </button>
        ))}
      </div>

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

function OllamaSection({ status }) {
  const [models, setModels] = useState([]);
  useEffect(() => {
    Ollama.ListModels()
      .then(setModels)
      .catch(() => setModels([]));
  }, []);
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
        <button
          type="button"
          onClick={() => Digest.RunNow()}
          className="ml-2 px-3 py-1.5 rounded bg-surface-2 text-xs hover:bg-surface-3"
        >
          Send test digest now
        </button>
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
