import { useCallback, useEffect, useState } from "react";
import { Inbox, Triage } from "../api";

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
  if (!email) {
    return (
      <div className="text-zinc-500 text-sm flex items-center justify-center min-h-[40vh]">
        Pick a message to read it.
      </div>
    );
  }
  return (
    <article className="bg-zinc-900 border border-zinc-800 rounded p-4 max-h-[70vh] overflow-y-auto">
      <h2 className="text-lg font-medium mb-1">{email.subject}</h2>
      <div className="text-xs text-zinc-500 mb-4">
        {email.fromName ? `${email.fromName} — ` : ""}
        {email.fromAddress} · {email.receivedAt}
      </div>
      <pre className="whitespace-pre-wrap text-sm text-zinc-200 font-sans">{email.bodyPlain}</pre>
    </article>
  );
}

function Empty({ children }) {
  return <div className="text-zinc-500 text-sm py-12 text-center">{children}</div>;
}
