import { useCallback, useEffect, useState } from "react";
import { Triage } from "../api";

export default function FiltersView({ accounts }) {
  const [active, setActive] = useState(null);
  const [rules, setRules] = useState([]);
  const [draft, setDraft] = useState({
    pattern: "",
    reason: "",
    kind: "block",
  });

  useEffect(() => {
    if (!active && accounts.length) setActive(accounts[0]);
  }, [accounts, active]);

  const refresh = useCallback(async () => {
    if (!active) return;
    setRules(await Triage.ListFilterRules(active.id));
  }, [active]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (accounts.length === 0) {
    return <p className="text-sm text-ink-subtle py-12 text-center">No accounts.</p>;
  }

  return (
    <div className="max-w-2xl space-y-4">
      <div className="flex gap-2 flex-wrap">
        {accounts.map((a) => (
          <button
            key={a.id}
            type="button"
            onClick={() => setActive(a)}
            className={`px-3 py-1 rounded text-xs ${
              active?.id === a.id ? "bg-ink text-canvas" : "bg-surface-2 text-ink-muted"
            }`}
          >
            {a.emailAddress}
          </button>
        ))}
      </div>

      <form
        className="flex gap-2 items-end"
        onSubmit={async (e) => {
          e.preventDefault();
          if (!active || !draft.pattern) return;
          await Triage.AddFilterRule(active.id, draft.kind, draft.pattern, draft.reason);
          setDraft({ pattern: "", reason: "", kind: "block" });
          refresh();
        }}
      >
        <select
          value={draft.kind}
          onChange={(e) => setDraft({ ...draft, kind: e.target.value })}
          className="px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        >
          <option value="block">block</option>
          <option value="allow">allow</option>
        </select>
        <input
          value={draft.pattern}
          placeholder="user@host or @host or host"
          onChange={(e) => setDraft({ ...draft, pattern: e.target.value })}
          className="flex-1 px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        />
        <input
          value={draft.reason}
          placeholder="reason (optional)"
          onChange={(e) => setDraft({ ...draft, reason: e.target.value })}
          className="flex-1 px-2 py-1.5 bg-surface-2 border border-hairline-strong rounded text-sm"
        />
        <button type="submit" className="px-3 py-1.5 rounded bg-brand text-sm">
          Add
        </button>
      </form>

      <ul className="space-y-1.5">
        {rules.map((r) => (
          <li
            key={r.id}
            className="px-3 py-2 rounded border border-hairline bg-surface-1 flex justify-between items-center"
          >
            <span className="text-sm">
              <span
                className={`mr-2 text-xs px-1.5 py-0.5 rounded ${
                  r.kind === "block" ? "bg-danger/15 text-danger" : "bg-success/15 text-success"
                }`}
              >
                {r.kind}
              </span>
              {r.pattern}
              {r.reason && <span className="text-ink-subtle"> — {r.reason}</span>}
            </span>
            <button
              type="button"
              onClick={async () => {
                await Triage.DeleteFilterRule(r.id);
                refresh();
              }}
              className="text-xs text-ink-subtle hover:text-danger"
            >
              delete
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
