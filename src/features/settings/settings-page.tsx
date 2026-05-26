import { useNavigate, useSearch } from '@tanstack/react-router';
import { Settings } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/atoms/page-header';
import { ScrollArea } from '@/components/ui/scroll-area';
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

export function SettingsPage() {
  const { t } = useTranslation();
  const { section } = useSearch({ from: '/settings' });
  const navigate = useNavigate({ from: '/settings' });
  const Section = SECTIONS[section];

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 p-4">
        <PageHeader icon={<Settings className="size-5 text-muted-foreground" />} title={t('settings.title')} subtitle={t('settings.subtitle')} />
      </div>
      <div className="flex min-h-0 flex-1 gap-6 px-6 pb-6">
        <SettingsNav current={section} onSelect={(s) => navigate({ search: { section: s } })} />
        <ScrollArea className="flex-1">
          <div className="pb-6 pr-4">
            <Section />
          </div>
        </ScrollArea>
      </div>
    </div>
  );
}
