import { useCallback, useEffect, useState } from 'react';
import { GetDokployDashboard } from '@/wailsjs/go/main/App';
import type { dokploy } from '@/wailsjs/go/models';
import { dokploy as dokployModel } from '@/wailsjs/go/models';
import type { ConnectedDokployConfig } from './types';

export type DokployDeployment = dokploy.Deployment;
export type DokployService = dokploy.Service;

export function useDokployDashboard(config: ConnectedDokployConfig): {
  services: DokployService[];
  deployments: DokployDeployment[];
  loading: boolean;
  error: string | null;
  reload: () => void;
} {
  const [services, setServices] = useState<DokployService[]>([]);
  const [deployments, setDeployments] = useState<DokployDeployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const apiCfg = dokployModel.Config.createFrom({ baseUrl: config.baseUrl, apiKey: config.apiKey });
      const dashboard = await GetDokployDashboard(apiCfg, config.projects);
      setServices(dashboard?.services ?? []);
      setDeployments(dashboard?.deployments ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [config.baseUrl, config.apiKey, config.projects]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { services, deployments, loading, error, reload: () => void reload() };
}
