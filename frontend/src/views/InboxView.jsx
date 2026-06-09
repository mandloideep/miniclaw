import { useCallback, useEffect, useState } from "react";
import { Inbox, SMTPSender, Triage } from "../api";

export default function InboxView({ workspace, accounts }) {
  const [emails, setEmails] = useState([]);
  const [selected, setSelected] = useState(null);

  const refresh = useCallback(async () => {
    if (!workspace) {
      setEmails([]);
      return;
    }
    setEmails(await Inbox.ListByWorkspace(workspace.id, 100));
  }, [workspace]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (!workspace) {
    return <Empty>Pick a workspace to see its inbox.</Empty>;
  }
  if (accounts.length === 0) {
    return (
      <Empty>
        No accounts in {workspace.name}. Add one from{" "}
        <span className="text-emerald-400">Settings</span>.
      </Empty>
    );
  }
  if (emails.length === 0) {
    return <Empty>Inbox is empty. Sync runs on cadence — check back soon.</Empty>;
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-[2fr_3fr] gap-4">
      <ul className="space-y-1.5 overflow-y-auto max-h-[70vh]">
        {emails.map((e) => (
          <EmailRow
            key={e.id}
            email={e}
            active={selected?.id === e.id}
            onClick={async () => {
              setSelected(e);
              if (!e.isRead) {
                await Inbox.MarkRead(e.id);
                refresh();
              }
            }}
            onPutAside={async () => {
              await Triage.TogglePutAside(e.id);
              refresh();
            }}
          />
        ))}
      </ul>
      <EmailDetail email={selected} />
    </div>
  );
}

function EmailRow({ email, active, onClick, onPutAside }) {
  const who = email.fromName || email.fromAddress;
  return (
    <li
      className={`p-3 rounded border cursor-pointer ${
        active ? "bg-zinc-800 border-zinc-700" : "bg-zinc-900 border-zinc-800 hover:border-zinc-700"
      }`}
    >
      <button type="button" className="w-full text-left" onClick={onClick}>
        <div className="flex items-baseline justify-between gap-2">
          <span
            className={`text-sm truncate ${
              email.isRead ? "text-zinc-400" : "text-zinc-100 font-medium"
            }`}
          >
            {who}
          </span>
          {email.category && (
            <span className="text-[10px] uppercase tracking-wide text-zinc-500">
              {email.category}
            </span>
          )}
        </div>
        <div className="text-sm text-zinc-300 truncate">{email.subject}</div>
        <div className="text-xs text-zinc-500 truncate">{email.bodyPlain}</div>
      </button>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          onPutAside();
        }}
        className="mt-1 text-[11px] text-zinc-500 hover:text-zinc-300"
      >
        {email.isPutAside ? "↩ unstash" : "→ put aside"}
      </button>
    </li>
  );
}

function EmailDetail({ email }) {
  const [replying, setReplying] = useState(false);

  if (!email) {
    return (
      <div className="text-zinc-500 text-sm flex items-center justify-center min-h-[40vh]">
        Pick a message to read it.
      </div>
    );
  }
  return (
    <article className="bg-zinc-900 border border-zinc-800 rounded p-4 max-h-[70vh] overflow-y-auto">
      <div className="flex items-start justify-between gap-3 mb-1">
        <h2 className="text-lg font-medium">{email.subject}</h2>
        {!replying && (
          <button
            type="button"
            onClick={() => setReplying(true)}
            className="text-xs px-3 py-1.5 rounded bg-emerald-700 hover:bg-emerald-600 whitespace-nowrap"
          >
            Reply
          </button>
        )}
      </div>
      <div className="text-xs text-zinc-500 mb-4">
        {email.fromName ? `${email.fromName} — ` : ""}
        {email.fromAddress} · {email.receivedAt}
      </div>
      <pre className="whitespace-pre-wrap text-sm text-zinc-200 font-sans">{email.bodyPlain}</pre>

      {replying && (
        <ReplyComposer
          email={email}
          onClose={() => setReplying(false)}
          onSent={() => setReplying(false)}
        />
      )}
    </article>
  );
}

function ReplyComposer({ email, onClose, onSent }) {
  const [body, setBody] = useState(
    `\n\nOn ${email.receivedAt}, ${email.fromName || email.fromAddress} wrote:\n> ${email.bodyPlain.split("\n").join("\n> ")}`,
  );
  const [subject, setSubject] = useState(
    email.subject.startsWith("Re:") ? email.subject : `Re: ${email.subject}`,
  );
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function send() {
    setBusy(true);
    setErr("");
    try {
      await SMTPSender.Send(email.accountId, {
        to: [email.fromAddress],
        cc: [],
        subject,
        body,
      });
      onSent();
    } catch (e) {
      setErr(String(e?.message ?? e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mt-4 border border-zinc-700 rounded p-3 bg-zinc-950">
      <div className="text-xs text-zinc-500 mb-2">
        To: <span className="text-zinc-300">{email.fromAddress}</span>
      </div>
      <input
        value={subject}
        onChange={(e) => setSubject(e.target.value)}
        className="w-full px-2 py-1.5 mb-2 bg-zinc-900 border border-zinc-700 rounded text-sm"
      />
      <textarea
        rows={10}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        className="w-full px-2 py-1.5 bg-zinc-900 border border-zinc-700 rounded text-sm font-mono"
      />
      {err && <p className="mt-2 text-xs text-rose-400">{err}</p>}
      <div className="flex gap-2 mt-2">
        <button
          type="button"
          onClick={send}
          disabled={busy}
          className="px-3 py-1.5 rounded bg-emerald-700 text-sm disabled:opacity-50"
        >
          {busy ? "Sending…" : "Send"}
        </button>
        <button type="button" onClick={onClose} className="px-3 py-1.5 rounded bg-zinc-800 text-sm">
          Cancel
        </button>
      </div>
    </div>
  );
}

function Empty({ children }) {
  return <div className="text-zinc-500 text-sm py-12 text-center">{children}</div>;
}
