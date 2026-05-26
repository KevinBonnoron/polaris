import type { PropsWithChildren } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { useCurrentProject } from '@/state/projects';
import { IntegrationBrowser } from './integration-browser';

export function AddIntegrationModal({ children, ...modeProps }: PropsWithChildren<DialogModeProps>) {
  const { t } = useTranslation();
  const { open, setOpen } = useDialogMode(modeProps);
  const { project } = useCurrentProject();

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent className="w-[min(95vw,640px)] max-w-[640px] gap-4">
        <DialogHeader>
          <DialogTitle>{t('integrations.add.title')}</DialogTitle>
          <DialogDescription>{t('integrations.add.description', { project: project?.name ?? t('integrations.add.thisProject') })}</DialogDescription>
        </DialogHeader>
        {project && <IntegrationBrowser project={project} />}
      </DialogContent>
    </Dialog>
  );
}
