import { useLiveQuery } from '@tanstack/react-db';
import { useEffect, useState } from 'react';
import type { GhEntry } from '@/collections/github.repository.collection';
import { humanizeError } from './utils';

export type GhView<T extends object> = {
  data: T[];
  loading: boolean;
  initial: boolean;
  error: string | null;
  hasMore: boolean;
  reload: () => void;
  loadMore: () => void;
};

export function useGhEntry<T extends object>(entry: GhEntry<T>): GhView<T> {
  const [, setTick] = useState(0);
  useEffect(() => entry.subscribe(() => setTick((t) => t + 1)), [entry]);

  const { data = [], isReady } = useLiveQuery((q) => q.from({ x: entry.collection }), [entry]);

  return {
    data,
    loading: entry.loading,
    initial: !isReady,
    error: entry.error ? humanizeError(entry.error) : null,
    hasMore: entry.hasMore,
    reload: () => {
      void entry.reload();
    },
    loadMore: () => {
      void entry.loadMore();
    },
  };
}
