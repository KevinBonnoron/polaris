import { type Collection, createCollection } from '@tanstack/db';
import { DockerCapabilities, ListDockerBaseImages, ParseDockerfile } from '@/wailsjs/go/main/App';
import type { docker } from '@/wailsjs/go/models';

type Listener = () => void;

export type DockerEntry = {
  collection: Collection<docker.Image, string>;
  parsed: docker.Dockerfile | null;
  caps: docker.Capabilities | null;
  loading: boolean;
  error: string | null;
  reload: () => Promise<void>;
  subscribe: (l: Listener) => () => void;
};

export const imageRef = (img: docker.Image): string => `${img.repository}:${img.tag}`;

const entries = new Map<string, DockerEntry>();

function createDockerEntry(dockerfilePath: string): DockerEntry {
  const listeners = new Set<Listener>();
  const notify = () => {
    for (const l of listeners) {
      l();
    }
  };
  const current = new Map<string, docker.Image>();

  let syncBegin: (() => void) | null = null;
  let syncWrite: ((op: { type: 'insert' | 'update' | 'delete'; value: docker.Image }) => void) | null = null;
  let syncCommit: (() => void) | null = null;

  const entry: DockerEntry = {
    collection: null as unknown as Collection<docker.Image, string>,
    parsed: null,
    caps: null,
    loading: false,
    error: null,
    reload: async () => {},
    subscribe: (l) => {
      listeners.add(l);
      return () => {
        listeners.delete(l);
      };
    },
  };

  const applyImages = (next: docker.Image[]) => {
    if (!syncBegin || !syncWrite || !syncCommit) {
      return;
    }
    syncBegin();
    const seen = new Set<string>();
    for (const item of next) {
      const k = imageRef(item);
      seen.add(k);
      const prev = current.get(k);
      if (!prev) {
        syncWrite({ type: 'insert', value: item });
        current.set(k, item);
      } else if (JSON.stringify(prev) !== JSON.stringify(item)) {
        syncWrite({ type: 'update', value: item });
        current.set(k, item);
      }
    }
    for (const [k, v] of Array.from(current)) {
      if (!seen.has(k)) {
        syncWrite({ type: 'delete', value: v });
        current.delete(k);
      }
    }
    syncCommit();
  };

  const refresh = async () => {
    entry.loading = true;
    entry.error = null;
    notify();
    try {
      const [caps, parsed] = await Promise.all([DockerCapabilities(), ParseDockerfile(dockerfilePath)]);
      entry.caps = caps;
      entry.parsed = parsed;
      const images = caps.dockerDaemon ? ((await ListDockerBaseImages(dockerfilePath)) ?? []) : [];
      applyImages(images as docker.Image[]);
    } catch (err) {
      entry.error = err instanceof Error ? err.message : String(err);
    } finally {
      entry.loading = false;
      notify();
    }
  };
  entry.reload = refresh;

  entry.collection = createCollection({
    getKey: imageRef,
    sync: {
      sync: ({ begin, write, commit, markReady }) => {
        syncBegin = begin;
        syncWrite = write as typeof syncWrite;
        syncCommit = commit;
        current.clear();
        refresh().finally(() => markReady());
        return () => {
          syncBegin = null;
          syncWrite = null;
          syncCommit = null;
        };
      },
    },
  }) as unknown as Collection<docker.Image, string>;

  return entry;
}

export function getDockerEntry(dockerfilePath: string): DockerEntry {
  const cached = entries.get(dockerfilePath);
  if (cached) {
    return cached;
  }
  const entry = createDockerEntry(dockerfilePath);
  entries.set(dockerfilePath, entry);
  return entry;
}
