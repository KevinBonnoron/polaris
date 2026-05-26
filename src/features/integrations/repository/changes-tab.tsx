import { Check, ChevronDown, GitBranch } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { type GitChangesOps, GitChangesPanel } from '@/features/git/git-changes-panel';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { CommitProject, GetProjectDiff, GetProjectFileStatuses, GetProjectGitState, ListProjectBranches, PushProject, StageProjectChanges, StageProjectFile, StageProjectFiles, SwitchProjectBranch, SyncProject, UnstageProjectChanges, UnstageProjectFile, UnstageProjectFiles } from '@/wailsjs/go/main/App';
import type { git as gitModels } from '@/wailsjs/go/models';

type Branch = gitModels.BranchInfo;

interface Props {
  projectId: string;
}

export function ChangesTab({ projectId }: Props) {
  const { t } = useTranslation();
  const [branches, setBranches] = useState<Branch[]>([]);
  const [currentBranch, setCurrentBranch] = useState<string | null>(null);
  const [aheadCount, setAheadCount] = useState(0);
  const [isProtected, setIsProtected] = useState(false);
  const [switching, setSwitching] = useState(false);

  const refreshBranches = useCallback(async () => {
    const [b, g] = await Promise.all([ListProjectBranches(projectId).catch(() => null), GetProjectGitState(projectId).catch(() => null)]);
    if (b) {
      setBranches(b);
    }
    if (g) {
      setCurrentBranch(g.branch);
      setAheadCount(g.aheadCount);
      setIsProtected(g.isProtected);
    }
  }, [projectId]);

  useEffect(() => {
    void refreshBranches();
    const id = window.setInterval(() => void refreshBranches(), 2000);
    return () => window.clearInterval(id);
  }, [refreshBranches]);

  const switchBranch = async (branch: string) => {
    if (switching) {
      return;
    }
    setSwitching(true);
    try {
      await SwitchProjectBranch(projectId, branch);
      toast.success(t('integrations.repository.switchedBranch', { branch }));
      await refreshBranches();
    } catch (err) {
      toastError({ title: t('integrations.repository.switchBranchFailed'), err });
    } finally {
      setSwitching(false);
    }
  };

  const ops: GitChangesOps = useMemo(
    () => ({
      getDiff: () => GetProjectDiff(projectId),
      getFileStatuses: () => GetProjectFileStatuses(projectId),
      getGitState: () => GetProjectGitState(projectId),
      stageFile: (path) => StageProjectFile(projectId, path),
      stageFiles: (paths) => StageProjectFiles(projectId, paths),
      unstageFile: (path) => UnstageProjectFile(projectId, path),
      unstageFiles: (paths) => UnstageProjectFiles(projectId, paths),
      stageAll: () => StageProjectChanges(projectId),
      unstageAll: () => UnstageProjectChanges(projectId),
      commit: (message, amend) => CommitProject(projectId, message, amend),
      push: (force) => PushProject(projectId, force),
      sync: (force) => SyncProject(projectId, force),
    }),
    [projectId],
  );

  const header = (
    <div className="flex shrink-0 items-center gap-2 text-xs">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" className="h-7 gap-1.5 px-2 font-mono text-xs" disabled={switching || branches.length === 0}>
            <GitBranch className="size-3.5 text-muted-foreground" />
            {currentBranch ?? '—'}
            <ChevronDown className="size-3 opacity-60" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="max-h-80 min-w-56 overflow-y-auto">
          {branches.map((b) => {
            const blocked = Boolean(b.worktreePath);
            return (
              <DropdownMenuItem
                key={b.name}
                disabled={blocked || b.isCurrent || switching}
                onSelect={() => {
                  if (!blocked && !b.isCurrent) {
                    void switchBranch(b.name);
                  }
                }}
                className="flex items-center justify-between gap-2 font-mono text-xs"
              >
                <span className="flex items-center gap-1.5">
                  {b.isCurrent ? <Check className="size-3.5 text-emerald-400" /> : <span className="size-3.5" />}
                  <span className={cn('truncate', blocked && 'text-muted-foreground')}>{b.name}</span>
                </span>
                {blocked && <span className="shrink-0 text-[10px] text-amber-400">{t('integrations.repository.inWorktree')}</span>}
              </DropdownMenuItem>
            );
          })}
        </DropdownMenuContent>
      </DropdownMenu>
      {aheadCount > 0 && <span className="text-muted-foreground">↑{aheadCount}</span>}
      {isProtected && <span className="text-amber-400">· protected</span>}
    </div>
  );

  return <GitChangesPanel ops={ops} pollInterval={2000} headerSlot={header} resetKey={projectId} />;
}
