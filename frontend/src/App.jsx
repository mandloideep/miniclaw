import { Events } from "@wailsio/runtime";
import {
  AtSign,
  Bell,
  CalendarDays,
  Clock,
  Cog,
  Filter,
  Inbox as InboxIcon,
  LoaderCircle,
  PauseCircle,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
  Tag,
  Users,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Accounts, GmailOAuth, IMAPSync, Keychain, MSOAuth, Ollama, Workspaces } from "./api";
import CommandPalette from "./components/CommandPalette";
import { Badge } from "./components/ui/badge";
import { Button } from "./components/ui/button";
import { Separator } from "./components/ui/separator";
import CategoriesView from "./views/CategoriesView";
import FiltersView from "./views/FiltersView";
import InboxView from "./views/InboxView";
import PlannerView from "./views/PlannerView";
import PutAsideView from "./views/PutAsideView";
import ScreenerView from "./views/ScreenerView";
import SettingsView from "./views/SettingsView";
import SnoozedView from "./views/SnoozedView";

const NAV = [
  { id: "inbox", label: "Inbox", icon: InboxIcon },
  { id: "put-aside", label: "Put aside", icon: PauseCircle },
  { id: "snoozed", label: "Snoozed", icon: Clock },
  { id: "screener", label: "Screener", icon: Users },
  { id: "filters", label: "Filters", icon: Filter },
  { id: "categories", label: "Categories", icon: Tag },
  { id: "planner", label: "Planner", icon: CalendarDays },
];

export default function App() {
  const [nav, setNav] = useState("inbox");
  const [workspaces, setWorkspaces] = useState([]);
  const [accounts, setAccounts] = useState([]);
  const [activeWorkspaceId, setActiveWorkspaceId] = useState(null);
  const [ollamaStatus, setOllamaStatus] = useState({ running: false });
  const [keychainOk, setKeychainOk] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [syncToast, setSyncToast] = useState(null);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [pendingEmailId, setPendingEmailId] = useState(null);

  const refreshWorkspaces = useCallback(async () => {
    const ws = await Workspaces.List();
    setWorkspaces(ws);
    if (ws.length && activeWorkspaceId == null) setActiveWorkspaceId(ws[0].id);
  }, [activeWorkspaceId]);

  const refreshAccounts = useCallback(async () => {
    setAccounts(await Accounts.List());
  }, []);

  useEffect(() => {
    refreshWorkspaces();
    refreshAccounts();
    Ollama.Status().then(setOllamaStatus);
    Keychain.Available().then(setKeychainOk);
    // 60s — the status pill only changes when Ollama is restarted; a busier
    // poll just adds noise to the Ollama server log.
    const t = setInterval(() => Ollama.Status().then(setOllamaStatus), 60000);
    return () => clearInterval(t);
  }, [refreshWorkspaces, refreshAccounts]);

  useEffect(() => {
    const onKey = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    // Live ingest status. Scheduler emits start → done/error per account.
    // We let "done" toasts fade after a few seconds; errors stick until
    // the next event replaces them.
    let fadeT;
    const off = Events.On("sync_progress", (ev) => {
      const p = ev?.data?.[0] ?? ev?.data ?? {};
      clearTimeout(fadeT);
      setSyncToast(p);
      if (p?.phase === "done") {
        fadeT = setTimeout(() => setSyncToast(null), 4000);
      }
    });
    return () => {
      clearTimeout(fadeT);
      off?.();
    };
  }, []);

  const activeWorkspace = useMemo(
    () => workspaces.find((w) => w.id === activeWorkspaceId) ?? null,
    [workspaces, activeWorkspaceId],
  );

  const activeAccounts = useMemo(
    () => (activeWorkspace ? accounts.filter((a) => a.workspaceId === activeWorkspace.id) : []),
    [accounts, activeWorkspace],
  );

  const syncAll = useCallback(async () => {
    if (!activeAccounts.length) return;
    setSyncing(true);
    try {
      await Promise.all(
        activeAccounts.map(async (a) => {
          if (a.authKind === "gmail_oauth") await GmailOAuth.SyncNow(a.id).catch(() => {});
          if (a.authKind === "ms_oauth") await MSOAuth.Sync?.(a.id).catch(() => {});
          if (a.authKind === "imap") await IMAPSync.SyncNow(a.id).catch(() => {});
        }),
      );
    } finally {
      setSyncing(false);
    }
  }, [activeAccounts]);

  return (
    <div className="min-h-screen flex flex-col bg-canvas text-ink">
      <TopBar
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        onPick={setActiveWorkspaceId}
        ollama={ollamaStatus}
        keychainOk={keychainOk}
        syncing={syncing}
        syncToast={syncToast}
        onSync={syncAll}
        onSettings={() => setShowSettings(true)}
      />
      <CommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        workspace={activeWorkspace}
        onNavigate={(id) => {
          setShowSettings(false);
          setNav(id);
        }}
        onSync={syncAll}
        onSettings={() => setShowSettings(true)}
        onOpenEmail={(em) => {
          setShowSettings(false);
          setNav("inbox");
          setPendingEmailId(em.id);
        }}
      />

      {showSettings ? (
        <main className="flex-1 overflow-auto px-8 py-6">
          <div className="max-w-3xl mx-auto">
            <div className="flex items-center justify-between mb-6">
              <h1 className="display text-2xl">Settings</h1>
              <Button variant="secondary" onClick={() => setShowSettings(false)}>
                Done
              </Button>
            </div>
            <SettingsView
              workspaces={workspaces}
              accounts={accounts}
              ollamaStatus={ollamaStatus}
              onWorkspacesChanged={refreshWorkspaces}
              onAccountsChanged={refreshAccounts}
            />
          </div>
        </main>
      ) : (
        <div className="flex-1 flex min-h-0">
          <SideRail
            nav={nav}
            onNav={setNav}
            accounts={activeAccounts}
            workspace={activeWorkspace}
            onSearch={() => setPaletteOpen(true)}
          />
          <main className="flex-1 min-w-0 flex flex-col bg-canvas">
            {nav === "inbox" && (
              <InboxView
                workspace={activeWorkspace}
                accounts={activeAccounts}
                openEmailId={pendingEmailId}
                onEmailOpened={() => setPendingEmailId(null)}
              />
            )}
            {nav === "put-aside" && (
              <PaneShell title="Put aside" subtitle="Saved for later, out of the main flow.">
                <PutAsideView workspace={activeWorkspace} />
              </PaneShell>
            )}
            {nav === "snoozed" && (
              <PaneShell title="Snoozed" subtitle="Hidden until their wake time.">
                <SnoozedView workspace={activeWorkspace} />
              </PaneShell>
            )}
            {nav === "screener" && (
              <PaneShell title="Screener" subtitle="First-time senders waiting on a decision.">
                <ScreenerView accounts={activeAccounts} />
              </PaneShell>
            )}
            {nav === "filters" && (
              <PaneShell title="Filters" subtitle="Block rules, applied at ingest time.">
                <FiltersView accounts={activeAccounts} />
              </PaneShell>
            )}
            {nav === "categories" && (
              <PaneShell title="Categories" subtitle="Promotions, social, updates, newsletters.">
                <CategoriesView workspace={activeWorkspace} accounts={activeAccounts} />
              </PaneShell>
            )}
            {nav === "planner" && (
              <PaneShell
                title="Planner"
                subtitle="Calendar blocks, todos, notes — workspace-local."
              >
                <PlannerView workspace={activeWorkspace} accounts={activeAccounts} />
              </PaneShell>
            )}
          </main>
        </div>
      )}
    </div>
  );
}

function TopBar({
  workspaces,
  activeWorkspaceId,
  onPick,
  ollama,
  keychainOk,
  syncing,
  syncToast,
  onSync,
  onSettings,
}) {
  const toastLabel = useMemo(() => {
    if (!syncToast) return null;
    const who = syncToast.emailAddress || `account ${syncToast.accountId}`;
    if (syncToast.phase === "start") return `Syncing ${who}…`;
    if (syncToast.phase === "done") {
      return syncToast.written > 0 ? `${syncToast.written} new from ${who}` : `${who} up to date`;
    }
    if (syncToast.phase === "error") return `Sync failed for ${who}`;
    return null;
  }, [syncToast]);
  return (
    <header className="flex items-center gap-3 px-4 h-14 border-b border-hairline bg-canvas">
      <div className="flex items-center gap-2 pr-3">
        <span className="inline-flex items-center justify-center w-7 h-7 rounded-md bg-brand/15 text-brand">
          <Sparkles className="w-4 h-4" />
        </span>
        <span className="display text-[15px] font-medium tracking-[-0.01em]">miniclaw</span>
      </div>
      <Separator orientation="vertical" className="h-6" />
      <WorkspaceStrip workspaces={workspaces} activeId={activeWorkspaceId} onPick={onPick} />
      <div className="ml-auto flex items-center gap-2">
        {toastLabel && (
          <span
            className={
              "hidden md:inline-flex items-center gap-1.5 text-[11px] px-2 py-0.5 rounded-full border " +
              (syncToast?.phase === "error"
                ? "border-danger/40 text-danger bg-danger/10"
                : syncToast?.phase === "done"
                  ? "border-success/40 text-success bg-success/10"
                  : "border-brand-focus/40 text-brand bg-brand/10")
            }
            title={syncToast?.err || toastLabel}
          >
            {syncToast?.phase === "start" && <LoaderCircle className="w-3 h-3 animate-spin" />}
            {toastLabel}
          </span>
        )}
        <StatusPill
          ok={ollama?.running}
          label={ollama?.running ? "Ollama online" : "Ollama offline"}
        />
        <StatusPill ok={keychainOk} label={keychainOk ? "Keychain ready" : "Keychain blocked"} />
        <Button
          size="sm"
          variant="secondary"
          onClick={onSync}
          disabled={syncing}
          aria-label="Sync now"
        >
          {syncing ? (
            <LoaderCircle className="w-3.5 h-3.5 animate-spin" />
          ) : (
            <RefreshCw className="w-3.5 h-3.5" />
          )}
          Sync
        </Button>
        <Button size="sm" variant="ghost" onClick={onSettings} aria-label="Settings">
          <Cog className="w-4 h-4" />
        </Button>
      </div>
    </header>
  );
}

function WorkspaceStrip({ workspaces, activeId, onPick }) {
  if (!workspaces.length) return <div className="text-xs text-ink-subtle">No workspaces yet.</div>;
  return (
    <div className="flex items-center gap-1 overflow-x-auto">
      {workspaces.map((w) => {
        const active = w.id === activeId;
        return (
          <button
            key={w.id}
            type="button"
            onClick={() => onPick(w.id)}
            className={
              "h-7 px-2.5 rounded-full text-[12px] inline-flex items-center gap-1.5 whitespace-nowrap transition-colors " +
              (active
                ? "bg-surface-2 text-ink border border-hairline-strong"
                : "text-ink-subtle hover:bg-surface-1")
            }
          >
            {w.emoji ? <span aria-hidden>{w.emoji}</span> : null}
            <span>{w.name}</span>
          </button>
        );
      })}
    </div>
  );
}

function StatusPill({ ok, label }) {
  return (
    <span className="hidden md:inline-flex items-center gap-1.5 text-[11px] text-ink-subtle">
      <span className={`w-1.5 h-1.5 rounded-full ${ok ? "bg-success" : "bg-danger"}`} aria-hidden />
      {label}
    </span>
  );
}

function SideRail({ nav, onNav, accounts, workspace, onSearch }) {
  return (
    <aside className="w-60 shrink-0 border-r border-hairline bg-canvas flex flex-col">
      <div className="px-3 pt-4 pb-2">
        <button
          type="button"
          onClick={onSearch}
          className="w-full inline-flex items-center gap-2 h-8 px-2 rounded-md text-[12px] text-ink-subtle bg-surface-1 border border-hairline hover:bg-surface-2"
        >
          <Search className="w-3.5 h-3.5" />
          <span>Search</span>
          <span className="ml-auto text-[10px] text-ink-tertiary">⌘K</span>
        </button>
      </div>

      <nav className="px-2 pb-2">
        {NAV.map(({ id, label, icon: Icon }) => {
          const active = nav === id;
          return (
            <button
              key={id}
              type="button"
              onClick={() => onNav(id)}
              className={
                "w-full h-8 px-2 rounded-md inline-flex items-center gap-2 text-[13px] transition-colors " +
                (active
                  ? "bg-surface-1 text-ink"
                  : "text-ink-subtle hover:bg-surface-1 hover:text-ink")
              }
            >
              <Icon className="w-3.5 h-3.5" />
              {label}
            </button>
          );
        })}
      </nav>

      <Separator className="mx-3 my-2" />

      <div className="px-3 pb-2">
        <div className="text-[10px] uppercase tracking-[0.08em] text-ink-tertiary mb-2 px-2">
          {workspace ? `${workspace.name} accounts` : "Accounts"}
        </div>
        <ul className="space-y-0.5">
          {accounts.length === 0 && (
            <li className="text-[12px] text-ink-tertiary px-2 py-1">No accounts yet.</li>
          )}
          {accounts.map((a) => (
            <li
              key={a.id}
              className="px-2 py-1.5 rounded-md hover:bg-surface-1 text-[12px] text-ink-muted flex items-center gap-2"
              title={a.emailAddress}
            >
              <AuthGlyph kind={a.authKind} />
              <span className="truncate">{a.emailAddress}</span>
            </li>
          ))}
        </ul>
      </div>

      <div className="mt-auto px-3 py-3 border-t border-hairline">
        <div className="flex items-center gap-2 text-[11px] text-ink-tertiary">
          <ShieldCheck className="w-3.5 h-3.5" />
          local-only · keychain-backed
        </div>
      </div>
    </aside>
  );
}

function AuthGlyph({ kind }) {
  if (kind === "gmail_oauth") {
    return <AtSign className="w-3 h-3 text-brand" aria-label="Gmail" />;
  }
  if (kind === "ms_oauth") {
    return <AtSign className="w-3 h-3 text-brand-secure" aria-label="Microsoft" />;
  }
  return <AtSign className="w-3 h-3 text-ink-tertiary" aria-label="IMAP" />;
}

function PaneShell({ title, subtitle, children, action }) {
  return (
    <div className="flex flex-col h-full min-h-0">
      <div className="flex items-end justify-between px-6 pt-5 pb-4 border-b border-hairline">
        <div>
          <h2 className="display text-lg leading-none tracking-[-0.015em]">{title}</h2>
          {subtitle && <p className="text-[12px] text-ink-subtle mt-1">{subtitle}</p>}
        </div>
        {action}
      </div>
      <div className="flex-1 overflow-auto px-6 py-5">{children}</div>
    </div>
  );
}

// Bell + Badge are imported but only used in InboxView. Re-export so the
// rest of the tree can share the icon set without re-importing lucide.
export { Badge, Bell };
