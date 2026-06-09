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
    return <p className="text-sm text-ink-subtle py-12 text-center">No accounts.</p>;
  }

  return (
    <div className="space-y-6 max-w-2xl">
      {accounts.map((a) => {
        const rows = byAccount[a.id] ?? [];
        return (
          <section key={a.id}>
            <h3 className="text-sm font-medium text-ink-muted mb-2">
              {a.emailAddress}
              <span className="ml-2 text-ink-subtle">({rows.length} pending)</span>
            </h3>
            {rows.length === 0 ? (
              <p className="text-xs text-ink-tertiary">All clear.</p>
            ) : (
              <ul className="space-y-2">
                {rows.map((r) => (
                  <li
                    key={r.senderId}
                    className="p-3 rounded border border-hairline bg-surface-1 flex items-center gap-3"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-ink truncate">{r.address}</div>
                      <div className="text-xs text-ink-subtle truncate">
                        first seen via "{r.subject}"
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={async () => {
                        await Triage.ApproveSender(r.senderId);
                        refresh();
                      }}
                      className="px-2 py-1 text-xs rounded bg-success/15 hover:bg-brand/80 text-success"
                    >
                      Approve
                    </button>
                    <button
                      type="button"
                      onClick={async () => {
                        await Triage.BlockSender(r.senderId);
                        refresh();
                      }}
                      className="px-2 py-1 text-xs rounded bg-danger/15 hover:bg-danger/25 text-danger"
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
