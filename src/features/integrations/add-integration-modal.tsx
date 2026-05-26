import { Search, Sparkles } from 'lucide-react';
import { type PropsWithChildren, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { projectsCollection } from '@/collections/projects.collection';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { toastError } from '@/lib/toast-error';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { cn } from '@/lib/utils';
import { useCurrentProject } from '@/state/projects';
import { ConfigureIntegrationModal } from './configure-integration-modal';
import { IntegrationCard } from './integration-card';
import { INTEGRATIONS } from './integration-catalog';
import { isConnected, withIntegration } from './project-integrations';

export function AddIntegrationModal({ children, ...modeProps }: PropsWithChildren<DialogModeProps>) {
  const { t } = useTranslation();
  const { open, setOpen } = useDialogMode(modeProps);
  const { project } = useCurrentProject();
  const [query, setQuery] = useState('');
  const [configureId, setConfigureId] = useState<string | null>(null);
  const [detecting, setDetecting] = useState(false);

  const { available, connected } = useMemo(() => {
    const q = query.trim().toLowerCase();
    const matches = INTEGRATIONS.filter((i) => !q || i.name.toLowerCase().includes(q));
    return {
      available: matches.filter((i) => !isConnected(project, i.id)),
      connected: matches.filter((i) => isConnected(project, i.id)),
    };
  }, [query, project]);

  const handleSelect = (id: string) => {
    if (!project) {
      return;
    }
    setOpen(false);
    setConfigureId(id);
  };

  const detect = async () => {
    if (!project?.path) {
      return;
    }
    const path = project.path;
    setDetecting(true);
    try {
      const results = await Promise.all(
        INTEGRATIONS.map(async (i) => {
          if (!i.detect || isConnected(project, i.id)) {
            return null;
          }
          try {
            const config = await i.detect(path);
            return config ? ({ id: i.id, config } as const) : null;
          } catch {
            return null;
          }
        }),
      );
      const added = results.filter((r): r is { id: string; config: Record<string, unknown> } => r !== null);
      if (added.length === 0) {
        toast.message(t('projects.settings.redetectedNone'));
        return;
      }
      const tx = projectsCollection.update(project.id, (draft) => {
        let next = draft.integrations ?? {};
        for (const item of added) {
          next = withIntegration({ ...draft, integrations: next }, item.id, item.config);
        }
        draft.integrations = next;
      });
      await tx.isPersisted.promise;
      const names = added.map((a) => INTEGRATIONS.find((i) => i.id === a.id)?.name ?? a.id).join(', ');
      toast.success(t('projects.settings.redetectedAdded', { count: added.length }), { description: names });
    } catch (err) {
      toastError({ title: t('projects.settings.couldNotRedetect'), err });
    } finally {
      setDetecting(false);
    }
  };

  return (
    <>
      <Dialog open={open} onOpenChange={setOpen}>
        {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
        <DialogContent className="w-[min(95vw,640px)] max-w-[640px] gap-4">
          <DialogHeader>
            <DialogTitle>{t('integrations.add.title')}</DialogTitle>
            <DialogDescription>{t('integrations.add.description', { project: project?.name ?? t('integrations.add.thisProject') })}</DialogDescription>
          </DialogHeader>

          <div className="flex items-center gap-2">
            <div className="relative flex-1">
              <Search className="-translate-y-1/2 absolute top-1/2 left-3 size-4 text-muted-foreground" />
              <Input autoFocus value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('integrations.add.searchPlaceholder')} className="h-10 pl-9" />
            </div>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button type="button" variant="outline" size="icon" onClick={detect} disabled={detecting || !project?.path} className="size-10 shrink-0" aria-label={t('projects.settings.redetect')}>
                  <Sparkles className={cn('size-4', detecting && 'animate-pulse')} />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">{detecting ? t('projects.settings.redetecting') : t('projects.settings.redetect')}</TooltipContent>
            </Tooltip>
          </div>

          <ScrollArea className="-mr-2 max-h-[60vh] pr-2">
            {available.length === 0 && connected.length === 0 ? (
              <div className="py-12 text-center text-sm text-muted-foreground">{t('integrations.add.noMatch', { query })}</div>
            ) : (
              <div className="flex flex-col gap-4">
                {available.length > 0 && (
                  <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                    {available.map((integration) => (
                      <IntegrationCard key={integration.id} integration={integration} connected={false} onToggle={handleSelect} />
                    ))}
                  </div>
                )}
                {connected.length > 0 && (
                  <div className="flex flex-col gap-2">
                    <span className="px-1 text-muted-foreground text-xs">{t('integrations.add.connectedSection')}</span>
                    <div className="grid grid-cols-1 gap-2 opacity-60 md:grid-cols-2">
                      {connected.map((integration) => (
                        <IntegrationCard key={integration.id} integration={integration} connected={true} onToggle={handleSelect} />
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          </ScrollArea>
        </DialogContent>
      </Dialog>
      {project && configureId && <ConfigureIntegrationModal projectId={project.id} integrationId={configureId} open={true} onOpenChange={(o) => !o && setConfigureId(null)} />}
    </>
  );
}
