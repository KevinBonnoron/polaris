import { useMemo, useState } from 'react';
import { FILTER_ALL, FILTER_ME } from './list-filters';
import { useGhCurrentUser } from './use-gh-current-user';
import { useGhEntry } from './use-gh-entry';

interface ListItem {
  number: number;
  title: string;
  author: string;
  updatedAt: number;
}

type EntryParam<T extends ListItem> = Parameters<typeof useGhEntry<T>>[0];

export function useGhListTab<T extends ListItem>(entry: EntryParam<T>) {
  const { data, loading, initial, error, reload } = useGhEntry(entry);
  const currentUser = useGhCurrentUser();
  const defaultAuthor = currentUser ? FILTER_ME : FILTER_ALL;
  const [query, setQuery] = useState('');
  const [author, setAuthor] = useState<string | null>(null);
  const effectiveAuthor = author ?? defaultAuthor;

  const sorted = useMemo(() => [...data].sort((a, b) => b.updatedAt - a.updatedAt), [data]);
  const authors = useMemo(() => Array.from(new Set(sorted.map((i) => i.author).filter(Boolean))).sort(), [sorted]);
  const items = useMemo(() => {
    const q = query.trim().toLowerCase();
    return sorted.filter((it) => {
      if (effectiveAuthor === FILTER_ME) {
        if (!currentUser || it.author !== currentUser) {
          return false;
        }
      } else if (effectiveAuthor !== FILTER_ALL && it.author !== effectiveAuthor) {
        return false;
      }
      if (!q) {
        return true;
      }
      return it.title.toLowerCase().includes(q) || String(it.number).includes(q);
    });
  }, [sorted, query, effectiveAuthor, currentUser]);

  return { items, data, loading, initial, error, reload, currentUser, defaultAuthor, effectiveAuthor, query, setQuery, author, setAuthor, authors };
}
