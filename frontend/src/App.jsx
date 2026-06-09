import { useCallback, useEffect, useMemo, useState } from "react";
import { Accounts, Keychain, Ollama, Workspaces } from "./api";
import StatusBar from "./components/StatusBar";
import TabBar from "./components/TabBar";
import CategoriesView from "./views/CategoriesView";
import FiltersView from "./views/FiltersView";
import InboxView from "./views/InboxView";
import PutAsideView from "./views/PutAsideView";
import ScreenerView from "./views/ScreenerView";
import SettingsView from "./views/SettingsView";

const TABS = [
  { id: "inbox", label: "Inbox" },
  { id: "put-aside", label: "Put Aside" },
  { id: "screener", label: "Screener" },
  { id: "filters", label: "Filters" },
  { id: "categories", label: "Categories" },
  { id: "settings", label: "Settings" },
];

export default function App() {
  const [tab, setTab] = useState("inbox");
  const [workspaces, setWorkspaces] = useState([]);
  const [accounts, setAccounts] = useState([]);
  const [activeWorkspaceId, setActiveWorkspaceId] = useState(null);
  const [ollamaStatus, setOllamaStatus] = useState({ running: false });
  const [keychainOk, setKeychainOk] = useState(false);

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
    const t = setInterval(() => Ollama.Status().then(setOllamaStatus), 30000);
    return () => clearInterval(t);
  }, [refreshWorkspaces, refreshAccounts]);

  const activeWorkspace = useMemo(
    () => workspaces.find((w) => w.id === activeWorkspaceId) ?? null,
    [workspaces, activeWorkspaceId],
  );

  const activeAccounts = useMemo(
    () => (activeWorkspace ? accounts.filter((a) => a.workspaceId === activeWorkspace.id) : []),
    [accounts, activeWorkspace],
  );

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <StatusBar ollama={ollamaStatus} keychainOk={keychainOk} />
      <TabBar tabs={TABS} active={tab} onChange={setTab} />

      {tab !== "settings" && workspaces.length > 0 && (
        <WorkspacePicker
          workspaces={workspaces}
          activeId={activeWorkspaceId}
          onPick={setActiveWorkspaceId}
        />
      )}

      <main className="px-6 py-4">
        {tab === "inbox" && <InboxView workspace={activeWorkspace} accounts={activeAccounts} />}
        {tab === "put-aside" && <PutAsideView workspace={activeWorkspace} />}
        {tab === "screener" && <ScreenerView accounts={activeAccounts} />}
        {tab === "filters" && <FiltersView accounts={activeAccounts} />}
        {tab === "categories" && (
          <CategoriesView workspace={activeWorkspace} accounts={activeAccounts} />
        )}
        {tab === "settings" && (
          <SettingsView
            workspaces={workspaces}
            accounts={accounts}
            ollamaStatus={ollamaStatus}
            onWorkspacesChanged={refreshWorkspaces}
            onAccountsChanged={refreshAccounts}
          />
        )}
      </main>
    </div>
  );
}

function WorkspacePicker({ workspaces, activeId, onPick }) {
  return (
    <div className="flex gap-2 px-6 py-3 border-b border-zinc-800 overflow-x-auto">
      {workspaces.map((w) => (
        <button
          key={w.id}
          type="button"
          onClick={() => onPick(w.id)}
          className={`px-3 py-1.5 rounded-full text-sm whitespace-nowrap ${
            w.id === activeId
              ? "bg-zinc-100 text-zinc-900"
              : "bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
          }`}
        >
          {w.emoji} {w.name}
        </button>
      ))}
    </div>
  );
}
