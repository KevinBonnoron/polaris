import { Loader2, MoreVertical, Play, RotateCw, Square } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { toastError } from '@/lib/toast-error';
import { RunDokployAction } from '@/wailsjs/go/main/App';
import { dokploy as dokployModel } from '@/wailsjs/go/models';
import type { ConnectedDokployConfig } from './types';
import type { DokployService } from './use-dokploy-dashboard';

type Action = 'redeploy' | 'start' | 'stop';

const ACTIONS: { action: Action; icon: typeof Play }[] = [
  { action: 'redeploy', icon: RotateCw },
  { action: 'start', icon: Play },
  { action: 'stop', icon: Square },
];

export function ServiceActions({ config, service, onDone }: { config: ConnectedDokployConfig; service: DokployService; onDone: () => void }) {
  const { t } = useTranslation();
  const [pending, setPending] = useState<Action | null>(null);
  const [running, setRunning] = useState(false);

  const run = async (action: Action) => {
    setRunning(true);
    try {
      await RunDokployAction(dokployModel.Config.createFrom({ baseUrl: config.baseUrl, apiKey: config.apiKey }), service, action);
      toast.success(t('integrations.dokploy.actionStarted', { action: t(`integrations.dokploy.actions.${action}`), service: service.name }));
      onDone();
    } catch (err) {
      toastError({ title: t('integrations.dokploy.actionFailed', { service: service.name }), err });
    } finally {
      setRunning(false);
      setPending(null);
    }
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" className="size-7 shrink-0" aria-label={t('integrations.dokploy.actions.menu')}>
            {running ? <Loader2 className="size-3.5 animate-spin" /> : <MoreVertical className="size-3.5" />}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {ACTIONS.map(({ action, icon: Icon }) => (
            <DropdownMenuItem key={action} onSelect={() => setPending(action)} variant={action === 'stop' ? 'destructive' : 'default'}>
              <Icon className="size-3.5" />
              {t(`integrations.dokploy.actions.${action}`)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={pending !== null} onOpenChange={(o) => !o && setPending(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{pending && t('integrations.dokploy.confirm.title', { action: t(`integrations.dokploy.actions.${pending}`), service: service.name })}</DialogTitle>
            <DialogDescription>{t('integrations.dokploy.confirm.body')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPending(null)} disabled={running}>
              {t('common.cancel')}
            </Button>
            <Button variant={pending === 'stop' ? 'destructive' : 'default'} onClick={() => pending && run(pending)} disabled={running}>
              {running && <Loader2 className="size-3.5 animate-spin" />}
              {pending && t(`integrations.dokploy.actions.${pending}`)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
