import { useCallback, useEffect, useState } from "react";
import { Triage } from "../api";

export default function ScreenerView({ accounts }) {
  const [byAccount, setByAccount] = useState({});

  const refresh = useCallback(async () => {
    const next = {};
    await Promise.all(
      accounts.map(async (a) => {
        next[a.id] = await Triage.ListUnscreened(a.id);
      }),
    );
    setByAccount(next);
  }, [accounts]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (accounts.length === 0) {
    return <p className="text-sm text-zinc-500 py-12 text-center">No accounts.</p>;
  }

  return (
    <div className="space-y-6 max-w-2xl">
      {accounts.map((a) => {
        const rows = byAccount[a.id] ?? [];
        return (
          <section key={a.id}>
            <h3 className="text-sm font-medium text-zinc-300 mb-2">
              {a.emailAddress}
              <span className="ml-2 text-zinc-500">({rows.length} pending)</span>
            </h3>
            {rows.length === 0 ? (
              <p className="text-xs text-zinc-600">All clear.</p>
            ) : (
              <ul className="space-y-2">
                {rows.map((r) => (
                  <li
                    key={r.senderId}
                    className="p-3 rounded border border-zinc-800 bg-zinc-900 flex items-center gap-3"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-zinc-100 truncate">{r.address}</div>
                      <div className="text-xs text-zinc-500 truncate">
                        first seen via "{r.subject}"
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={async () => {
                        await Triage.ApproveSender(r.senderId);
                        refresh();
                      }}
                      className="px-2 py-1 text-xs rounded bg-emerald-900 hover:bg-emerald-800 text-emerald-100"
                    >
                      Approve
                    </button>
                    <button
                      type="button"
                      onClick={async () => {
                        await Triage.BlockSender(r.senderId);
                        refresh();
                      }}
                      className="px-2 py-1 text-xs rounded bg-rose-900 hover:bg-rose-800 text-rose-100"
                    >
                      Block
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        );
      })}
    </div>
  );
}
