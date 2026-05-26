import { useLiveQuery } from '@tanstack/react-db';
import { useEffect, useMemo, useState } from 'react';
import { getDockerEntry } from '@/collections/docker.collection';

export function useDockerEntry(dockerfilePath: string) {
  const entry = useMemo(() => getDockerEntry(dockerfilePath), [dockerfilePath]);

  const [, setTick] = useState(0);
  useEffect(() => entry.subscribe(() => setTick((t) => t + 1)), [entry]);

  const { data = [], isReady } = useLiveQuery((q) => q.from({ x: entry.collection }), [entry]);

  return {
    images: data,
    parsed: entry.parsed,
    caps: entry.caps,
    loading: entry.loading,
    initial: !isReady,
    error: entry.error,
    reload: () => {
      void entry.reload();
    },
  };
}
