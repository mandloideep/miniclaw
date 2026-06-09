import {
  ArrowRight,
  Cog,
  Filter,
  Inbox,
  PauseCircle,
  RefreshCw,
  Search,
  Tag,
  Users,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Inbox as InboxApi } from "../api";
import { Badge } from "./ui/badge";
import { Dialog, DialogContent } from "./ui/dialog";

const ACTION_DEFS = [
  { id: "nav:inbox", label: "Go to Inbox", icon: Inbox, hint: "Inbox" },
  {
    id: "nav:put-aside",
    label: "Go to Put aside",
    icon: PauseCircle,
    hint: "Triage",
  },
  { id: "nav:screener", label: "Go to Screener", icon: Users, hint: "Triage" },
  { id: "nav:filters", label: "Go to Filters", icon: Filter, hint: "Triage" },
  {
    id: "nav:categories",
    label: "Go to Categories",
    icon: Tag,
    hint: "Triage",
  },
  {
    id: "sync",
    label: "Sync now (all accounts)",
    icon: RefreshCw,
    hint: "Action",
  },
  { id: "settings", label: "Open Settings", icon: Cog, hint: "Action" },
];

export default function CommandPalette({
  open,
  onOpenChange,
  workspace,
  onNavigate,
  onSync,
  onSettings,
  onOpenEmail,
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState([]);
  const [active, setActive] = useState(0);
  const debounce = useRef(null);

  useEffect(() => {
    if (!open) {
      setQuery("");
      setResults([]);
      setActive(0);
    }
  }, [open]);

  useEffect(() => {
    clearTimeout(debounce.current);
    if (!workspace || query.trim().length < 2) {
      setResults([]);
      return;
    }
    debounce.current = setTimeout(async () => {
      try {
        const hits = await InboxApi.Search(workspace.id, query, 20);
        setResults(hits ?? []);
      } catch {
        setResults([]);
      }
    }, 120);
    return () => clearTimeout(debounce.current);
  }, [query, workspace]);

  const filteredActions = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return ACTION_DEFS;
    return ACTION_DEFS.filter((a) => a.label.toLowerCase().includes(q));
  }, [query]);

  const items = useMemo(
    () => [
      ...filteredActions.map((a) => ({ kind: "action", id: a.id, action: a })),
      ...results.map((r) => ({ kind: "email", id: `email:${r.id}`, email: r })),
    ],
    [filteredActions, results],
  );

  useEffect(() => {
    if (active >= items.length) setActive(0);
  }, [items, active]);

  const runItem = useCallback(
    (it) => {
      if (!it) return;
      if (it.kind === "action") {
        const id = it.action.id;
        if (id.startsWith("nav:")) onNavigate?.(id.slice(4));
        else if (id === "sync") onSync?.();
        else if (id === "settings") onSettings?.();
      } else if (it.kind === "email") {
        onOpenEmail?.(it.email);
      }
      onOpenChange?.(false);
    },
    [onNavigate, onSync, onSettings, onOpenEmail, onOpenChange],
  );

  const onKeyDown = (e) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((a) => Math.min(items.length - 1, a + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => Math.max(0, a - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      runItem(items[active]);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showClose={false} className="top-[12%] max-w-xl">
        <div className="flex items-center gap-2 px-3 h-12 border-b border-hairline">
          <Search className="w-4 h-4 text-ink-subtle" />
          <input
            // biome-ignore lint/a11y/noAutofocus: command palette needs immediate focus
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Search mail or jump to…"
            className="flex-1 bg-transparent text-[14px] outline-none placeholder:text-ink-tertiary"
            aria-label="Command palette query"
          />
          <kbd className="text-[10px] text-ink-tertiary border border-hairline rounded px-1 py-0.5">
            esc
          </kbd>
        </div>
        <div className="flex-1 overflow-auto p-1.5 min-h-[200px]">
          {items.length === 0 ? (
            <p className="text-[12px] text-ink-tertiary px-3 py-6 text-center">
              No matches. Try a different query.
            </p>
          ) : (
            <ul className="flex flex-col">
              {filteredActions.length > 0 && <SectionHeader>Quick actions</SectionHeader>}
              {items.map((it, i) => (
                <li key={it.id}>
                  {i === filteredActions.length && results.length > 0 && (
                    <SectionHeader>Messages</SectionHeader>
                  )}
                  <button
                    type="button"
                    onClick={() => runItem(it)}
                    onMouseEnter={() => setActive(i)}
                    className={
                      "w-full text-left flex items-center gap-2 px-2 py-1.5 rounded-md text-[13px] transition-colors " +
                      (i === active ? "bg-surface-2 text-ink" : "text-ink-muted hover:bg-surface-2")
                    }
                  >
                    {it.kind === "action" ? (
                      <ActionRow action={it.action} />
                    ) : (
                      <EmailRow email={it.email} />
                    )}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="h-8 px-3 flex items-center text-[10px] text-ink-tertiary border-t border-hairline">
          ↑↓ navigate · ⏎ select · esc close
        </div>
      </DialogContent>
    </Dialog>
  );
}

function SectionHeader({ children }) {
  return (
    <div className="px-2 pt-2 pb-1 text-[10px] uppercase tracking-[0.08em] text-ink-tertiary">
      {children}
    </div>
  );
}

function ActionRow({ action }) {
  const Icon = action.icon;
  return (
    <>
      <Icon className="w-3.5 h-3.5 text-ink-subtle" />
      <span>{action.label}</span>
      <Badge variant="muted" className="ml-auto">
        {action.hint}
      </Badge>
    </>
  );
}

function EmailRow({ email }) {
  return (
    <>
      <ArrowRight className="w-3.5 h-3.5 text-ink-tertiary" />
      <span className="min-w-0 flex-1 truncate">
        <span className="text-ink">{email.fromName || email.fromAddress}</span>{" "}
        <span className="text-ink-subtle">— {email.subject || "(no subject)"}</span>
      </span>
      <span className="text-[10px] text-ink-tertiary shrink-0">
        {email.receivedAt ? email.receivedAt.slice(0, 10) : ""}
      </span>
    </>
  );
}
