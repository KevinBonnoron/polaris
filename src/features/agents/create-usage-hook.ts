import { useCallback, useEffect, useState } from 'react';

export function createUsageHook<T>(fetchFn: (force: boolean) => Promise<T>) {
  let cache: T | null = null;

  return function useUsage() {
    const [usage, setUsage] = useState<T | null>(cache);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const refresh = useCallback(async (force = false) => {
      setLoading(true);
      setError(null);
      try {
        const data = await fetchFn(force);
        const fetchError = (data as { error?: string }).error;
        if (fetchError) {
          setError(fetchError);
        } else {
          cache = data;
          setUsage(data);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    }, []);

    useEffect(() => {
      void refresh(false);
    }, [refresh]);

    return { usage, loading, error, refresh };
  };
}
