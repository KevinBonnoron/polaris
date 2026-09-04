import { useLiveQuery } from '@tanstack/react-db';
import { useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { notificationsCollection } from '@/collections/notifications.collection';

export function NotificationToast() {
  const { data: notifications = [], isReady } = useLiveQuery((q) => q.from({ n: notificationsCollection }));
  const seenIdsRef = useRef<Set<string> | null>(null);

  useEffect(() => {
    if (!isReady) return;

    if (seenIdsRef.current === null) {
      // First completed result — mark all existing notifications as seen
      // without toasting, including when the initial snapshot is empty.
      seenIdsRef.current = new Set(notifications.map((n) => n.id));
      return;
    }

    for (const n of notifications) {
      if (!seenIdsRef.current.has(n.id)) {
        seenIdsRef.current.add(n.id);
        if (n.severity === 'error') {
          toast.error(n.title, { duration: 6000 });
        }
      }
    }
  }, [notifications, isReady]);

  return null;
}
