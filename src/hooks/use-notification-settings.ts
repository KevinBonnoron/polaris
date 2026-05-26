import { useCallback, useEffect, useState } from 'react';
import { GetNotificationSettings, UpdateNotificationSettings } from '@/wailsjs/go/main/App';
import { EventsOff, EventsOn } from '@/wailsjs/runtime/runtime';

export interface NotificationEventFlags {
  waiting: boolean;
  completed: boolean;
  errored: boolean;
  started: boolean;
  automation: boolean;
  user: boolean;
}

interface NotificationSettings {
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

const CHANGE_EVENT = 'collection:notificationSettings:changed';

export function useNotificationSettings() {
  const [settings, setSettings] = useState<NotificationSettings>(DEFAULTS);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const refresh = () => {
      GetNotificationSettings()
        .then((value) => {
          if (cancelled || !value) {
            return;
          }
          setSettings(value as NotificationSettings);
          setLoaded(true);
        })
        .catch(() => {
          setLoaded(true);
        });
    };
    refresh();
    EventsOn(CHANGE_EVENT, refresh);
    return () => {
      cancelled = true;
      EventsOff(CHANGE_EVENT);
    };
  }, []);

  const update = useCallback(
    async (patch: Partial<Omit<NotificationSettings, 'events'>> & { events?: Partial<NotificationEventFlags> }) => {
      const next: NotificationSettings = {
        ...settings,
        ...patch,
        events: { ...settings.events, ...(patch.events ?? {}) },
      };
      setSettings(next);
      try {
        const saved = await UpdateNotificationSettings(next as never);
        if (saved) {
          setSettings(saved as NotificationSettings);
        }
      } catch (err) {
        console.error('updateNotificationSettings failed', err);
      }
    },
    [settings],
  );

  return { settings, loaded, update };
}
