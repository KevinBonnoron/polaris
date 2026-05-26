import { createCollection } from '@tanstack/db';
import { DeleteNotification, ListNotifications, UpsertNotification } from '@/wailsjs/go/main/App';
import { wailsCollectionOptions } from '../lib/wails-db-collection';
import type { Notification } from '../types';

export const notificationsCollection = createCollection(
  wailsCollectionOptions<Notification>({
    name: 'notifications',
    list: () => ListNotifications() as unknown as Promise<Notification[]>,
    upsert: (n) => UpsertNotification(n as never) as unknown as Promise<Notification>,
    remove: (id) => DeleteNotification(id),
  }),
);
