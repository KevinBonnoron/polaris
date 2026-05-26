import { useTranslation } from 'react-i18next';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { type NotificationEventFlags, useNotificationSettings } from '@/hooks/use-notification-settings';
import { SettingsRow } from './settings-row';

const EVENT_TOGGLES: (keyof NotificationEventFlags)[] = ['waiting', 'completed', 'errored', 'started', 'automation', 'user'];

const SOUNDS = [
  { value: 'default', label: 'Default' },
  { value: 'none', label: 'None' },
];

export function NotificationsSettings() {
  const { t } = useTranslation();
  const { settings, update } = useNotificationSettings();

  return (
    <section className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <h3 className="text-base font-semibold">{t('settings.notifications.title')}</h3>
        <p className="text-xs text-muted-foreground">{t('settings.notifications.subtitle')}</p>
      </div>

      <div>
        <h4 className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.notifications.eventTypes')}</h4>
        <div className="rounded-md border border-border">
          {EVENT_TOGGLES.map((id) => (
            <SettingsRow key={id} label={t(`settings.notifications.${id}` as const)} description={t(`settings.notifications.${id}Desc` as const)} control={<Switch checked={settings.events[id]} onCheckedChange={(checked) => update({ events: { [id]: checked } })} />} className="px-3" />
          ))}
        </div>
      </div>

      <div>
        <h4 className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.notifications.delivery')}</h4>
        <div className="rounded-md border border-border">
          <SettingsRow label={t('settings.notifications.osNotifications')} description={t('settings.notifications.osNotificationsDesc')} control={<Switch checked={settings.osEnabled} onCheckedChange={(checked) => update({ osEnabled: checked })} />} className="px-3" />
          <SettingsRow
            label={t('settings.notifications.sound')}
            description={t('settings.notifications.soundDesc')}
            control={
              <Select value={settings.sound} onValueChange={(value) => update({ sound: value })}>
                <SelectTrigger size="sm" className="w-[180px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SOUNDS.map((s) => (
                    <SelectItem key={s.value} value={s.value}>
                      {s.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            }
            className="px-3"
          />
          <SettingsRow label={t('settings.notifications.silence')} description={t('settings.notifications.silenceDesc')} control={<Switch checked={settings.silenceAll} onCheckedChange={(checked) => update({ silenceAll: checked })} />} className="px-3" />
        </div>
      </div>
    </section>
  );
}
