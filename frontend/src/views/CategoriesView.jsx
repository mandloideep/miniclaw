import { LoaderCircle, Sparkles } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Categories, Inbox } from "../api";
import { Button } from "../components/ui/button";

const CATS = ["promotions", "updates", "social", "newsletter"];

export default function CategoriesView({ workspace, accounts }) {
  const [emails, setEmails] = useState([]);
  const [active, setActive] = useState("promotions");
  const [reclassifying, setReclassifying] = useState(false);
  const [lastResult, setLastResult] = useState(null);

  const refresh = useCallback(async () => {
    if (!workspace) return;
    setEmails(await Inbox.ListByWorkspace(workspace.id, 200));
  }, [workspace]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const reclassify = useCallback(async () => {
    if (!accounts?.length || reclassifying) return;
    setReclassifying(true);
    setLastResult(null);
    try {
      const counts = await Promise.all(
        accounts.map((a) => Categories.ClassifyAccount(a.id).catch(() => 0)),
      );
      const total = counts.reduce((acc, n) => acc + (n || 0), 0);
      setLastResult(`Classified ${total} new email${total === 1 ? "" : "s"}.`);
      await refresh();
    } finally {
      setReclassifying(false);
    }
  }, [accounts, reclassifying, refresh]);

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
      <div className="flex items-center gap-2">
        <div className="flex flex-wrap gap-1.5">
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
        <div className="ml-auto flex items-center gap-2">
          {lastResult && <span className="text-xs text-ink-subtle">{lastResult}</span>}
          <Button
            size="sm"
            variant="secondary"
            onClick={reclassify}
            disabled={reclassifying || !accounts?.length}
            title="Re-run the local rule pack against the last 500 emails per account"
          >
            {reclassifying ? (
              <LoaderCircle className="w-3.5 h-3.5 animate-spin" />
            ) : (
              <Sparkles className="w-3.5 h-3.5" />
            )}
            {reclassifying ? "Classifying…" : "Re-classify"}
          </Button>
        </div>
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
