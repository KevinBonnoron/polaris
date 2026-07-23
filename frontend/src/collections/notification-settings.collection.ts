import { createCollection } from '@tanstack/db';
import { GetNotificationSettings, UpdateNotificationSettings } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';
import { wailsCollectionOptions } from '@/lib/wails-db-collection';

export type NotificationSettingsItem = polaris.NotificationSettings & { id: string };

export const SINGLETON_ID = 'settings';

export const notificationSettingsCollection = createCollection(
  wailsCollectionOptions<NotificationSettingsItem>({
    name: 'notificationSettings',
    list: async () => {
      const s = await GetNotificationSettings();
      return s ? [{ ...s, id: SINGLETON_ID }] : [];
    },
    upsert: async (item) => {
      const { id: _id, ...settings } = item;
      const saved = await UpdateNotificationSettings(settings as polaris.NotificationSettings);
      return { ...saved, id: SINGLETON_ID };
    },
  }),
);
