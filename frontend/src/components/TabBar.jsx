export default function TabBar({ tabs, active, onChange }) {
  return (
    <nav className="flex gap-1 px-4 border-b border-zinc-800 bg-zinc-900">
      {tabs.map((t) => (
        <button
          key={t.id}
          type="button"
          onClick={() => onChange(t.id)}
          className={`px-4 py-2.5 text-sm border-b-2 -mb-px transition-colors ${
            t.id === active
              ? "border-emerald-400 text-zinc-100"
              : "border-transparent text-zinc-400 hover:text-zinc-200"
          }`}
        >
          {t.label}
        </button>
      ))}
    </nav>
  );
}
