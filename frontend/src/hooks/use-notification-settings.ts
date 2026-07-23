import { useLiveQuery } from '@tanstack/react-db';
import { useCallback } from 'react';
import { SINGLETON_ID, type NotificationSettingsItem, notificationSettingsCollection } from '@/collections/notification-settings.collection';

export interface NotificationEventFlags {
  waiting: boolean;
  completed: boolean;
  errored: boolean;
  started: boolean;
  automation: boolean;
  user: boolean;
}

export interface NotificationSettings {
  osEnabled: boolean;
  sound: string;
  silenceAll: boolean;
  events: NotificationEventFlags;
}

const DEFAULTS: NotificationSettings = {
  osEnabled: true,
  sound: 'default',
  silenceAll: false,
  events: { waiting: true, completed: true, errored: true, started: false, automation: true, user: true },
};

export function useNotificationSettings() {
  const { data, isReady } = useLiveQuery((q) => q.from({ s: notificationSettingsCollection }));
  const settings = (data[0] ?? DEFAULTS) as NotificationSettings;

  const update = useCallback(
    (patch: Partial<Omit<NotificationSettings, 'events'>> & { events?: Partial<NotificationEventFlags> }) => {
      if (!isReady) return;
      const { events, ...settingsPatch } = patch;
      if (data[0]) {
        notificationSettingsCollection.update(SINGLETON_ID, (draft: NotificationSettingsItem) => {
          Object.assign(draft, settingsPatch);
          if (events) {
            draft.events = { ...draft.events, ...events };
          }
        });
      } else {
        notificationSettingsCollection.insert({
          ...DEFAULTS,
          ...settingsPatch,
          events: { ...DEFAULTS.events, ...(events ?? {}) },
          id: SINGLETON_ID,
        });
      }
    },
    [isReady, data],
  );

  return { settings, loaded: isReady, update };
}
