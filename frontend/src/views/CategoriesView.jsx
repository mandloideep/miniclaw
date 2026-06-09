import { useCallback, useEffect, useMemo, useState } from "react";
import { Inbox } from "../api";

const CATS = ["promotions", "updates", "social", "newsletter"];

export default function CategoriesView({ workspace }) {
  const [emails, setEmails] = useState([]);
  const [active, setActive] = useState("promotions");

  const refresh = useCallback(async () => {
    if (!workspace) return;
    setEmails(await Inbox.ListByWorkspace(workspace.id, 200));
  }, [workspace]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const grouped = useMemo(() => {
    const m = Object.fromEntries(CATS.map((c) => [c, []]));
    for (const e of emails) {
      if (e.category && m[e.category]) m[e.category].push(e);
    }
    return m;
  }, [emails]);

  if (!workspace) return null;

  return (
    <div className="max-w-3xl space-y-3">
      <div className="flex gap-1.5">
        {CATS.map((c) => (
          <button
            key={c}
            type="button"
            onClick={() => setActive(c)}
            className={`px-3 py-1 rounded text-xs ${
              active === c ? "bg-ink text-canvas" : "bg-surface-2 text-ink-muted"
            }`}
          >
            {c} ({grouped[c].length})
          </button>
        ))}
      </div>
      <ul className="space-y-1.5">
        {grouped[active].map((e) => (
          <li key={e.id} className="p-3 rounded border border-hairline bg-surface-1">
            <div className="text-sm text-ink truncate">
              {e.fromName || e.fromAddress} — {e.subject}
            </div>
            <div className="text-xs text-ink-subtle">{e.receivedAt}</div>
          </li>
        ))}
        {grouped[active].length === 0 && (
          <li className="text-xs text-ink-subtle">No {active} yet.</li>
        )}
      </ul>
    </div>
  );
}
