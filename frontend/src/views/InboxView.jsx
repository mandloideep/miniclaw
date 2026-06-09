import { useVirtualizer } from "@tanstack/react-virtual";
import {
  ChevronDown,
  Clock,
  CornerUpLeft,
  Image as ImageIcon,
  ImageOff,
  Inbox,
  LoaderCircle,
  PauseCircle,
  PlayCircle,
  RefreshCw,
  Send,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Attachments,
  GmailOAuth,
  IMAPSync,
  Inbox as InboxApi,
  SMTPSender,
  Snooze,
  Triage,
} from "../api";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../components/ui/dropdown-menu";
import { Input } from "../components/ui/input";
import { ScrollArea } from "../components/ui/scroll-area";
import { Separator } from "../components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/ui/tabs";
import { Textarea } from "../components/ui/textarea";

const PAGE = 80;
// Above this row count the list switches to virtualization. Below it,
// keeping the static markup is simpler and avoids the off-by-pixel
// jankiness that windowing introduces on tiny lists.
const VIRTUALIZE_AT = 200;
const ROW_HEIGHT_PX = 76;

export default function InboxView({ workspace, accounts, openEmailId, onEmailOpened }) {
  const [emails, setEmails] = useState([]);
  const [selectedId, setSelectedId] = useState(null);
  const [detail, setDetail] = useState(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [backfilling, setBackfilling] = useState(false);
  const [reachedEnd, setReachedEnd] = useState(false);

  const oldestReceivedAt = useMemo(
    () => (emails.length ? emails[emails.length - 1].receivedAt : ""),
    [emails],
  );

  const refresh = useCallback(async () => {
    if (!workspace) {
      setEmails([]);
      return;
    }
    const rows = await InboxApi.ListByWorkspace(workspace.id, PAGE);
    setEmails(rows);
    setReachedEnd(rows.length < PAGE);
    if (!rows.length) {
      setSelectedId(null);
      setDetail(null);
    }
  }, [workspace]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    if (selectedId == null) {
      setDetail(null);
      return;
    }
    InboxApi.Get(selectedId)
      .then(setDetail)
      .catch(() => setDetail(null));
  }, [selectedId]);

  useEffect(() => {
    if (openEmailId != null) {
      setSelectedId(openEmailId);
      onEmailOpened?.();
    }
  }, [openEmailId, onEmailOpened]);

  const loadOlder = useCallback(async () => {
    if (!workspace || !oldestReceivedAt || loadingOlder) return;
    setLoadingOlder(true);
    try {
      const more = await InboxApi.ListOlderByWorkspace(workspace.id, oldestReceivedAt, PAGE);
      if (more.length === 0) {
        setReachedEnd(true);
      } else {
        setEmails((prev) => [...prev, ...more]);
        if (more.length < PAGE) setReachedEnd(true);
      }
    } finally {
      setLoadingOlder(false);
    }
  }, [workspace, oldestReceivedAt, loadingOlder]);

  const backfillFromServer = useCallback(async () => {
    const remoteAccounts = accounts.filter(
      (a) => a.authKind === "gmail_oauth" || a.authKind === "imap",
    );
    if (!remoteAccounts.length || backfilling) return;
    const before = oldestReceivedAt
      ? oldestReceivedAt.slice(0, 10)
      : new Date().toISOString().slice(0, 10);
    setBackfilling(true);
    try {
      await Promise.all(
        remoteAccounts.map((a) => {
          if (a.authKind === "gmail_oauth")
            return GmailOAuth.BackfillBefore(a.id, before, 200).catch(() => 0);
          return IMAPSync.BackfillBefore(a.id, before, 200).catch(() => 0);
        }),
      );
      await refresh();
    } finally {
      setBackfilling(false);
    }
  }, [accounts, oldestReceivedAt, backfilling, refresh]);

  if (!workspace) {
    return (
      <Empty icon={Inbox} title="Pick a workspace">
        Use the workspace strip up top to choose one.
      </Empty>
    );
  }
  if (accounts.length === 0) {
    return (
      <Empty icon={Inbox} title={`No accounts in ${workspace.name}`}>
        Open Settings (top-right gear) to connect a Gmail or IMAP account.
      </Empty>
    );
  }

  return (
    <div className="flex h-full min-h-0">
      <section className="w-[420px] shrink-0 border-r border-hairline flex flex-col">
        <div className="h-14 px-4 flex items-center gap-2 border-b border-hairline">
          <h2 className="display text-sm font-medium tracking-[-0.01em]">{workspace.name}</h2>
          <Badge variant="muted">{emails.length}</Badge>
          <div className="ml-auto">
            <Button size="xs" variant="ghost" onClick={refresh} aria-label="Refresh list">
              <RefreshCw className="w-3 h-3" />
            </Button>
          </div>
        </div>
        <EmailList
          emails={emails}
          selectedId={selectedId}
          onSelect={(id) => {
            setSelectedId(id);
            const e = emails.find((x) => x.id === id);
            if (e && !e.isRead) {
              InboxApi.MarkRead(id).then(() => {
                setEmails((prev) =>
                  prev.map((row) => (row.id === id ? { ...row, isRead: true } : row)),
                );
              });
            }
          }}
          footer={
            <div className="px-3 pb-4 pt-1 flex flex-col gap-2 items-stretch">
              {!reachedEnd && (
                <Button size="sm" variant="secondary" onClick={loadOlder} disabled={loadingOlder}>
                  {loadingOlder ? (
                    <LoaderCircle className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    <ChevronDown className="w-3.5 h-3.5" />
                  )}
                  {loadingOlder ? "Loading…" : "Load older messages"}
                </Button>
              )}
              {accounts.some((a) => a.authKind === "gmail_oauth" || a.authKind === "imap") && (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={backfillFromServer}
                  disabled={backfilling}
                  title="Pull the next 200 messages older than the oldest one shown"
                >
                  {backfilling ? (
                    <LoaderCircle className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    <ChevronDown className="w-3.5 h-3.5" />
                  )}
                  {backfilling ? "Fetching older…" : "Fetch 200 older from server"}
                </Button>
              )}
              {reachedEnd && (
                <p className="text-[11px] text-ink-tertiary text-center">
                  You've reached the oldest cached message.
                </p>
              )}
            </div>
          }
        />
      </section>
      <section className="flex-1 min-w-0 flex flex-col">
        <EmailReader detail={detail} onPutAside={refresh} />
      </section>
    </div>
  );
}

// EmailList swaps between a plain mapped list and @tanstack/react-virtual
// once the row count crosses VIRTUALIZE_AT. The virtualizer needs an
// outer scrolling element to measure against — we mount one with the
// list filling it and the footer rendered below the windowed area so
// "load older" stays reachable.
function EmailList({ emails, selectedId, onSelect, footer }) {
  const parentRef = useRef(null);
  const virtualizer = useVirtualizer({
    count: emails.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 8,
  });
  const useVirtual = emails.length >= VIRTUALIZE_AT;

  if (!useVirtual) {
    return (
      <ScrollArea className="flex-1">
        <ul className="p-1.5">
          {emails.map((e) => (
            <EmailRow
              key={e.id}
              email={e}
              active={selectedId === e.id}
              onClick={() => onSelect(e.id)}
            />
          ))}
        </ul>
        {footer}
      </ScrollArea>
    );
  }

  const items = virtualizer.getVirtualItems();
  return (
    <div ref={parentRef} className="flex-1 overflow-auto">
      <div className="relative" style={{ height: `${virtualizer.getTotalSize()}px`, padding: 6 }}>
        {items.map((v) => {
          const e = emails[v.index];
          if (!e) return null;
          return (
            <div
              key={e.id}
              data-index={v.index}
              ref={virtualizer.measureElement}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                right: 0,
                transform: `translateY(${v.start}px)`,
              }}
            >
              <ul className="px-1.5">
                <EmailRow email={e} active={selectedId === e.id} onClick={() => onSelect(e.id)} />
              </ul>
            </div>
          );
        })}
      </div>
      {footer}
    </div>
  );
}

function EmailRow({ email, active, onClick }) {
  const who = email.fromName || email.fromAddress;
  return (
    <li
      className={
        "group rounded-md cursor-pointer transition-colors mb-0.5 " +
        (active
          ? "bg-surface-1 border border-hairline-strong"
          : "border border-transparent hover:bg-surface-1")
      }
    >
      <button type="button" className="w-full text-left px-3 py-2" onClick={onClick}>
        <div className="flex items-center gap-2 min-w-0">
          {!email.isRead && (
            <span
              className="w-1.5 h-1.5 rounded-full bg-brand shrink-0"
              role="status"
              aria-label="unread"
            />
          )}
          <span
            className={`text-[13px] truncate ${email.isRead ? "text-ink-subtle" : "text-ink font-medium"}`}
          >
            {who}
          </span>
          <span className="ml-auto text-[11px] text-ink-tertiary shrink-0">
            {formatStamp(email.receivedAt)}
          </span>
        </div>
        <div
          className={`text-[13px] truncate mt-0.5 ${email.isRead ? "text-ink-subtle" : "text-ink-muted"}`}
        >
          {email.subject || <span className="italic text-ink-tertiary">(no subject)</span>}
        </div>
        <div className="flex items-center gap-1.5 mt-1.5">
          {email.category && (
            <Badge variant="outline" className="text-[10px]">
              {email.category}
            </Badge>
          )}
          {email.isPutAside && (
            <Badge variant="muted" className="text-[10px]">
              <PauseCircle className="w-2.5 h-2.5" /> aside
            </Badge>
          )}
          <span className="text-[11px] text-ink-tertiary truncate">
            {(email.bodyPlain || "").slice(0, 90)}
          </span>
        </div>
      </button>
    </li>
  );
}

function EmailReader({ detail, onPutAside }) {
  const [replying, setReplying] = useState(false);
  const [showImages, setShowImages] = useState(false);
  const [snoozeBusy, setSnoozeBusy] = useState(false);

  useEffect(() => {
    setReplying(false);
    setShowImages(false);
  }, []);

  const handleSnooze = useCallback(
    async (kind) => {
      if (!detail) return;
      const until = nextSnoozeAt(kind);
      if (!until) return;
      setSnoozeBusy(true);
      try {
        await Snooze.Snooze(detail.id, until);
        onPutAside?.();
      } finally {
        setSnoozeBusy(false);
      }
    },
    [detail, onPutAside],
  );

  if (!detail) {
    return (
      <div className="flex-1 flex items-center justify-center text-ink-tertiary text-sm">
        Select a message.
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full min-h-0">
      <header className="px-6 pt-5 pb-3 border-b border-hairline">
        <div className="flex items-start gap-3">
          <div className="min-w-0 flex-1">
            <h1 className="display text-[20px] leading-tight tracking-[-0.015em]">
              {detail.subject || <span className="text-ink-tertiary italic">(no subject)</span>}
            </h1>
            <div className="mt-1.5 text-[12px] text-ink-subtle flex flex-wrap gap-x-3 gap-y-0.5">
              <span>
                <span className="text-ink-muted">{detail.fromName || detail.fromAddress}</span>
                {detail.fromName ? (
                  <span className="text-ink-tertiary"> · {detail.fromAddress}</span>
                ) : null}
              </span>
              <span>{formatStamp(detail.receivedAt, true)}</span>
              {detail.to && <span>to {detail.to}</span>}
              {detail.category && <Badge variant="outline">{detail.category}</Badge>}
            </div>
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setShowImages((v) => !v)}
              title="Toggle remote image loading"
            >
              {showImages ? (
                <ImageOff className="w-3.5 h-3.5" />
              ) : (
                <ImageIcon className="w-3.5 h-3.5" />
              )}
              {showImages ? "Hide images" : "Show images"}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={async () => {
                await Triage.TogglePutAside(detail.id);
                onPutAside?.();
              }}
            >
              {detail.isPutAside ? (
                <PlayCircle className="w-3.5 h-3.5" />
              ) : (
                <PauseCircle className="w-3.5 h-3.5" />
              )}
              {detail.isPutAside ? "Unstash" : "Put aside"}
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="sm" variant="ghost" disabled={snoozeBusy}>
                  {snoozeBusy ? (
                    <LoaderCircle className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    <Clock className="w-3.5 h-3.5" />
                  )}
                  Snooze
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>Hide until</DropdownMenuLabel>
                <DropdownMenuItem onSelect={() => handleSnooze("later-today")}>
                  Later today
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => handleSnooze("tomorrow")}>
                  Tomorrow morning
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => handleSnooze("next-week")}>
                  Next week
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => handleSnooze("next-month")}>
                  Next month
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onSelect={async () => {
                    setSnoozeBusy(true);
                    try {
                      await Snooze.Unsnooze(detail.id);
                      onPutAside?.();
                    } finally {
                      setSnoozeBusy(false);
                    }
                  }}
                >
                  Wake now
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            {!replying && (
              <Button size="sm" onClick={() => setReplying(true)}>
                <CornerUpLeft className="w-3.5 h-3.5" />
                Reply
              </Button>
            )}
          </div>
        </div>
      </header>
      <ScrollArea className="flex-1 px-6 py-5">
        {detail.bodyHtml && detail.bodyHtml.trim().length > 0 ? (
          <Tabs defaultValue="html">
            <TabsList>
              <TabsTrigger value="html">Rendered</TabsTrigger>
              <TabsTrigger value="plain">Plain text</TabsTrigger>
            </TabsList>
            <TabsContent value="html">
              <HtmlBody detail={detail} loadRemoteImages={showImages} />
            </TabsContent>
            <TabsContent value="plain">
              <pre className="whitespace-pre-wrap text-[14px] leading-relaxed text-ink-muted font-sans">
                {detail.bodyPlain || "(empty body)"}
              </pre>
            </TabsContent>
          </Tabs>
        ) : (
          <HtmlBody detail={detail} loadRemoteImages={showImages} />
        )}
        {replying && (
          <>
            <Separator className="my-6" />
            <ReplyComposer
              email={detail}
              onClose={() => setReplying(false)}
              onSent={() => setReplying(false)}
            />
          </>
        )}
      </ScrollArea>
    </div>
  );
}

function HtmlBody({ detail, loadRemoteImages }) {
  const ref = useRef(null);
  const [cidMap, setCidMap] = useState({});

  useEffect(() => {
    let cancelled = false;
    if (!detail?.id) {
      setCidMap({});
      return;
    }
    Attachments.GetInline(detail.id)
      .then((rows) => {
        if (cancelled) return;
        const next = {};
        for (const r of rows ?? []) {
          if (r?.contentId) next[r.contentId.toLowerCase()] = r.dataUrl;
        }
        setCidMap(next);
      })
      .catch(() => {
        if (!cancelled) setCidMap({});
      });
    return () => {
      cancelled = true;
    };
  }, [detail?.id]);

  const srcDoc = useMemo(
    () => buildSrcDoc(detail, loadRemoteImages, cidMap),
    [detail, loadRemoteImages, cidMap],
  );

  const onLoad = useCallback(() => {
    const iframe = ref.current;
    if (!iframe) return;
    try {
      const doc = iframe.contentDocument;
      if (!doc) return;
      const h = doc.documentElement.scrollHeight || doc.body?.scrollHeight || 0;
      iframe.style.height = `${Math.min(Math.max(h, 200), 4000)}px`;
    } catch {
      /* same-origin sandbox should let us read; if not, fall back to default height */
    }
  }, []);

  if (!detail.bodyHtml || detail.bodyHtml.trim().length === 0) {
    return (
      <pre className="whitespace-pre-wrap text-[14px] leading-relaxed text-ink-muted font-sans">
        {detail.bodyPlain || "(empty body)"}
      </pre>
    );
  }

  return (
    <iframe
      ref={ref}
      title="email body"
      sandbox="allow-same-origin allow-popups"
      srcDoc={srcDoc}
      className="w-full block bg-white rounded-md border border-hairline"
      style={{ height: "60vh", colorScheme: "light" }}
      onLoad={onLoad}
    />
  );
}

// buildSrcDoc strips scripts, splices inline-image data URLs into cid:
// references, and (optionally) blocks remote images. Anything we couldn't
// match in cidMap gets a small placeholder so the layout doesn't
// collapse and the user knows there's a missing image.
function buildSrcDoc(detail, loadRemoteImages, cidMap = {}) {
  let html = detail.bodyHtml || "";
  html = html.replace(/<script[\s\S]*?<\/script>/gi, "");
  html = html.replace(/\son\w+\s*=\s*"[^"]*"/gi, "");
  html = html.replace(/\son\w+\s*=\s*'[^']*'/gi, "");
  html = html.replace(/javascript:/gi, "blocked:");
  html = html.replace(/<img\b([^>]*?)\bsrc\s*=\s*["']([^"']*)["']/gi, (_m, rest, src) => {
    if (/^cid:/i.test(src)) {
      const cid = src.replace(/^cid:/i, "").toLowerCase();
      const dataUrl = cidMap[cid];
      if (dataUrl) return `<img${rest} src="${dataUrl}"`;
      return `<img${rest} alt="(missing inline image)" style="display:inline-block;min-width:24px;min-height:24px;background:#f3f4f6;border:1px dashed #d1d5db"`;
    }
    if (!loadRemoteImages) {
      return `<img${rest} data-blocked-src="${src}" alt="(remote image)" style="display:inline-block;min-width:24px;min-height:24px;background:#e5e7eb;border:1px dashed #cbd5e1"`;
    }
    return `<img${rest} src="${src}"`;
  });
  return `<!doctype html>
<html><head><base target="_blank">
<style>
  html,body { margin:0; padding:16px; background:#ffffff; color:#111827; font-family: -apple-system, system-ui, "Segoe UI", Roboto, sans-serif; font-size:14px; line-height:1.55; }
  a { color: #5e6ad2; }
  img { max-width: 100%; height: auto; }
  table { max-width: 100%; }
  blockquote { border-left: 3px solid #e5e7eb; padding-left: 12px; color: #4b5563; margin: 0 0 12px 0; }
</style></head>
<body>${html}</body></html>`;
}

function ReplyComposer({ email, onClose, onSent }) {
  const [body, setBody] = useState(
    "\n\nOn " +
      email.receivedAt +
      ", " +
      (email.fromName || email.fromAddress) +
      " wrote:\n> " +
      (email.bodyPlain || "").split("\n").join("\n> "),
  );
  const [subject, setSubject] = useState(
    email.subject.startsWith("Re:") ? email.subject : `Re: ${email.subject}`,
  );
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function send() {
    setBusy(true);
    setErr("");
    try {
      await SMTPSender.Send(email.accountId, {
        to: [email.fromAddress],
        cc: [],
        subject,
        body,
      });
      onSent();
    } catch (e) {
      setErr(String(e?.message ?? e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="rounded-lg border border-hairline bg-surface-1 p-4">
      <div className="text-[11px] text-ink-subtle mb-2">
        Replying to <span className="text-ink-muted">{email.fromAddress}</span>
      </div>
      <Input
        value={subject}
        onChange={(e) => setSubject(e.target.value)}
        className="mb-2"
        aria-label="Subject"
      />
      <Textarea
        rows={10}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        className="font-mono text-[13px]"
        aria-label="Body"
      />
      {err && <p className="mt-2 text-[12px] text-danger">{err}</p>}
      <div className="flex gap-2 mt-3">
        <Button onClick={send} disabled={busy} size="sm">
          {busy ? (
            <LoaderCircle className="w-3.5 h-3.5 animate-spin" />
          ) : (
            <Send className="w-3.5 h-3.5" />
          )}
          {busy ? "Sending…" : "Send"}
        </Button>
        <Button onClick={onClose} variant="secondary" size="sm">
          Cancel
        </Button>
      </div>
    </div>
  );
}

function Empty({ icon: Icon, title, children }) {
  return (
    <div className="h-full flex items-center justify-center">
      <div className="text-center max-w-xs">
        <div className="inline-flex items-center justify-center w-10 h-10 rounded-full bg-surface-1 border border-hairline mb-3">
          <Icon className="w-4 h-4 text-ink-subtle" />
        </div>
        <h3 className="display text-base mb-1">{title}</h3>
        <p className="text-[13px] text-ink-subtle">{children}</p>
      </div>
    </div>
  );
}

// nextSnoozeAt resolves the dropdown choice into an RFC3339 UTC timestamp.
// "Later today" snoozes 3 hours forward (or to 9 PM, whichever is sooner)
// so a morning click doesn't bounce back into the inbox before lunch.
function nextSnoozeAt(kind) {
  const now = new Date();
  let target = new Date(now);
  if (kind === "later-today") {
    target.setHours(now.getHours() + 3, 0, 0, 0);
    const ninePM = new Date(now);
    ninePM.setHours(21, 0, 0, 0);
    if (target > ninePM) target = ninePM;
  } else if (kind === "tomorrow") {
    target.setDate(now.getDate() + 1);
    target.setHours(9, 0, 0, 0);
  } else if (kind === "next-week") {
    const daysUntilMon = (1 - now.getDay() + 7) % 7 || 7;
    target.setDate(now.getDate() + daysUntilMon);
    target.setHours(9, 0, 0, 0);
  } else if (kind === "next-month") {
    target.setMonth(now.getMonth() + 1, 1);
    target.setHours(9, 0, 0, 0);
  } else {
    return null;
  }
  // Backend requires a future timestamp; nudge it forward if we somehow
  // computed something <= now (clock skew, sub-second rounding).
  if (target <= now) target = new Date(now.getTime() + 60_000);
  return target.toISOString();
}

function formatStamp(iso, withTime = false) {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const now = new Date();
  const sameDay = d.toDateString() === now.toDateString();
  if (sameDay && !withTime) {
    return d.toLocaleTimeString(undefined, {
      hour: "numeric",
      minute: "2-digit",
    });
  }
  if (withTime) {
    return d.toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      year: d.getFullYear() === now.getFullYear() ? undefined : "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  }
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: d.getFullYear() === now.getFullYear() ? undefined : "numeric",
  });
}
