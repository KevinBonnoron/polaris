import { ChevronDown, History, Pencil, Play, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { evictAutomationRunsCollection, getAutomationRunsCollection } from '@/collections/automation-runs.collection';
import { automationsCollection } from '@/collections/automations.collection';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
import { formatAgo } from '@/lib/format-ago';
import { cn } from '@/lib/utils';
import type { Automation } from '@/types';
import { FireAutomation } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';
import { useLiveQuery } from '@tanstack/react-db';
import { AutomationEditModal } from './automation-edit-modal';
import { actionsSummary, triggerSummary } from './automation-summary';

interface Props {
  automation: Automation;
  statusName?: string;
  fromStatusNames?: string[];
}

export function AutomationCard({ automation, statusName, fromStatusNames }: Props) {
  const { t, i18n } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [firing, setFiring] = useState(false);

  const runsCollection = useMemo(() => getAutomationRunsCollection(automation.id), [automation.id]);
  const { data: runs = [], isReady } = useLiveQuery((q) => (expanded ? q.from({ r: runsCollection }).orderBy(({ r }) => r.startedAt, 'desc') : undefined), [expanded, runsCollection]);
  const runsLoading = expanded && !isReady && runs.length === 0;

  const toggleEnabled = () => {
    automationsCollection.update(automation.id, (draft) => {
      draft.enabled = !automation.enabled;
    });
  };

  const handleFire = async () => {
    setFiring(true);
    try {
      await FireAutomation(automation.id);
      toast.success(t('automations.fired'));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setFiring(false);
    }
  };

  const handleDelete = async () => {
    try {
      await automationsCollection.delete(automation.id);
      evictAutomationRunsCollection(automation.id);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  };

  const lastRun = runs[0];

  return (
    <Card className={automation.enabled ? undefined : 'opacity-60'}>
      <CardContent className="flex flex-col gap-2 py-3">
        <div className="flex items-start gap-3">
          <Switch checked={automation.enabled} onCheckedChange={toggleEnabled} className="mt-1" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="truncate text-sm font-medium">{automation.name || t('automations.unnamed')}</span>
              <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase text-muted-foreground">{automation.source}</span>
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{triggerSummary(automation.trigger, statusName, fromStatusNames)}</p>
            <p className="text-xs text-muted-foreground">
              {t('automations.spawnLabel')} {actionsSummary(automation)}
            </p>
          </div>
          <div className="flex items-center gap-1">
            <Button type="button" variant="ghost" size="sm" onClick={() => setExpanded((v) => !v)} aria-label={expanded ? t('automations.runs.collapse') : t('automations.runs.expand')}>
              <History className="size-3.5" />
              <ChevronDown className={cn('ml-0.5 size-3 transition-transform', expanded && 'rotate-180')} />
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={handleFire} disabled={firing} aria-label={t('automations.fire')}>
              <Play className="size-3.5" />
            </Button>
            <AutomationEditModal automationId={automation.id}>
              <Button variant="ghost" size="sm">
                <Pencil className="size-3.5" />
              </Button>
            </AutomationEditModal>
            <Button variant="ghost" size="sm" onClick={handleDelete}>
              <Trash2 className="size-3.5 text-destructive" />
            </Button>
          </div>
        </div>

        {expanded && (
          <div className="flex flex-col gap-1.5 rounded-md border bg-muted/30 px-2 py-2">
            {runsLoading ? (
              <p className="px-1 py-0.5 text-xs text-muted-foreground">{t('common.loading')}</p>
            ) : runs.length === 0 ? (
              <p className="px-1 py-0.5 text-xs text-muted-foreground">{t('automations.runs.empty')}</p>
            ) : (
              <>
                {lastRun && (
                  <p className="px-1 pb-1 text-[10px] uppercase text-muted-foreground">
                    {t('automations.runs.lastRunPrefix')} {formatAgo(lastRun.startedAt, i18n.language)}
                  </p>
                )}
                <ul className="flex flex-col gap-1">
                  {runs.map((r) => (
                    <RunRow key={r.id} run={r} />
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function RunRow({ run }: { run: polaris.AutomationRun }) {
  const { i18n } = useTranslation();
  const ok = run.outcome === 'fired';
  return (
    <li className="flex items-start gap-2 rounded px-1 py-1">
      <span className={cn('mt-1 size-1.5 shrink-0 rounded-full', ok ? 'bg-emerald-500' : 'bg-destructive')} aria-hidden />
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs">{run.reason || run.outcome}</p>
        <p className="truncate text-[10px] text-muted-foreground">
          {formatAgo(run.startedAt, i18n.language)}
          {run.actions?.length ? <> · {run.actions.map((a) => `${a.kind}:${a.status}`).join(', ')}</> : null}
        </p>
      </div>
    </li>
  );
}
