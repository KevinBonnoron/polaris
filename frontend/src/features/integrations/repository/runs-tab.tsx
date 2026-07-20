import { Check, ChevronDown, ExternalLink, Loader2, RotateCw, Timer, Workflow, X } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { FilterBar, type FilterToken } from '@/components/atoms/filter-bar';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { getWorkflowRunsEntry } from '@/collections/github.repository.collection';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { CancelRepoWorkflowRun, RerunRepoWorkflowRun } from '@/wailsjs/go/main/App';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { FILTER_ALL } from './list-filters';
import { ListShell } from './list-shell';
import { TriggerWorkflowPopover } from './trigger-workflow-popover';
import type { WorkflowDispatchSpec, WorkflowRun } from './types';
import { useGhEntry } from './use-gh-entry';
import { type ReloadRegister, useRegisterReload } from './use-register-reload';
import { useWorkflowDispatches } from './use-workflow-dispatches';
import { formatAgo, formatDuration, runColor, runDurationSeconds, runLabel } from './utils';

interface Props {
  owner: string;
  repo: string;
  onRegister?: ReloadRegister;
  defaultBranch?: string;
}

function isActive(run: WorkflowRun): boolean {
  return run.status !== 'completed';
}

function useTick(active: boolean, intervalMs = 1000) {
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!active) {
      return;
    }
    const id = window.setInterval(() => setTick((t) => t + 1), intervalMs);
    return () => window.clearInterval(id);
  }, [active, intervalMs]);
}

export function RunsTab({ owner, repo, onRegister, defaultBranch }: Props) {
  const { t } = useTranslation();
  const entry = useMemo(() => getWorkflowRunsEntry(owner, repo), [owner, repo]);
  const { data, loading, initial, error, hasMore, reload, loadMore } = useGhEntry(entry);
  useRegisterReload(onRegister, { reload, loading });
  const [tokens, setTokens] = useState<FilterToken[]>(defaultBranch ? [{ key: 'branch', value: defaultBranch }] : []);

  useEffect(() => {
    if (defaultBranch) {
      setTokens((prev) => [...prev.filter((t) => t.key !== 'branch'), { key: 'branch', value: defaultBranch }]);
    }
  }, [defaultBranch]);

  const branch = tokens.find((t) => t.key === 'branch')?.value ?? FILTER_ALL;
  const query = tokens.find((t) => t.key === 'term')?.value ?? '';

  const sorted = useMemo(() => [...data].sort((a, b) => b.createdAt - a.createdAt), [data]);
  const hasActive = useMemo(() => sorted.some(isActive), [sorted]);
  useTick(hasActive);
  const branches = useMemo(() => Array.from(new Set(sorted.map((r) => r.branch).filter(Boolean))).sort(), [sorted]);
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return sorted.filter((run) => {
      if (branch !== FILTER_ALL && run.branch !== branch) {
        return false;
      }
      if (!q) {
        return true;
      }
      return (run.name || '').toLowerCase().includes(q);
    });
  }, [sorted, query, branch]);
  const unnamedLabel = t('integrations.repository.unnamedWorkflow');
  const groups = useMemo(() => groupByWorkflow(filtered, unnamedLabel), [filtered, unnamedLabel]);
  const workflowIds = useMemo(() => groups.map(([, runs]) => runs[0].workflowId).filter((id) => id > 0), [groups]);
  const specs = useWorkflowDispatches(owner, repo, workflowIds);

  const branchOptions = useMemo(() => branches.map((b) => ({ value: b, label: b })), [branches]);
  const defs = useMemo(() => [{ key: 'branch', label: t('integrations.repository.branch'), options: branchOptions }], [branchOptions, t]);

  const filters = <FilterBar tokens={tokens} onTokensChange={setTokens} defs={defs} placeholder={t('integrations.repository.searchRuns')} />;

  return (
    <ListShell title={t('integrations.repository.recentRuns')} initial={initial} error={error} empty={filtered.length === 0} emptyText={t('integrations.repository.noRuns')} filters={filters}>
      {groups.map(([name, runs]) => (
        <WorkflowGroup key={name} name={name} runs={runs} owner={owner} repo={repo} spec={specs.get(runs[0].workflowId)} onTriggered={reload} onCancelled={reload} />
      ))}
      {hasMore && (
        <div className="flex justify-center pt-2">
          <Button variant="outline" size="sm" onClick={loadMore} disabled={loading}>
            {loading ? <Loader2 className="size-3.5 animate-spin" /> : null}
            {t('integrations.repository.loadMore')}
          </Button>
        </div>
      )}
    </ListShell>
  );
}

function RunDurationLabel({ run }: { run: WorkflowRun }) {
  const seconds = runDurationSeconds(run);
  if (seconds === null) {
    return null;
  }
  return (
    <span className="inline-flex items-center gap-1">
      <Timer className="size-3" />
      {formatDuration(seconds)}
    </span>
  );
}

interface GroupProps {
  name: string;
  runs: WorkflowRun[];
  owner: string;
  repo: string;
  spec: WorkflowDispatchSpec | null | undefined;
  onTriggered: () => void;
  onCancelled: () => void;
}

function WorkflowGroup({ name, runs, owner, repo, spec, onTriggered, onCancelled }: GroupProps) {
  const { t, i18n } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const latest = runs[0];
  const canTrigger = Boolean(spec?.dispatchable) && latest.workflowId > 0;

  return (
    <div className="overflow-hidden rounded-md border">
      <div className="flex items-stretch">
        <button type="button" onClick={() => setExpanded((v) => !v)} className="flex min-w-0 flex-1 items-center gap-3 px-3 py-2 text-left transition-colors hover:bg-accent">
          <Workflow className={cn('size-4 shrink-0', runColor(latest))} />
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{name}</div>
            <div className="flex items-center gap-1.5 truncate text-xs text-muted-foreground">
              <span className="truncate">{t('integrations.repository.runGroupSubtitle', { count: runs.length, label: runLabel(latest), when: formatAgo(latest.createdAt, i18n.language) })}</span>
              <RunDurationLabel run={latest} />
            </div>
          </div>
        </button>
        <div className="flex items-center gap-1 pr-2">
          {isActive(latest) && <CancelRunButton owner={owner} repo={repo} run={latest} onCancelled={onCancelled} />}
          {canTrigger && spec && <TriggerWorkflowPopover owner={owner} repo={repo} workflowId={latest.workflowId} workflowName={name} defaultRef={latest.branch || 'main'} spec={spec} onTriggered={onTriggered} />}
          <button type="button" onClick={() => setExpanded((v) => !v)} aria-label={expanded ? t('integrations.repository.collapse') : t('integrations.repository.expand')} className="flex items-center px-1 text-muted-foreground transition-colors hover:text-foreground">
            <ChevronDown className={cn('size-4 transition-transform', expanded && 'rotate-180')} />
          </button>
        </div>
      </div>
      {expanded && (
        <div className="flex flex-col gap-1 border-t bg-muted/30 px-2 py-2">
          {runs.map((run) => (
            <RunRow key={run.id} run={run} owner={owner} repo={repo} onCancelled={onCancelled} onRerun={onTriggered} />
          ))}
        </div>
      )}
    </div>
  );
}

interface RunRowProps {
  run: WorkflowRun;
  owner: string;
  repo: string;
  onCancelled: () => void;
  onRerun: () => void;
}

function RunRow({ run, owner, repo, onCancelled, onRerun }: RunRowProps) {
  const { t, i18n } = useTranslation();
  const active = isActive(run);
  return (
    <div className="flex items-center gap-2 rounded-md px-2 py-1">
      <Workflow className={cn('size-4 shrink-0', runColor(run))} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{t('integrations.repository.runRowTitle', { branch: run.branch || '—', id: run.id })}</div>
        <div className="flex items-center gap-1.5 truncate text-xs text-muted-foreground">
          <span className="truncate">{t('integrations.repository.runRowSubtitle', { label: runLabel(run), event: run.event || '—', when: formatAgo(run.createdAt, i18n.language) })}</span>
          <RunDurationLabel run={run} />
        </div>
      </div>
      {active && <CancelRunButton owner={owner} repo={repo} run={run} onCancelled={onCancelled} />}
      {!active && <RerunRunButton owner={owner} repo={repo} run={run} onRerun={onRerun} />}
      <Button variant="ghost" size="sm" className="h-7 px-2" onClick={() => BrowserOpenURL(run.url)} aria-label={t('integrations.repository.openRun', { id: run.id })}>
        <ExternalLink className="size-3.5" />
      </Button>
    </div>
  );
}

interface RerunRunButtonProps {
  owner: string;
  repo: string;
  run: WorkflowRun;
  onRerun: () => void;
}

function RerunRunButton({ owner, repo, run, onRerun }: RerunRunButtonProps) {
  const { t } = useTranslation();
  const [pending, setPending] = useState(false);

  const handleClick = async () => {
    if (pending) {
      return;
    }
    setPending(true);
    try {
      await RerunRepoWorkflowRun(owner, repo, run.id);
      toast.success(t('integrations.repository.rerunRun', { id: run.id }));
      onRerun();
    } catch (err) {
      toastError({ title: t('integrations.repository.rerunFailed'), err });
    } finally {
      setPending(false);
    }
  };

  return (
    <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs" onClick={handleClick} disabled={pending} aria-label={t('integrations.repository.rerunAria', { id: run.id })}>
      {pending ? <Loader2 className="size-3.5 animate-spin" /> : <RotateCw className="size-3.5" />}
      {t('integrations.repository.rerun')}
    </Button>
  );
}

interface CancelRunButtonProps {
  owner: string;
  repo: string;
  run: WorkflowRun;
  onCancelled: () => void;
}

function CancelRunButton({ owner, repo, run, onCancelled }: CancelRunButtonProps) {
  const { t } = useTranslation();
  const [pending, setPending] = useState(false);

  const handleClick = async () => {
    if (pending) {
      return;
    }
    setPending(true);
    try {
      await CancelRepoWorkflowRun(owner, repo, run.id);
      toast.success(t('integrations.repository.cancelledRun', { id: run.id }));
      onCancelled();
    } catch (err) {
      toastError({ title: t('integrations.repository.cancelFailed'), err });
    } finally {
      setPending(false);
    }
  };

  return (
    <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs" onClick={handleClick} disabled={pending} aria-label={t('integrations.repository.cancelAria', { id: run.id })}>
      {pending ? <Loader2 className="size-3.5 animate-spin" /> : <X className="size-3.5" />}
      {t('integrations.repository.cancel')}
    </Button>
  );
}

function groupByWorkflow(runs: WorkflowRun[], unnamedLabel: string): Array<[string, WorkflowRun[]]> {
  const map = new Map<string, WorkflowRun[]>();
  for (const run of runs) {
    const key = run.name || unnamedLabel;
    const list = map.get(key);
    if (list) {
      list.push(run);
    } else {
      map.set(key, [run]);
    }
  }
  return Array.from(map.entries());
}
