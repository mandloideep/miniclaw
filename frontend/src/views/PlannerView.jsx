import { CalendarDays, CheckSquare, NotebookPen, Plus, Trash2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { Calendar, Notes, Todos } from "../api";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/ui/tabs";
import { Textarea } from "../components/ui/textarea";

// PlannerView is the unified "non-email" surface: time blocks, todos, and
// notes share a single nav slot since each is workspace-scoped and the
// three feel like one tab in practice ("the rest of my day").
export default function PlannerView({ workspace }) {
  if (!workspace) {
    return <p className="text-[13px] text-ink-subtle">Pick a workspace.</p>;
  }
  return (
    <Tabs defaultValue="calendar" className="w-full">
      <TabsList>
        <TabsTrigger value="calendar">
          <CalendarDays className="w-3.5 h-3.5" />
          Calendar
        </TabsTrigger>
        <TabsTrigger value="todos">
          <CheckSquare className="w-3.5 h-3.5" />
          Todos
        </TabsTrigger>
        <TabsTrigger value="notes">
          <NotebookPen className="w-3.5 h-3.5" />
          Notes
        </TabsTrigger>
      </TabsList>
      <TabsContent value="calendar">
        <CalendarPane workspace={workspace} />
      </TabsContent>
      <TabsContent value="todos">
        <TodosPane workspace={workspace} />
      </TabsContent>
      <TabsContent value="notes">
        <NotesPane workspace={workspace} />
      </TabsContent>
    </Tabs>
  );
}

function CalendarPane({ workspace }) {
  const [blocks, setBlocks] = useState([]);
  const [draft, setDraft] = useState({
    title: "",
    startAt: "",
    endAt: "",
    kind: "block",
  });

  const refresh = useCallback(async () => {
    setBlocks(await Calendar.List(workspace.id));
  }, [workspace.id]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function submit(e) {
    e.preventDefault();
    if (!draft.title || !draft.startAt || !draft.endAt) return;
    await Calendar.Create(
      workspace.id,
      draft.title,
      "",
      new Date(draft.startAt).toISOString(),
      new Date(draft.endAt).toISOString(),
      draft.kind,
    );
    setDraft({ title: "", startAt: "", endAt: "", kind: "block" });
    refresh();
  }

  return (
    <div className="space-y-4">
      <form
        onSubmit={submit}
        className="border border-hairline rounded-md p-3 bg-surface-1 grid grid-cols-1 md:grid-cols-5 gap-2 items-end"
      >
        <Input
          placeholder="Block title"
          value={draft.title}
          onChange={(e) => setDraft({ ...draft, title: e.target.value })}
          className="md:col-span-2"
        />
        <Input
          type="datetime-local"
          value={draft.startAt}
          onChange={(e) => setDraft({ ...draft, startAt: e.target.value })}
        />
        <Input
          type="datetime-local"
          value={draft.endAt}
          onChange={(e) => setDraft({ ...draft, endAt: e.target.value })}
        />
        <Button type="submit">
          <Plus className="w-3.5 h-3.5" />
          Add block
        </Button>
      </form>
      <ul className="space-y-1.5">
        {blocks.length === 0 && (
          <li className="text-[12px] text-ink-tertiary">No upcoming blocks.</li>
        )}
        {blocks.map((b) => (
          <li
            key={b.id}
            className="px-3 py-2.5 border border-hairline rounded-md bg-surface-1 flex items-start gap-3"
          >
            <div className="flex-1 min-w-0">
              <div className="text-[13px] text-ink truncate">{b.title}</div>
              <div className="text-[11px] text-ink-subtle">{formatRange(b.startAt, b.endAt)}</div>
            </div>
            <Badge variant="outline" className="text-[10px]">
              {b.kind}
            </Badge>
            {b.googleEventId ? (
              <Badge variant="muted" className="text-[10px]">
                synced
              </Badge>
            ) : (
              <Button
                size="xs"
                variant="ghost"
                onClick={async () => {
                  await Calendar.Promote(b.id);
                  refresh();
                }}
              >
                Promote
              </Button>
            )}
            <Button
              size="xs"
              variant="ghost"
              onClick={async () => {
                await Calendar.Delete(b.id);
                refresh();
              }}
              className="text-ink-subtle hover:text-danger"
            >
              <Trash2 className="w-3 h-3" />
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

const TODO_FILTERS = [
  { key: "open", label: "Open" },
  { key: "overdue", label: "Overdue" },
  { key: "today", label: "Today" },
  { key: "upcoming", label: "Upcoming" },
  { key: "no-date", label: "No date" },
  { key: "done", label: "Done" },
  { key: "all", label: "All" },
];

function TodosPane({ workspace }) {
  const [items, setItems] = useState([]);
  const [draft, setDraft] = useState("");
  const [draftDue, setDraftDue] = useState("");
  const [filter, setFilter] = useState("open");

  const refresh = useCallback(async () => {
    setItems(await Todos.List(workspace.id));
  }, [workspace.id]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const sortedFiltered = filterAndSort(items, filter);

  return (
    <div className="space-y-3">
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          if (!draft.trim()) return;
          const due = draftDue ? new Date(draftDue).toISOString() : "";
          await Todos.Create(workspace.id, draft.trim(), due);
          setDraft("");
          setDraftDue("");
          refresh();
        }}
        className="flex gap-2"
      >
        <Input placeholder="Add a todo" value={draft} onChange={(e) => setDraft(e.target.value)} />
        <Input
          type="datetime-local"
          value={draftDue}
          onChange={(e) => setDraftDue(e.target.value)}
          className="w-52"
          aria-label="Due date"
        />
        <Button type="submit">
          <Plus className="w-3.5 h-3.5" />
          Add
        </Button>
      </form>
      <div className="flex flex-wrap gap-1.5">
        {TODO_FILTERS.map((f) => (
          <button
            key={f.key}
            type="button"
            onClick={() => setFilter(f.key)}
            className={`px-2.5 py-1 rounded text-[11px] ${
              filter === f.key ? "bg-ink text-canvas" : "bg-surface-2 text-ink-muted"
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>
      <ul className="space-y-1">
        {sortedFiltered.length === 0 && (
          <li className="text-[12px] text-ink-tertiary">Nothing in this view.</li>
        )}
        {sortedFiltered.map((t) => (
          <li
            key={t.id}
            className={
              "px-3 py-2 border border-hairline rounded-md bg-surface-1 flex items-center gap-2"
            }
          >
            <input
              type="checkbox"
              checked={t.done}
              onChange={async (e) => {
                await Todos.SetDone(t.id, e.target.checked);
                refresh();
              }}
              aria-label="Toggle done"
            />
            <span
              className={`flex-1 text-[13px] ${t.done ? "line-through text-ink-tertiary" : "text-ink"}`}
            >
              {t.text}
            </span>
            {t.dueAt && (
              <span className={`text-[11px] ${dueClassName(t)}`}>{formatDue(t.dueAt)}</span>
            )}
            <Button
              size="xs"
              variant="ghost"
              onClick={async () => {
                await Todos.Delete(t.id);
                refresh();
              }}
              className="text-ink-subtle hover:text-danger"
            >
              <Trash2 className="w-3 h-3" />
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

// Categorise a todo against today's bounds so the filter buttons can apply a
// consistent rule without dragging Date math into every render.
function dueBucket(todo, now) {
  if (todo.done) return "done";
  if (!todo.dueAt) return "no-date";
  const due = new Date(todo.dueAt);
  if (Number.isNaN(due.getTime())) return "no-date";
  const endOfToday = new Date(now);
  endOfToday.setHours(23, 59, 59, 999);
  if (due < now) return "overdue";
  if (due <= endOfToday) return "today";
  return "upcoming";
}

function filterAndSort(items, filter) {
  const now = new Date();
  const filtered = items.filter((t) => {
    const bucket = dueBucket(t, now);
    if (filter === "all") return true;
    if (filter === "open") return !t.done;
    return bucket === filter;
  });
  // Sort by dueAt ascending, undated last, then by id for stability.
  return filtered.slice().sort((a, b) => {
    const av = a.dueAt ? new Date(a.dueAt).getTime() : Number.POSITIVE_INFINITY;
    const bv = b.dueAt ? new Date(b.dueAt).getTime() : Number.POSITIVE_INFINITY;
    if (av !== bv) return av - bv;
    return a.id - b.id;
  });
}

function dueClassName(todo) {
  const bucket = dueBucket(todo, new Date());
  if (bucket === "overdue") return "text-danger";
  if (bucket === "today") return "text-ink-muted";
  return "text-ink-tertiary";
}

function formatDue(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const now = new Date();
  const sameDay = d.toDateString() === now.toDateString();
  if (sameDay) {
    return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  }
  return d.toLocaleDateString([], { month: "short", day: "numeric" });
}

function NotesPane({ workspace }) {
  const [list, setList] = useState([]);
  const [active, setActive] = useState(null);
  const [draft, setDraft] = useState({ title: "", bodyMd: "" });

  const refresh = useCallback(async () => {
    const rows = await Notes.List(workspace.id);
    setList(rows);
    if (active && !rows.find((r) => r.id === active.id)) {
      setActive(null);
    }
  }, [workspace.id, active]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function save() {
    if (active) {
      await Notes.Update(active.id, active.title, active.bodyMd);
    } else {
      if (!draft.title && !draft.bodyMd) return;
      await Notes.Create(workspace.id, draft.title, draft.bodyMd);
      setDraft({ title: "", bodyMd: "" });
    }
    refresh();
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-[220px_1fr] gap-4">
      <div className="space-y-1">
        <Button size="sm" variant="secondary" onClick={() => setActive(null)} className="w-full">
          <Plus className="w-3.5 h-3.5" />
          New note
        </Button>
        <ul className="space-y-0.5">
          {list.map((n) => {
            const sel = active?.id === n.id;
            return (
              <li key={n.id}>
                <button
                  type="button"
                  onClick={() => setActive({ ...n })}
                  className={
                    "w-full text-left px-2 py-1.5 rounded-md text-[13px] " +
                    (sel ? "bg-surface-2 text-ink" : "text-ink-muted hover:bg-surface-1")
                  }
                >
                  {n.title || <span className="italic text-ink-tertiary">(untitled)</span>}
                </button>
              </li>
            );
          })}
        </ul>
      </div>
      <div className="space-y-2">
        <Input
          placeholder="Title"
          value={active ? active.title : draft.title}
          onChange={(e) => {
            if (active) setActive({ ...active, title: e.target.value });
            else setDraft({ ...draft, title: e.target.value });
          }}
        />
        <Tabs defaultValue="edit" className="w-full">
          <TabsList>
            <TabsTrigger value="edit">Write</TabsTrigger>
            <TabsTrigger value="preview">Preview</TabsTrigger>
          </TabsList>
          <TabsContent value="edit">
            <Textarea
              rows={14}
              placeholder="Write in markdown…"
              value={active ? active.bodyMd : draft.bodyMd}
              onChange={(e) => {
                if (active) setActive({ ...active, bodyMd: e.target.value });
                else setDraft({ ...draft, bodyMd: e.target.value });
              }}
              className="font-mono text-[13px]"
            />
          </TabsContent>
          <TabsContent value="preview">
            <div
              className="min-h-[280px] px-3 py-2 border border-hairline rounded-md bg-surface-1 text-[13px] note-preview"
              // The renderer escapes HTML before inserting tags, so this is safe.
              // biome-ignore lint/security/noDangerouslySetInnerHtml: rendered from sanitised markdown
              dangerouslySetInnerHTML={{
                __html: renderMarkdown(active ? active.bodyMd : draft.bodyMd),
              }}
            />
          </TabsContent>
        </Tabs>
        <div className="flex gap-2">
          <Button onClick={save} size="sm">
            Save
          </Button>
          {active && (
            <Button
              size="sm"
              variant="ghost"
              onClick={async () => {
                await Notes.Delete(active.id);
                setActive(null);
                refresh();
              }}
              className="text-ink-subtle hover:text-danger"
            >
              <Trash2 className="w-3.5 h-3.5" />
              Delete
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

// renderMarkdown is a deliberately small subset renderer — headings, bold,
// italic, inline code, fenced code, bullet lists, paragraphs. Anything more
// involved should pull in a real library, but for personal notes this covers
// the 90% case without a dep.
function renderMarkdown(src) {
  if (!src) return '<p class="text-ink-tertiary italic">Nothing yet.</p>';
  const esc = (s) =>
    s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

  // Pull fenced code blocks out first so their content doesn't get
  // mis-handled by line-level rules.
  const blocks = [];
  let body = src.replace(/```([\w-]*)\n([\s\S]*?)```/g, (_m, _lang, code) => {
    const idx = blocks.push(`<pre><code>${esc(code.replace(/\n$/, ""))}</code></pre>`) - 1;
    return `@@MDBLOCK${idx}@@`;
  });
  body = esc(body);

  const lines = body.split("\n");
  const out = [];
  let inList = false;
  const flushList = () => {
    if (inList) {
      out.push("</ul>");
      inList = false;
    }
  };
  for (const raw of lines) {
    const line = raw.trimEnd();
    const h = line.match(/^(#{1,6})\s+(.+)$/);
    if (h) {
      flushList();
      const level = h[1].length;
      out.push(`<h${level}>${inlineMd(h[2])}</h${level}>`);
      continue;
    }
    const bullet = line.match(/^[-*]\s+(.+)$/);
    if (bullet) {
      if (!inList) {
        out.push("<ul>");
        inList = true;
      }
      out.push(`<li>${inlineMd(bullet[1])}</li>`);
      continue;
    }
    if (line.trim() === "") {
      flushList();
      out.push("");
      continue;
    }
    flushList();
    out.push(`<p>${inlineMd(line)}</p>`);
  }
  flushList();

  let html = out.join("\n");
  html = html.replace(/@@MDBLOCK(\d+)@@/g, (_m, i) => blocks[Number(i)]);
  return html;
}

function inlineMd(s) {
  return s
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/(?<!\*)\*([^*]+)\*(?!\*)/g, "<em>$1</em>")
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
}

function formatRange(startISO, endISO) {
  if (!startISO || !endISO) return "";
  const s = new Date(startISO);
  const e = new Date(endISO);
  if (Number.isNaN(s.getTime()) || Number.isNaN(e.getTime())) return `${startISO} → ${endISO}`;
  const same = s.toDateString() === e.toDateString();
  const sFmt = s.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
  const eFmt = same
    ? e.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })
    : e.toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
      });
  return `${sFmt} → ${eFmt}`;
}
