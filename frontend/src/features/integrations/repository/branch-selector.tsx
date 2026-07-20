import { Check, ChevronDown, CloudDownload, GitBranch, GitBranchPlus, ShieldAlert, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { ClickToConfirm } from '@/components/atoms/click-to-confirm';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { CreateProjectBranch, DeleteProjectBranch, GetProjectGitState, ListProjectBranches, SwitchProjectBranch } from '@/wailsjs/go/main/App';
import type { git as gitModels } from '@/wailsjs/go/models';

type Branch = gitModels.BranchInfo;

interface Props {
  projectId: string;
  onBranchChange?: (branch: string) => void;
}

export function BranchSelector({ projectId, onBranchChange }: Props) {
  const { t } = useTranslation();
  const [branches, setBranches] = useState<Branch[]>([]);
  const [currentBranch, setCurrentBranch] = useState<string | null>(null);
  const [aheadCount, setAheadCount] = useState(0);
  const [isProtected, setIsProtected] = useState(false);
  const [switching, setSwitching] = useState(false);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newBranchName, setNewBranchName] = useState('');
  const [creating, setCreating] = useState(false);
  const newBranchInputRef = useRef<HTMLInputElement>(null);

  const refreshSeq = useRef(0);
  const refresh = useCallback(async () => {
    const seq = ++refreshSeq.current;
    const [b, g] = await Promise.all([ListProjectBranches(projectId).catch(() => null), GetProjectGitState(projectId).catch(() => null)]);
    if (seq !== refreshSeq.current) return;
    if (b) setBranches(b);
    if (g) {
      setCurrentBranch(g.branch);
      setAheadCount(g.aheadCount);
      setIsProtected(g.isProtected);
      if (g.branch) onBranchChange?.(g.branch);
    }
  }, [projectId]);

  useEffect(() => {
    void refresh();
    const id = window.setInterval(() => void refresh(), 2000);
    return () => window.clearInterval(id);
  }, [refresh]);

  const switchBranch = async (branch: string) => {
    if (switching) return;
    setSwitching(true);
    try {
      await SwitchProjectBranch(projectId, branch);
      toast.success(t('integrations.repository.switchedBranch', { branch }));
      await refresh();
    } catch (err) {
      toastError({ title: t('integrations.repository.switchBranchFailed'), err });
    } finally {
      setSwitching(false);
    }
  };

  const createBranch = async () => {
    const name = newBranchName.trim();
    if (!name) return;
    setCreating(true);
    try {
      await CreateProjectBranch(projectId, name, '');
      setShowCreateDialog(false);
      setNewBranchName('');
      try {
        await SwitchProjectBranch(projectId, name);
        toast.success(t('integrations.repository.branchCreated', { branch: name }));
      } catch (err) {
        toastError({ title: t('integrations.repository.switchBranchFailed'), err });
      }
      await refresh();
    } catch (err) {
      toastError({ title: t('integrations.repository.createBranchFailed'), err });
    } finally {
      setCreating(false);
    }
  };

  const deleteBranch = async (branch: string) => {
    try {
      await DeleteProjectBranch(projectId, branch, false);
      toast.success(t('integrations.repository.branchDeleted', { branch }));
      await refresh();
    } catch (err) {
      toastError({ title: t('integrations.repository.deleteBranchFailed'), err });
    }
  };

  return (
    <>
      <div className="flex items-center gap-2 text-xs">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="h-7 gap-1.5 px-2 font-mono text-xs" disabled={switching || branches.length === 0}>
              <GitBranch className="size-3.5 text-muted-foreground" />
              {currentBranch ?? '—'}
              {isProtected && <ShieldAlert className="size-3 text-amber-400" />}
              <ChevronDown className="size-3 opacity-60" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="max-h-80 min-w-56 overflow-y-auto">
            {branches
              .filter((b) => !b.isRemote)
              .map((b) => {
                const blocked = Boolean(b.worktreePath);
                const canDelete = !b.isCurrent && !blocked && !b.isProtected;
                return (
                  <DropdownMenuItem
                    key={b.name}
                    disabled={blocked || b.isCurrent || switching}
                    onSelect={() => {
                      if (!blocked && !b.isCurrent) void switchBranch(b.name);
                    }}
                    className="flex items-center justify-between gap-2 font-mono text-xs"
                  >
                    <span className="flex min-w-0 items-center gap-1.5">
                      {b.isCurrent ? <Check className="size-3.5 shrink-0 text-emerald-400" /> : <span className="size-3.5 shrink-0" />}
                      <span className={cn('truncate', blocked && 'text-muted-foreground')}>{b.name}</span>
                    </span>
                    <span className="flex shrink-0 items-center gap-1">
                      {blocked && <span className="text-[10px] text-amber-400">{t('integrations.repository.inWorktree')}</span>}
                      {canDelete && (
                        <ClickToConfirm onConfirm={() => void deleteBranch(b.name)} icon={<Trash2 className="size-3" />} confirmIcon={<Check className="size-3" />} aria-label={t('integrations.repository.deleteBranch')} confirmAriaLabel={t('integrations.repository.confirmDeleteBranch', { branch: b.name })} />
                      )}
                    </span>
                  </DropdownMenuItem>
                );
              })}
            {branches.some((b) => b.isRemote) && (
              <>
                <DropdownMenuSeparator />
                <p className="px-2 py-1 text-[10px] uppercase tracking-wide text-muted-foreground">{t('integrations.repository.remoteBranches')}</p>
                {branches
                  .filter((b) => b.isRemote)
                  .map((b) => (
                    <DropdownMenuItem key={b.name} disabled={switching} onSelect={() => void switchBranch(b.name)} className="flex items-center gap-2 font-mono text-xs">
                      <CloudDownload className="size-3.5 shrink-0 text-muted-foreground" />
                      <span className="truncate">{b.name}</span>
                    </DropdownMenuItem>
                  ))}
              </>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setShowCreateDialog(true)} className="gap-1.5 text-xs">
              <GitBranchPlus className="size-3.5 text-muted-foreground" />
              {t('integrations.repository.createBranch')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        {aheadCount > 0 && <span className="text-muted-foreground">↑{aheadCount}</span>}
      </div>

      <Dialog
        open={showCreateDialog}
        onOpenChange={(open) => {
          setShowCreateDialog(open);
          if (!open) setNewBranchName('');
        }}
      >
        <DialogContent className="sm:max-w-sm" onOpenAutoFocus={() => newBranchInputRef.current?.focus()}>
          <DialogHeader>
            <DialogTitle>{t('integrations.repository.createBranch')}</DialogTitle>
          </DialogHeader>
          <Input
            ref={newBranchInputRef}
            value={newBranchName}
            onChange={(e) => setNewBranchName(e.target.value)}
            placeholder={t('integrations.repository.createBranchPlaceholder')}
            className="font-mono text-sm"
            onKeyDown={(e) => {
              if (e.key === 'Enter') void createBranch();
            }}
          />
          <DialogFooter>
            <Button variant="ghost" size="sm" onClick={() => setShowCreateDialog(false)}>
              {t('common.cancel')}
            </Button>
            <Button size="sm" onClick={() => void createBranch()} disabled={!newBranchName.trim() || creating}>
              {t('integrations.repository.createBranch')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
