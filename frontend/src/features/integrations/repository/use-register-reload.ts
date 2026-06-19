import { useEffect, useRef } from 'react';

export interface ReloadApi {
  reload: () => void;
  loading: boolean;
}

export type ReloadRegister = (api: ReloadApi | null) => void;

export function useRegisterReload(register: ReloadRegister | undefined, api: ReloadApi) {
  const { reload, loading } = api;
  const reloadRef = useRef(reload);
  reloadRef.current = reload;

  useEffect(() => {
    if (!register) {
      return;
    }
    register({ reload: () => reloadRef.current(), loading });
    return () => register(null);
  }, [register, loading]);
}
