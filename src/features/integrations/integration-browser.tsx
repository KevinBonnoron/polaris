import { Search, Sparkles } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { projectsCollection } from '@/collections/projects.collection';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import type { Project } from '@/types';
import { ConfigureIntegrationModal } from './configure-integration-modal';
import { IntegrationCard } from './integration-card';
import { INTEGRATIONS } from './integration-catalog';
import { effectiveStorageKey, isIntegrationConnected, withIntegration } from './project-integrations';

interface Props {
  project: Project;
  className?: string;
  scrollClassName?: string;
}

export function IntegrationBrowser({ project, className, scrollClassName }: Props) {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const [configureId, setConfigureId] = useState<string | null>(null);
  const [detecting, setDetecting] = useState(false);

  const { available, connected } = useMemo(() => {
    const q = query.trim().toLowerCase();
    const matches = INTEGRATIONS.filter((i) => !q || i.name.toLowerCase().includes(q));
    return {
      available: matches.filter((i) => !isIntegrationConnected(project, i)),
      connected: matches.filter((i) => isIntegrationConnected(project, i)),
    };
  }, [query, project]);

  const detect = async () => {
    if (!project.path) {
      return;
    }
    const path = project.path;
    setDetecting(true);
    try {
      const seenStorageKeys = new Set<string>();
      const results = await Promise.all(
        INTEGRATIONS.map(async (i) => {
          const key = effectiveStorageKey(i);
          if (!i.detect || isIntegrationConnected(project, i) || seenStorageKeys.has(key)) {
            return null;
          }
          seenStorageKeys.add(key);
          try {
            const result = await i.detect(path);
            if (!result) {
              return null;
            }
            let config: Record<string, unknown>;
            if (i.multi && Array.isArray(result)) {
              config = { _instances: result };
            } else if (i.multi && !Array.isArray(result)) {
              config = { _instances: [result] };
            } else if (!Array.isArray(result)) {
              config = { ...i.fixedValues, ...result };
            } else {
              return null;
            }
            return { storageKey: key, name: i.name, config } as const;
          } catch {
            return null;
          }
        }),
      );
      const added = results.filter((r): r is { storageKey: string; name: string; config: Record<string, unknown> } => r !== null);
      if (added.length === 0) {
        toast.message(t('projects.settings.redetectedNone'));
        return;
      }
      const tx = projectsCollection.update(project.id, (draft) => {
        let next = draft.integrations ?? {};
        for (const item of added) {
          next = withIntegration({ ...draft, integrations: next }, item.storageKey, item.config);
        }
        draft.integrations = next;
      });
      await tx.isPersisted.promise;
      const names = added.map((a) => a.name).join(', ');
      toast.success(t('projects.settings.redetectedAdded', { count: added.length }), { description: names });
    } catch (err) {
      toastError({ title: t('projects.settings.couldNotRedetect'), err });
    } finally {
      setDetecting(false);
    }
  };

  return (
    <div className={cn('flex min-h-0 flex-col gap-4', className)}>
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search className="-translate-y-1/2 absolute top-1/2 left-3 size-4 text-muted-foreground" />
          <Input autoFocus value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('integrations.add.searchPlaceholder')} className="h-10 pl-9" />
        </div>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button type="button" variant="outline" size="icon" onClick={detect} disabled={detecting || !project.path} className="size-10 shrink-0" aria-label={t('projects.settings.redetect')}>
              <Sparkles className={cn('size-4', detecting && 'animate-pulse')} />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="bottom">{detecting ? t('projects.settings.redetecting') : t('projects.settings.redetect')}</TooltipContent>
        </Tooltip>
      </div>

      <ScrollArea className={cn('-mr-2 pr-2', scrollClassName ?? 'max-h-[60vh]')}>
        {available.length === 0 && connected.length === 0 ? (
          <div className="py-12 text-center text-sm text-muted-foreground">{t('integrations.add.noMatch', { query })}</div>
        ) : (
          <div className="flex flex-col gap-4">
            {available.length > 0 && (
              <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                {available.map((integration) => (
                  <IntegrationCard key={integration.id} integration={integration} connected={false} onToggle={setConfigureId} />
                ))}
              </div>
            )}
            {connected.length > 0 && (
              <div className="flex flex-col gap-2">
                <span className="px-1 text-muted-foreground text-xs">{t('integrations.add.connectedSection')}</span>
                <div className="grid grid-cols-1 gap-2 opacity-60 md:grid-cols-2">
                  {connected.map((integration) => (
                    <IntegrationCard key={integration.id} integration={integration} connected={true} onToggle={setConfigureId} />
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </ScrollArea>

      {configureId && <ConfigureIntegrationModal projectId={project.id} integrationId={configureId} open={true} onOpenChange={(o) => !o && setConfigureId(null)} />}
    </div>
  );
}
