import hljs from 'highlight.js/lib/common';
import 'highlight.js/styles/github-dark-dimmed.css';
import { Check, ChevronDown, ChevronRight, FileEdit, FileX, ListTree, Loader2, Plus, RefreshCw, Rows3, SquareArrowOutUpRight, Trash2, Undo2, Wand2 } from 'lucide-react';
import type React from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Textarea } from '@/components/ui/textarea';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { useGitChangesViewMode } from '@/providers/appearance';
import type { git as gitModels } from '@/wailsjs/go/models';

export type FileStatus = gitModels.FileChangeStatus;
export type GitState = gitModels.AgentState;

export interface GitChangesOps {
  getDiff(): Promise<string>;
  getFileStatuses(): Promise<FileStatus[]>;
  getGitState(): Promise<GitState>;
  stageFile(path: string): Promise<void>;
  stageFiles(paths: string[]): Promise<void>;
  unstageFile(path: string): Promise<void>;
  unstageFiles(paths: string[]): Promise<void>;
  stageAll(): Promise<void>;
  unstageAll(): Promise<void>;
  commit(message: string, amend: boolean): Promise<void>;
  push(force: boolean): Promise<void>;
  sync(force: boolean): Promise<void>;
  discardFile?(path: string, untracked: boolean): Promise<void>;
  discardFiles?(tracked: string[], untracked: string[]): Promise<void>;
  generateCommitMessage?(): Promise<string>;
}

interface Props {
  ops: GitChangesOps;
  // 0 = no polling; otherwise milliseconds between refreshes when not busy.
  pollInterval?: number;
  // Rendered above the file list / diff row, e.g. branch selector.
  headerSlot?: React.ReactNode;
  // Resets internal state (diff/statuses/selection) when this key changes.
  // Pass the agent id / project id so switching contexts clears stale state.
  resetKey?: string;
  onOpenFile?: (path: string, line: number) => void;
  // Called after a successful push/sync when the user has ticked "Close session".
  onClose?(): Promise<void>;
  onCountChange?: (count: number) => void;
}

type ShipAction = 'ship' | 'amend' | 'push-only' | 'push-with-lease' | 'commit-only' | 'sync';

type DiffLine = { type: '+' | '-' | ' ' | '\\'; text: string };
type DiffHunk = { header: string; lines: DiffLine[] };
type DiffFile = { path: string; hunks: DiffHunk[] };

const LIST_WIDTH_KEY = 'git-changes:list-width';
const LIST_WIDTH_DEFAULT = 288;
const LIST_WIDTH_MIN = 220;
const LIST_WIDTH_MAX = 560;

function clampListWidth(px: number): number {
  return Math.min(LIST_WIDTH_MAX, Math.max(LIST_WIDTH_MIN, px));
}

const EXT_TO_LANG: Record<string, string> = {
  ts: 'typescript',
  tsx: 'typescript',
  js: 'javascript',
  jsx: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  go: 'go',
  py: 'python',
  rs: 'rust',
  css: 'css',
  scss: 'scss',
  less: 'less',
  html: 'xml',
  htm: 'xml',
  xml: 'xml',
  svg: 'xml',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  sh: 'bash',
  bash: 'bash',
  zsh: 'bash',
  md: 'markdown',
  sql: 'sql',
  rb: 'ruby',
  java: 'java',
  c: 'c',
  cpp: 'cpp',
  cc: 'cpp',
  h: 'c',
  hpp: 'cpp',
  php: 'php',
  swift: 'swift',
  kt: 'kotlin',
};

function getLang(path: string): string | undefined {
  const ext = path.split('.').pop()?.toLowerCase();
  return ext ? EXT_TO_LANG[ext] : undefined;
}

function highlightLines(lines: DiffLine[], lang: string | undefined): string[] {
  const code = lines.map((l) => l.text).join('\n');
  try {
    const result = lang ? hljs.highlight(code, { language: lang }) : hljs.highlightAuto(code);
    return result.value.split('\n');
  } catch {
    return code.split('\n').map((l) => l.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'));
  }
}

function parseHunkStart(header: string): { oldStart: number; newStart: number } {
  const m = header.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
  return { oldStart: m ? Number(m[1]) : 1, newStart: m ? Number(m[2]) : 1 };
}

function parseDiff(raw: string): Map<string, DiffFile> {
  const files = new Map<string, DiffFile>();
  let current: DiffFile | null = null;
  let currentHunk: DiffHunk | null = null;
  for (const line of raw.split('\n')) {
    if (line.startsWith('diff --git ')) {
      if (current) {
        files.set(current.path, current);
      }
      const match = line.match(/diff --git a\/.+ b\/(.+)/);
      current = { path: match?.[1] ?? line, hunks: [] };
      currentHunk = null;
    } else if (line.startsWith('@@ ') && current) {
      currentHunk = { header: line, lines: [] };
      current.hunks.push(currentHunk);
    } else if (currentHunk && line.startsWith('+') && !line.startsWith('+++')) {
      currentHunk.lines.push({ type: '+', text: line.slice(1) });
    } else if (currentHunk && line.startsWith('-') && !line.startsWith('---')) {
      currentHunk.lines.push({ type: '-', text: line.slice(1) });
    } else if (currentHunk && line.startsWith('\\')) {
      currentHunk.lines.push({ type: '\\', text: line });
    } else if (currentHunk) {
      currentHunk.lines.push({ type: ' ', text: line.slice(1) });
    }
  }
  if (current) {
    files.set(current.path, current);
  }
  return files;
}

const STATUS_CLASS: Record<string, string> = {
  M: 'text-amber-400',
  A: 'text-emerald-400',
  '?': 'text-emerald-400',
  D: 'text-red-400',
  R: 'text-blue-400',
  U: 'text-purple-400',
};

function statusLabel(s: string): string {
  return s === '?' ? 'U' : s || 'M';
}

function splitPath(path: string): { name: string; dir: string } {
  const idx = path.lastIndexOf('/');
  if (idx < 0) {
    return { name: path, dir: '' };
  }
  return { name: path.slice(idx + 1), dir: path.slice(0, idx) };
}

export function GitChangesPanel({ ops, pollInterval = 0, headerSlot, resetKey, onOpenFile, onClose, onCountChange }: Props) {
  const { t } = useTranslation();
  const { viewMode: view, setViewMode: setView } = useGitChangesViewMode();
  const [diff, setDiff] = useState<string | null>(null);
  const [statuses, setStatuses] = useState<FileStatus[] | null>(null);
  const [gitState, setGitState] = useState<GitState | null>(null);
  const [error, setError] = useState(false);
  const [filter, setFilter] = useState('');
  const [selected, setSelected] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [subject, setSubject] = useState('');
  const [body, setBody] = useState('');
  const [showBody, setShowBody] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [closeAfterShip, setCloseAfterShip] = useState(false);
  const [approvedCollapsed, setApprovedCollapsed] = useState(false);
  const [pendingCollapsed, setPendingCollapsed] = useState(false);
  const [listWidth, setListWidth] = useState(() => {
    const stored = Number(localStorage.getItem(LIST_WIDTH_KEY));
    return Number.isFinite(stored) && stored > 0 ? clampListWidth(stored) : LIST_WIDTH_DEFAULT;
  });
  const dragState = useRef<{ startX: number; startWidth: number } | null>(null);

  useEffect(() => {
    localStorage.setItem(LIST_WIDTH_KEY, String(listWidth));
  }, [listWidth]);

  const onResizeStart = useCallback(
    (e: React.PointerEvent) => {
      e.preventDefault();
      dragState.current = { startX: e.clientX, startWidth: listWidth };
      document.body.style.userSelect = 'none';
      document.body.style.cursor = 'col-resize';
      const onMove = (ev: PointerEvent) => {
        if (!dragState.current) {
          return;
        }
        setListWidth(clampListWidth(dragState.current.startWidth + ev.clientX - dragState.current.startX));
      };
      const onUp = () => {
        dragState.current = null;
        document.body.style.userSelect = '';
        document.body.style.cursor = '';
        window.removeEventListener('pointermove', onMove);
        window.removeEventListener('pointerup', onUp);
      };
      window.addEventListener('pointermove', onMove);
      window.addEventListener('pointerup', onUp);
    },
    [listWidth],
  );

  const refresh = useMemo(
    () => async () => {
      const [d, s, g] = await Promise.all([ops.getDiff().catch(() => null), ops.getFileStatuses().catch(() => null), ops.getGitState().catch(() => null)]);
      if (d === null && s === null) {
        setError(true);
        return;
      }
      if (d !== null) {
        setDiff(d);
      }
      if (s !== null) {
        setStatuses(s);
        onCountChange?.(s.length);
      }
      if (g !== null) {
        setGitState(g);
      }
    },
    [ops],
  );

  useEffect(() => {
    setDiff(null);
    setStatuses(null);
    setGitState(null);
    setError(false);
    setSelected(null);
    setSubject('');
    setBody('');
    setShowBody(false);
    void refresh();
  }, [refresh, resetKey]);

  useEffect(() => {
    if (pollInterval <= 0) {
      return;
    }
    const id = window.setInterval(() => {
      if (!busy) {
        void refresh();
      }
    }, pollInterval);
    return () => window.clearInterval(id);
  }, [pollInterval, busy, refresh]);

  const diffFiles = useMemo(() => (diff ? parseDiff(diff) : new Map<string, DiffFile>()), [diff]);

  const needle = filter.trim().toLowerCase();
  const filtered = useMemo(() => (statuses ?? []).filter((f) => !needle || f.path.toLowerCase().includes(needle)), [statuses, needle]);

  const approved = filtered.filter((f) => f.staged);
  const pending = filtered.filter((f) => !f.staged);

  useEffect(() => {
    if (selected && filtered.some((f) => f.path === selected)) {
      return;
    }
    setSelected(pending[0]?.path ?? approved[0]?.path ?? null);
  }, [selected, filtered, pending, approved]);

  const runWith = async (op: () => Promise<void>, onFailKey: string) => {
    if (busy) {
      return;
    }
    setBusy(true);
    try {
      await op();
      await refresh();
    } catch (err) {
      toastError({ title: t(onFailKey), err });
    } finally {
      setBusy(false);
    }
  };

  const stageFile = (path: string) => {
    const idx = pending.findIndex((f) => f.path === path);
    const next = idx >= 0 ? (pending[idx + 1] ?? pending[idx - 1] ?? null) : null;
    return runWith(async () => {
      await ops.stageFile(path);
      setSelected(next?.path ?? null);
    }, 'agents.detail.approveFailed');
  };

  const unstageFile = (path: string) =>
    runWith(async () => {
      const idx = approved.findIndex((f) => f.path === path);
      const next = idx >= 0 ? (approved[idx + 1] ?? approved[idx - 1] ?? null) : null;
      await ops.unstageFile(path);
      setSelected(next?.path ?? null);
    }, 'agents.detail.unapproveFailed');

  const approveAll = () =>
    runWith(async () => {
      await ops.stageAll();
      toast.success(t('agents.detail.approveSuccess'));
    }, 'agents.detail.approveFailed');

  const unapproveAll = () =>
    runWith(async () => {
      await ops.unstageAll();
    }, 'agents.detail.unapproveFailed');

  const stageDir = (node: TreeNode) =>
    runWith(async () => {
      await ops.stageFiles(collectPaths(node));
    }, 'agents.detail.approveFailed');

  const unstageDir = (node: TreeNode) =>
    runWith(async () => {
      await ops.unstageFiles(collectPaths(node));
    }, 'agents.detail.unapproveFailed');

  const discardFile = (path: string, untracked: boolean) => {
    if (!ops.discardFile) return;
    if (!window.confirm(t('agents.detail.discardFileConfirm', { name: splitPath(path).name }))) return;
    runWith(() => ops.discardFile!(path, untracked), 'agents.detail.discardFailed');
  };

  const discardDir = (node: TreeNode) => {
    if (!ops.discardFiles) return;
    if (!window.confirm(t('agents.detail.discardDirConfirm', { name: node.name }))) return;
    const paths = collectPaths(node);
    const untrackedSet = new Set(paths.filter((p) => statuses?.find((s) => s.path === p)?.status === '?'));
    runWith(
      () =>
        ops.discardFiles!(
          paths.filter((p) => !untrackedSet.has(p)),
          [...untrackedSet],
        ),
      'agents.detail.discardFailed',
    );
  };

  const handleGenerate = async () => {
    if (!ops.generateCommitMessage || generating) return;
    setGenerating(true);
    try {
      const msg = await ops.generateCommitMessage();
      setSubject(msg);
    } catch (err) {
      toastError({ title: t('agents.detail.generateCommitMessageFailed'), err });
    } finally {
      setGenerating(false);
    }
  };

  const stagedCount = gitState?.stagedCount ?? approved.length;
  const aheadCount = gitState?.aheadCount ?? 0;
  const isProtected = gitState?.isProtected ?? false;

  const runShip = async (action: ShipAction) => {
    if (busy) {
      return;
    }
    const needsMessage = (action === 'ship' || action === 'commit-only' || action === 'sync') && stagedCount > 0;
    const message = subject.trim();
    const fullMessage = body.trim() ? `${message}\n\n${body.trim()}` : message;
    if (needsMessage && message === '') {
      toastError({ title: t('agents.detail.commitFailed'), err: new Error(t('agents.detail.commitMessageRequired')) });
      return;
    }
    setBusy(true);
    let pushed = false;
    try {
      if (action === 'amend') {
        await ops.commit(fullMessage, true);
        toast.success(t('agents.detail.commitSucceeded'));
      } else if (action === 'commit-only') {
        if (stagedCount > 0) {
          await ops.commit(fullMessage, false);
        }
        toast.success(t('agents.detail.commitSucceeded'));
      } else if (action === 'ship') {
        if (stagedCount > 0) {
          await ops.commit(fullMessage, false);
        }
        await ops.push(false);
        toast.success(t('agents.detail.pushSucceeded'));
        pushed = true;
      } else if (action === 'push-only') {
        await ops.push(false);
        toast.success(t('agents.detail.pushSucceeded'));
        pushed = true;
      } else if (action === 'push-with-lease') {
        await ops.push(true);
        toast.success(t('agents.detail.pushSucceeded'));
        pushed = true;
      } else if (action === 'sync') {
        if (stagedCount > 0) {
          await ops.commit(fullMessage, false);
        }
        await ops.sync(false);
        toast.success(t('agents.detail.syncSucceeded'));
        pushed = true;
      }
      setSubject('');
      setBody('');
      setShowBody(false);
      await refresh();
      if (pushed && closeAfterShip && onClose) {
        await onClose().catch(() => {});
      }
    } catch (err) {
      const failKey = action === 'sync' ? 'agents.detail.syncFailed' : action === 'ship' || action === 'push-only' || action === 'push-with-lease' ? 'agents.detail.pushFailed' : 'agents.detail.commitFailed';
      toastError({ title: t(failKey), err });
    } finally {
      setBusy(false);
    }
  };

  // Container-level Ctrl/Cmd+Enter so the shortcut works regardless of which
  // child has focus (textarea, file list buttons, etc.). The handler ignores
  // events from inputs unless modifier is held — typing in the filter must not
  // ship by accident.
  const onContainerKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'Enter' || (!e.ctrlKey && !e.metaKey)) {
      return;
    }
    e.preventDefault();
    void runShip(primaryAction);
  };

  if (error) {
    return (
      <div className="flex h-0 flex-1 flex-col gap-3">
        {headerSlot}
        <div className="flex h-0 flex-1 items-center justify-center rounded-md border border-dashed border-border text-xs text-muted-foreground">
          <FileX className="mr-2 size-4" />
          {t('agents.detail.diffError')}
        </div>
      </div>
    );
  }
  if (diff === null || statuses === null) {
    return (
      <div className="flex h-0 flex-1 flex-col gap-3">
        {headerSlot}
        <div className="flex h-0 flex-1 items-center justify-center text-xs text-muted-foreground">{t('agents.detail.diffLoading')}</div>
      </div>
    );
  }

  const selectedDiff = selected ? (diffFiles.get(selected) ?? null) : null;
  const canShip = !busy && (stagedCount > 0 || aheadCount > 0);
  const primaryAction: ShipAction = stagedCount === 0 && aheadCount > 0 ? 'push-only' : stagedCount > 0 && !gitState?.hasUpstream ? 'commit-only' : 'ship';
  const primaryLabel = primaryAction === 'push-only' ? t('agents.detail.push') : primaryAction === 'commit-only' ? t('agents.detail.commit') : t('agents.detail.commitAndPush');

  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: container key handler routes Ctrl+Enter to Ship
    // biome-ignore lint/a11y/useKeyWithClickEvents: container has no click handler, only key handler
    <div className="flex h-full flex-col gap-3" onKeyDown={onContainerKeyDown}>
      {headerSlot}

      {(statuses ?? []).length === 0 ? (
        <div className="flex h-0 flex-1 items-center justify-center rounded-md border border-dashed border-border text-xs text-muted-foreground">{t('agents.detail.diffEmpty')}</div>
      ) : (
        <div className="flex h-0 flex-1">
          <div style={{ width: listWidth }} className="flex h-full shrink-0 flex-col overflow-hidden rounded-md border border-border">
            <div className="flex shrink-0 items-center gap-1.5 border-b border-border p-1.5">
              <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t('agents.detail.filesFilterPlaceholder')} className="h-7 min-w-0 flex-1 text-xs" />
              <div className="flex shrink-0 items-center rounded-md border border-border">
                <button type="button" onClick={() => setView('list')} title={t('agents.detail.filesViewList')} className={cn('p-1 text-muted-foreground hover:text-foreground', view === 'list' && 'bg-muted text-foreground')}>
                  <Rows3 className="size-3.5" />
                </button>
                <button type="button" onClick={() => setView('tree')} title={t('agents.detail.filesViewTree')} className={cn('p-1 text-muted-foreground hover:text-foreground', view === 'tree' && 'bg-muted text-foreground')}>
                  <ListTree className="size-3.5" />
                </button>
              </div>
              <button type="button" onClick={() => void refresh()} disabled={busy} title={t('agents.detail.refreshChanges')} className="p-1 text-muted-foreground hover:text-foreground disabled:opacity-40">
                <RefreshCw className="size-3.5" />
              </button>
            </div>
            <ScrollArea className="h-0 flex-1">
              <div className="flex flex-col gap-0.5 p-1">
                <SectionHeader
                  label={t('agents.detail.filesApprovedSection', { count: approved.length })}
                  tone="approved"
                  collapsed={approvedCollapsed}
                  onToggle={() => setApprovedCollapsed((v) => !v)}
                  action={
                    approved.length > 0 && (
                      <Button type="button" size="icon" variant="ghost" onClick={() => void unapproveAll()} disabled={busy} title={t('agents.detail.unapproveFile')} className="size-6 text-muted-foreground hover:text-foreground">
                        <Undo2 className="size-3.5" />
                      </Button>
                    )
                  }
                />
                {!approvedCollapsed &&
                  (approved.length === 0 ? (
                    <p className="px-2 py-1 text-[11px] text-muted-foreground">{t('agents.detail.filesNoApproved')}</p>
                  ) : view === 'tree' ? (
                    <TreeView node={buildTree(approved)} depth={0} selected={selected} onSelect={setSelected} onUnstage={(p) => void unstageFile(p)} onUnstageDir={(n) => void unstageDir(n)} busy={busy} t={t} showStage={false} />
                  ) : (
                    approved.map((f) => <FileRow key={f.path} file={f} selected={selected === f.path} onSelect={() => setSelected(f.path)} onUnstage={() => void unstageFile(f.path)} busy={busy} t={t} />)
                  ))}

                <div className="mt-2">
                  <SectionHeader
                    label={t('agents.detail.filesChangesSection', { count: pending.length })}
                    tone="pending"
                    collapsed={pendingCollapsed}
                    onToggle={() => setPendingCollapsed((v) => !v)}
                    action={
                      pending.length > 0 && (
                        <Button type="button" size="icon" variant="ghost" onClick={() => void approveAll()} disabled={busy} title={t('agents.detail.approveAll')} className="size-6 text-muted-foreground hover:text-emerald-400">
                          <Check className="size-3.5" />
                        </Button>
                      )
                    }
                  />
                </div>
                {!pendingCollapsed &&
                  (view === 'tree' ? (
                    <TreeView
                      node={buildTree(pending)}
                      depth={0}
                      selected={selected}
                      onSelect={setSelected}
                      onStage={(p) => void stageFile(p)}
                      onStageDir={(n) => void stageDir(n)}
                      onDiscard={ops.discardFile ? (p, u) => discardFile(p, u) : undefined}
                      onDiscardDir={ops.discardFiles ? (n) => void discardDir(n) : undefined}
                      busy={busy}
                      t={t}
                      showStage={true}
                    />
                  ) : (
                    pending.map((f) => <FileRow key={f.path} file={f} selected={selected === f.path} onSelect={() => setSelected(f.path)} onStage={() => void stageFile(f.path)} onDiscard={ops.discardFile ? () => discardFile(f.path, f.status === '?') : undefined} busy={busy} t={t} />)
                  ))}
              </div>
            </ScrollArea>

            <div className="flex shrink-0 flex-col gap-1.5 border-t border-border p-1.5 pb-2.5">
              <div className="relative">
                <Input value={subject} onChange={(e) => setSubject(e.target.value)} placeholder={t('agents.detail.commitSubjectPlaceholder')} className="h-7 pr-7 text-xs" />
                {ops.generateCommitMessage && (
                  <button
                    type="button"
                    onClick={() => void handleGenerate()}
                    disabled={generating || busy}
                    title={generating ? t('agents.detail.generatingCommitMessage') : t('agents.detail.generateCommitMessage')}
                    className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded-sm p-0.5 text-muted-foreground hover:text-foreground disabled:opacity-40"
                  >
                    {generating ? <Loader2 className="size-3.5 animate-spin" /> : <Wand2 className="size-3.5" />}
                  </button>
                )}
              </div>
              {showBody ? (
                <div className="flex flex-col gap-1">
                  <Textarea value={body} onChange={(e) => setBody(e.target.value)} placeholder={t('agents.detail.commitBodyPlaceholder')} className="min-h-[52px] resize-none text-xs" />
                  <button
                    type="button"
                    onClick={() => {
                      setShowBody(false);
                      setBody('');
                    }}
                    className="self-start text-[10px] text-muted-foreground hover:text-foreground"
                  >
                    {t('agents.detail.hideDescription')}
                  </button>
                </div>
              ) : (
                <button type="button" onClick={() => setShowBody(true)} className="self-start text-[10px] text-muted-foreground hover:text-foreground">
                  {t('agents.detail.addDescription')}
                </button>
              )}
              {isProtected && <p className="px-1 text-[10px] text-amber-400">{t('agents.detail.protectedBranchWarn', { branch: gitState?.branch ?? '' })}</p>}
              <div className="flex">
                <Button type="button" size="sm" onClick={() => void runShip(primaryAction)} disabled={!canShip} className="h-8 flex-1 rounded-r-none">
                  <Check className="size-3.5" />
                  {primaryLabel}
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" size="sm" disabled={busy} className="h-8 rounded-l-none border-l border-primary-foreground/20 px-2">
                      <ChevronDown className="size-3.5" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="min-w-44">
                    <DropdownMenuItem onSelect={() => void runShip('amend')}>{t('agents.detail.amend')}</DropdownMenuItem>
                    {primaryAction !== 'commit-only' && (
                      <DropdownMenuItem onSelect={() => void runShip('commit-only')} disabled={stagedCount === 0}>
                        {t('agents.detail.commitOnly')}
                      </DropdownMenuItem>
                    )}
                    {primaryAction !== 'push-only' && (
                      <DropdownMenuItem onSelect={() => void runShip('push-only')} disabled={aheadCount === 0}>
                        {t('agents.detail.push')}
                      </DropdownMenuItem>
                    )}
                    <DropdownMenuItem onSelect={() => void runShip('sync')}>{t('agents.detail.sync')}</DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onSelect={() => void runShip('push-with-lease')} disabled={aheadCount === 0} className="text-amber-400 focus:text-amber-400">
                      {t('agents.detail.forcePush')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
              {onClose && (
                <label className="flex cursor-pointer items-center gap-1.5 text-[10px] text-muted-foreground">
                  <Checkbox checked={closeAfterShip} onCheckedChange={(v) => setCloseAfterShip(Boolean(v))} className="size-3" />
                  {t('agents.detail.closeSession')}
                </label>
              )}
            </div>
          </div>

          {/* biome-ignore lint/a11y/useSemanticElements: resize handle needs a flex container with a visible thumb, not an <hr> */}
          <div
            role="separator"
            aria-orientation="vertical"
            aria-valuenow={listWidth}
            aria-valuemin={LIST_WIDTH_MIN}
            aria-valuemax={LIST_WIDTH_MAX}
            tabIndex={0}
            onPointerDown={onResizeStart}
            onKeyDown={(e) => {
              if (e.key === 'ArrowLeft') {
                e.preventDefault();
                setListWidth((w) => clampListWidth(w - 16));
              } else if (e.key === 'ArrowRight') {
                e.preventDefault();
                setListWidth((w) => clampListWidth(w + 16));
              }
            }}
            className="group flex w-3 shrink-0 cursor-col-resize items-center justify-center outline-none"
          >
            <div className="h-full w-px bg-border transition-colors group-hover:bg-muted-foreground/40 group-focus-visible:bg-muted-foreground/40" />
          </div>

          {selectedDiff ? (
            <DiffPanel file={selectedDiff} onOpenFile={onOpenFile} />
          ) : (
            <div className="flex h-full flex-1 flex-col items-center justify-center gap-2 rounded-md border border-dashed border-border p-8 text-center text-xs text-muted-foreground">
              <FileEdit className="size-5 opacity-60" />
              <span>{t('agents.detail.filesSelectFile')}</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function FileRow({ file, selected, onSelect, onStage, onUnstage, onDiscard, busy, t, indent = 0 }: { file: FileStatus; selected: boolean; onSelect: () => void; onStage?: () => void; onUnstage?: () => void; onDiscard?: () => void; busy: boolean; t: (key: string) => string; indent?: number }) {
  const { name, dir } = splitPath(file.path);
  const status = statusLabel(file.status);
  return (
    <button type="button" onClick={onSelect} style={{ paddingLeft: `${0.5 + indent * 0.75}rem` }} className={cn('group flex w-full items-center gap-2 rounded-sm py-1 pr-1.5 text-left text-xs hover:bg-muted/60', selected && 'bg-muted')}>
      <span className="flex min-w-0 flex-1 items-baseline gap-2">
        <span className="shrink-0 font-medium text-foreground">{name}</span>
        {dir && <span className="min-w-0 truncate text-[10px] text-muted-foreground">{dir}</span>}
      </span>
      <span className={cn('shrink-0 font-mono text-[10px]', STATUS_CLASS[status] ?? 'text-muted-foreground')}>{status}</span>
      {onStage && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onStage();
          }}
          disabled={busy}
          title={t('agents.detail.approveFile')}
          className="shrink-0 rounded-sm p-0.5 text-muted-foreground opacity-0 hover:bg-emerald-500/20 hover:text-emerald-400 group-hover:opacity-100 disabled:opacity-30"
        >
          <Plus className="size-3.5" />
        </button>
      )}
      {onUnstage && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onUnstage();
          }}
          disabled={busy}
          title={t('agents.detail.unapproveFile')}
          className="shrink-0 rounded-sm p-0.5 text-muted-foreground opacity-0 hover:bg-muted hover:text-foreground group-hover:opacity-100 disabled:opacity-30"
        >
          <Undo2 className="size-3.5" />
        </button>
      )}
      {onDiscard && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onDiscard();
          }}
          disabled={busy}
          title={t('agents.detail.discardFile')}
          className="shrink-0 rounded-sm p-0.5 text-muted-foreground opacity-0 hover:bg-red-500/20 hover:text-red-400 group-hover:opacity-100 disabled:opacity-30"
        >
          <Trash2 className="size-3.5" />
        </button>
      )}
    </button>
  );
}

type TreeNode = {
  name: string;
  fullPath: string;
  file?: FileStatus;
  children: Map<string, TreeNode>;
};

function buildTree(files: FileStatus[]): TreeNode {
  const root: TreeNode = { name: '', fullPath: '', children: new Map() };
  for (const f of files) {
    const parts = f.path.split('/');
    let node = root;
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const isLeaf = i === parts.length - 1;
      let child = node.children.get(part);
      if (!child) {
        child = { name: part, fullPath: parts.slice(0, i + 1).join('/'), children: new Map() };
        node.children.set(part, child);
      }
      if (isLeaf) {
        child.file = f;
      }
      node = child;
    }
  }
  return root;
}

function collectPaths(node: TreeNode): string[] {
  const paths: string[] = [];
  const walk = (n: TreeNode) => {
    if (n.file) {
      paths.push(n.file.path);
    }
    for (const child of n.children.values()) {
      walk(child);
    }
  };
  walk(node);
  return paths;
}

function TreeView({
  node,
  depth,
  selected,
  onSelect,
  onStage,
  onUnstage,
  onStageDir,
  onUnstageDir,
  onDiscard,
  onDiscardDir,
  busy,
  t,
  showStage,
}: {
  node: TreeNode;
  depth: number;
  selected: string | null;
  onSelect: (path: string) => void;
  onStage?: (path: string) => void;
  onUnstage?: (path: string) => void;
  onStageDir?: (node: TreeNode) => void;
  onUnstageDir?: (node: TreeNode) => void;
  onDiscard?: (path: string, untracked: boolean) => void;
  onDiscardDir?: (node: TreeNode) => void;
  busy: boolean;
  t: (key: string) => string;
  showStage: boolean;
}) {
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const rows: React.ReactNode[] = [];
  const walk = (n: TreeNode, level: number) => {
    const children = Array.from(n.children.values()).sort((a, b) => {
      const aDir = a.children.size > 0 && !a.file ? 0 : 1;
      const bDir = b.children.size > 0 && !b.file ? 0 : 1;
      if (aDir !== bDir) {
        return aDir - bDir;
      }
      return a.name.localeCompare(b.name);
    });
    for (const child of children) {
      const isDir = child.children.size > 0 && !child.file;
      const isOpen = open[child.fullPath] ?? true;
      if (isDir) {
        const dirNode = child;
        rows.push(
          <button key={`d:${child.fullPath}`} type="button" onClick={() => setOpen((prev) => ({ ...prev, [child.fullPath]: !isOpen }))} style={{ paddingLeft: `${0.5 + level * 0.75}rem` }} className="group flex w-full items-center gap-1 rounded-sm py-1 pr-1.5 text-left text-xs text-muted-foreground hover:bg-muted/60">
            {isOpen ? <ChevronDown className="size-3.5 shrink-0" /> : <ChevronRight className="size-3.5 shrink-0" />}
            <span className="min-w-0 flex-1 truncate">{child.name}</span>
            {showStage && onStageDir && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onStageDir(dirNode);
                }}
                disabled={busy}
                title={t('agents.detail.approveFile')}
                className="shrink-0 rounded-sm p-0.5 text-muted-foreground opacity-0 hover:bg-emerald-500/20 hover:text-emerald-400 group-hover:opacity-100 disabled:opacity-30"
              >
                <Plus className="size-3.5" />
              </button>
            )}
            {!showStage && onUnstageDir && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onUnstageDir(dirNode);
                }}
                disabled={busy}
                title={t('agents.detail.unapproveFile')}
                className="shrink-0 rounded-sm p-0.5 text-muted-foreground opacity-0 hover:bg-muted hover:text-foreground group-hover:opacity-100 disabled:opacity-30"
              >
                <Undo2 className="size-3.5" />
              </button>
            )}
            {showStage && onDiscardDir && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onDiscardDir(dirNode);
                }}
                disabled={busy}
                title={t('agents.detail.discardFile')}
                className="shrink-0 rounded-sm p-0.5 text-muted-foreground opacity-0 hover:bg-red-500/20 hover:text-red-400 group-hover:opacity-100 disabled:opacity-30"
              >
                <Trash2 className="size-3.5" />
              </button>
            )}
          </button>,
        );
        if (isOpen) {
          walk(child, level + 1);
        }
      } else if (child.file) {
        const f = child.file;
        rows.push(
          <FileRow
            key={`f:${f.path}`}
            file={f}
            selected={selected === f.path}
            onSelect={() => onSelect(f.path)}
            onStage={showStage && onStage ? () => onStage(f.path) : undefined}
            onUnstage={!showStage && onUnstage ? () => onUnstage(f.path) : undefined}
            onDiscard={showStage && onDiscard ? () => onDiscard(f.path, f.status === '?') : undefined}
            busy={busy}
            t={t}
            indent={level + 1.5}
          />,
        );
      }
    }
  };
  walk(node, depth);
  return <>{rows}</>;
}

function SectionHeader({ label, tone, action, collapsed, onToggle }: { label: string; tone: 'approved' | 'pending'; action?: React.ReactNode; collapsed?: boolean; onToggle?: () => void }) {
  return (
    <div className="flex items-center justify-between gap-2 pr-1">
      <button type="button" onClick={onToggle} className="flex flex-1 items-center gap-1 px-1.5 py-1 text-left hover:bg-muted/40">
        {collapsed ? <ChevronRight className="size-3 text-muted-foreground" /> : <ChevronDown className="size-3 text-muted-foreground" />}
        <span className={cn('text-[10px] font-semibold uppercase tracking-wider', tone === 'approved' ? 'text-emerald-400' : 'text-amber-400')}>{label}</span>
      </button>
      {action}
    </div>
  );
}

function DiffPanel({ file, onOpenFile }: { file: DiffFile; onOpenFile?: (path: string, line: number) => void }) {
  const { t } = useTranslation();
  const lang = getLang(file.path);
  const firstLine = file.hunks[0] ? parseHunkStart(file.hunks[0].header).newStart : 1;
  return (
    <div className="flex h-full min-h-0 flex-1 flex-col overflow-hidden rounded-md border border-border">
      <div className="flex shrink-0 items-center justify-between border-b border-border bg-muted/60 px-3 py-1.5">
        <span className="min-w-0 truncate font-mono text-xs text-foreground">{file.path}</span>
        {onOpenFile && (
          <button type="button" onClick={() => onOpenFile(file.path, firstLine)} title={t('agents.detail.openInIde')} className="shrink-0 rounded-sm p-0.5 text-muted-foreground hover:text-foreground">
            <SquareArrowOutUpRight className="size-3.5" />
          </button>
        )}
      </div>
      <ScrollArea className="h-0 flex-1">
        {file.hunks.map((hunk) => {
          const highlighted = highlightLines(hunk.lines, lang);
          const { oldStart, newStart } = parseHunkStart(hunk.header);
          let oldLine = oldStart;
          let newLine = newStart;
          return (
            <div key={`${file.path}:${hunk.header}`}>
              <div className="bg-blue-950/30 px-3 py-0.5 font-mono text-xs text-blue-400">{hunk.header}</div>
              <div>
                {hunk.lines.map((line, li) => {
                  let ol: number | null = null;
                  let nl: number | null = null;
                  if (line.type === ' ') {
                    ol = oldLine++;
                    nl = newLine++;
                  } else if (line.type === '-') {
                    ol = oldLine++;
                  } else if (line.type === '+') {
                    nl = newLine++;
                  }
                  return (
                    // biome-ignore lint/suspicious/noArrayIndexKey: parsed diff lines are stable and may repeat content (blank context lines)
                    <div key={li} className={cn('flex items-start', line.type === '+' && 'bg-emerald-950/40 text-emerald-300', line.type === '-' && 'bg-red-950/40 text-red-300', line.type !== '+' && line.type !== '-' && 'text-muted-foreground')}>
                      <span className="inline-flex h-[1.375rem] w-5 shrink-0 select-none items-center justify-center font-mono text-[10px] leading-none opacity-50">{line.type === '+' || line.type === '-' ? line.type : ' '}</span>
                      <span className="inline-flex h-[1.375rem] w-9 shrink-0 select-none items-center justify-end font-mono text-[10px] leading-none opacity-30">{ol ?? ''}</span>
                      <span className="inline-flex h-[1.375rem] w-9 shrink-0 select-none items-center justify-end font-mono text-[10px] leading-none opacity-30">{nl ?? ''}</span>
                      {/* biome-ignore lint/security/noDangerouslySetInnerHtml: highlight.js output from local git diff */}
                      <span className="min-w-0 flex-1 whitespace-pre-wrap break-all font-mono text-xs leading-[1.375rem]" dangerouslySetInnerHTML={{ __html: highlighted[li] ?? '' }} />
                    </div>
                  );
                })}
              </div>
            </div>
          );
        })}
      </ScrollArea>
    </div>
  );
}
