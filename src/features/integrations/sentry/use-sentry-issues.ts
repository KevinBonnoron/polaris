import { useLiveQuery } from '@tanstack/react-db';
import { useEffect, useState } from 'react';
import { getSentryEntry, type SentryIssue } from '@/collections/sentry.issues.collection';
import type { ConnectedSentryConfig } from './types';

export type { SentryIssue };

export function useSentryIssues(config: ConnectedSentryConfig): {
  issues: SentryIssue[];
  loading: boolean;
  error: string | null;
  reload: () => void;
} {
  const entry = getSentryEntry(config);

  const [, setTick] = useState(0);
  useEffect(() => entry.subscribe(() => setTick((t) => t + 1)), [entry]);

  const { data: issues } = useLiveQuery((q) => q.from({ i: entry.collection }), [entry]);

  return {
    issues: (issues ?? []) as SentryIssue[],
    loading: entry.loading,
    error: entry.error,
    reload: () => {
      void entry.reload();
    },
  };
}
