import { Clock, PlayCircle } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Snooze } from "../api";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";

// SnoozedView lists every email currently hidden under a snoozed_until
// stamp inside this workspace. The ticker wakes them automatically, but
// the user can wake one early or just see what's pending.
export default function SnoozedView({ workspace }) {
  const [rows, setRows] = useState([]);

  const refresh = useCallback(async () => {
    if (!workspace) {
      setRows([]);
      return;
    }
    setRows(await Snooze.ListSnoozed(workspace.id));
  }, [workspace]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (!workspace) {
    return <p className="text-[13px] text-ink-subtle">Pick a workspace.</p>;
  }

  if (rows.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center text-center py-12 text-ink-subtle">
        <Clock className="w-5 h-5 mb-3 text-ink-tertiary" />
        <p className="text-sm">Nothing is snoozed right now.</p>
        <p className="text-[12px] text-ink-tertiary mt-1">
          Use the Snooze menu in the reader to hide a message until later.
        </p>
      </div>
    );
  }

  return (
    <ul className="space-y-1.5">
      {rows.map((r) => (
        <li
          key={r.emailId}
          className="px-3 py-2.5 border border-hairline rounded-md bg-surface-1 flex items-start gap-3"
        >
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 min-w-0">
              <span className="text-[13px] text-ink truncate">{r.fromName || r.fromAddress}</span>
              <Badge variant="muted" className="text-[10px]">
                <Clock className="w-2.5 h-2.5" />
                wakes {formatWake(r.snoozedUntil)}
              </Badge>
            </div>
            <div className="text-[13px] text-ink-muted truncate mt-0.5">
              {r.subject || <span className="italic text-ink-tertiary">(no subject)</span>}
            </div>
          </div>
          <Button
            size="xs"
            variant="ghost"
            onClick={async () => {
              await Snooze.Unsnooze(r.emailId);
              refresh();
            }}
          >
            <PlayCircle className="w-3 h-3" />
            Wake now
          </Button>
        </li>
      ))}
    </ul>
  );
}

function formatWake(iso) {
  if (!iso) return "?";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}
