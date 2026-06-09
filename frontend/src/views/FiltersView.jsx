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
    return <p className="text-sm text-zinc-500 py-12 text-center">No accounts.</p>;
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
              active?.id === a.id ? "bg-zinc-100 text-zinc-900" : "bg-zinc-800 text-zinc-300"
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
          className="px-2 py-1.5 bg-zinc-800 border border-zinc-700 rounded text-sm"
        >
          <option value="block">block</option>
          <option value="allow">allow</option>
        </select>
        <input
          value={draft.pattern}
          placeholder="user@host or @host or host"
          onChange={(e) => setDraft({ ...draft, pattern: e.target.value })}
          className="flex-1 px-2 py-1.5 bg-zinc-800 border border-zinc-700 rounded text-sm"
        />
        <input
          value={draft.reason}
          placeholder="reason (optional)"
          onChange={(e) => setDraft({ ...draft, reason: e.target.value })}
          className="flex-1 px-2 py-1.5 bg-zinc-800 border border-zinc-700 rounded text-sm"
        />
        <button type="submit" className="px-3 py-1.5 rounded bg-emerald-700 text-sm">
          Add
        </button>
      </form>

      <ul className="space-y-1.5">
        {rules.map((r) => (
          <li
            key={r.id}
            className="px-3 py-2 rounded border border-zinc-800 bg-zinc-900 flex justify-between items-center"
          >
            <span className="text-sm">
              <span
                className={`mr-2 text-xs px-1.5 py-0.5 rounded ${
                  r.kind === "block"
                    ? "bg-rose-900 text-rose-100"
                    : "bg-emerald-900 text-emerald-100"
                }`}
              >
                {r.kind}
              </span>
              {r.pattern}
              {r.reason && <span className="text-zinc-500"> — {r.reason}</span>}
            </span>
            <button
              type="button"
              onClick={async () => {
                await Triage.DeleteFilterRule(r.id);
                refresh();
              }}
              className="text-xs text-zinc-500 hover:text-rose-400"
            >
              delete
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
