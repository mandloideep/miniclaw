export default function StatusBar({ ollama, keychainOk }) {
  return (
    <header className="flex items-center justify-between px-6 py-3 border-b border-zinc-800 bg-zinc-900">
      <div className="flex items-center gap-3">
        <span className="text-lg font-semibold">miniclaw</span>
        <span className="text-xs text-zinc-500">local email triage</span>
      </div>
      <div className="flex items-center gap-4 text-xs">
        <Dot ok={ollama?.running}>
          Ollama {ollama?.running ? `v${ollama.version}` : (ollama?.error ?? "offline")}
        </Dot>
        <Dot ok={keychainOk}>Keychain {keychainOk ? "ok" : "blocked"}</Dot>
      </div>
    </header>
  );
}

function Dot({ ok, children }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className={`w-2 h-2 rounded-full ${ok ? "bg-emerald-500" : "bg-rose-500"}`} />
      <span className="text-zinc-300">{children}</span>
    </span>
  );
}
