import { useEffect, useState } from 'react';
import { GetRepositoryCurrentUser } from '@/wailsjs/go/main/App';

let cached: string | null | undefined;
let inflight: Promise<string | null> | null = null;

function fetchCurrentUser(): Promise<string | null> {
  if (cached !== undefined) {
    return Promise.resolve(cached);
  }
  if (inflight) {
    return inflight;
  }
  inflight = GetRepositoryCurrentUser()
    .then((login) => {
      cached = login || null;
      return cached;
    })
    .catch(() => {
      cached = null;
      return null;
    })
    .finally(() => {
      inflight = null;
    }) as Promise<string | null>;
  return inflight;
}

export function useGhCurrentUser(): string | null {
  const [user, setUser] = useState<string | null>(cached ?? null);
  useEffect(() => {
    if (cached !== undefined) {
      return;
    }
    let active = true;
    fetchCurrentUser().then((login) => {
      if (active) {
        setUser(login);
      }
    });
    return () => {
      active = false;
    };
  }, []);
  return user;
}
