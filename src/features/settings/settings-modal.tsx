import { type PropsWithChildren, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { ScrollArea } from '@/components/ui/scroll-area';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { AboutSettings } from './about-settings';
import { AccountSettings } from './account-settings';
import { AgentsSettings } from './agents-settings';
import { AppearanceSettings } from './appearance-settings';
import { GeneralSettings } from './general-settings';
import { NotificationsSettings } from './notifications-settings';
import { SettingsNav, type SettingsSection } from './settings-nav';
import { ShortcutsSettings } from './shortcuts-settings';

const SECTIONS: Record<SettingsSection, React.ComponentType> = {
  general: GeneralSettings,
  appearance: AppearanceSettings,
  agents: AgentsSettings,
  notifications: NotificationsSettings,
  shortcuts: ShortcutsSettings,
  account: AccountSettings,
  about: AboutSettings,
};

export function SettingsModal({ children, ...modeProps }: PropsWithChildren<DialogModeProps>) {
  const { t } = useTranslation();
  const { open, setOpen } = useDialogMode(modeProps);
  const [section, setSection] = useState<SettingsSection>('general');
  const Section = SECTIONS[section];

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent className="w-[min(95vw,1200px)] sm:max-w-[1200px] h-[min(90vh,820px)] grid-rows-[auto_1fr] gap-4 p-6">
        <DialogHeader>
          <DialogTitle>{t('settings.title')}</DialogTitle>
        </DialogHeader>
        <div className="flex min-h-0 gap-6">
          <SettingsNav current={section} onSelect={setSection} />
          <ScrollArea className="-mr-4 min-h-0 flex-1 pr-6">
            <Section />
          </ScrollArea>
        </div>
      </DialogContent>
    </Dialog>
  );
}
