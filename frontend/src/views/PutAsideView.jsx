import { useCallback, useEffect, useState } from "react";
import { Triage } from "../api";

export default function PutAsideView({ workspace }) {
  const [rows, setRows] = useState([]);

  const refresh = useCallback(async () => {
    if (!workspace) return;
    setRows(await Triage.ListPutAside(workspace.id));
  }, [workspace]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (!workspace) return null;
  if (rows.length === 0) {
    return (
      <p className="text-sm text-ink-subtle py-12 text-center">
        Nothing put aside in {workspace.name}.
      </p>
    );
  }

  return (
    <ul className="space-y-2 max-w-2xl">
      {rows.map((r) => (
        <li
          key={r.emailId}
          className="p-3 rounded border border-hairline bg-surface-1 flex items-start justify-between gap-3"
        >
          <div className="min-w-0">
            <div className="text-sm truncate">
              <span className="text-ink">{r.fromName || r.fromAddress}</span>{" "}
              <span className="text-ink-subtle">— {r.subject}</span>
            </div>
            <div className="text-xs text-ink-subtle">{r.receivedAt}</div>
          </div>
          <button
            type="button"
            onClick={async () => {
              await Triage.TogglePutAside(r.emailId);
              refresh();
            }}
            className="text-xs text-ink-subtle hover:text-ink"
          >
            unstash
          </button>
        </li>
      ))}
    </ul>
  );
}
