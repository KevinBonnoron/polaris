import { CircleDot } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FilterBar, type FilterToken } from '@/components/atoms/filter-bar';
import { getIssuesEntry } from '@/collections/github.repository.collection';
import { FILTER_ME } from './list-filters';
import { ListShell } from './list-shell';
import { Row } from './row';
import { useGhListTab } from './use-gh-list-tab';
import { type ReloadRegister, useRegisterReload } from './use-register-reload';
import { formatAgo } from './utils';

interface Props {
  owner: string;
  repo: string;
  onRegister?: ReloadRegister;
}

export function IssuesTab({ owner, repo, onRegister }: Props) {
  const { t, i18n } = useTranslation();
  const entry = useMemo(() => getIssuesEntry(owner, repo), [owner, repo]);
  const { items, loading, initial, error, reload, currentUser, defaultAuthor, setQuery, setAuthor, authors } = useGhListTab(entry);
  useRegisterReload(onRegister, { reload, loading });

  const [tokens, setTokens] = useState<FilterToken[]>([]);

  useEffect(() => {
    setQuery(tokens.find((t) => t.key === 'term')?.value ?? '');
    const authorToken = tokens.find((t) => t.key === 'author');
    setAuthor(authorToken ? (authorToken.value === 'me' ? FILTER_ME : authorToken.value) : null);
  }, [tokens, setQuery, setAuthor]);

  const [initialized, setInitialized] = useState(false);
  useEffect(() => {
    if (!initialized && currentUser !== undefined) {
      if (defaultAuthor === FILTER_ME) setTokens((prev) => (prev.length === 0 ? [{ key: 'author', value: 'me' }] : prev));
      setInitialized(true);
    }
  }, [currentUser, defaultAuthor, initialized]);

  const authorOptions = useMemo(() => [{ value: 'me', label: currentUser ? t('integrations.repository.meWithUser', { user: currentUser }) : t('integrations.repository.me') }, ...authors.filter((a) => a !== currentUser).map((a) => ({ value: a, label: a }))], [authors, currentUser, t]);

  const labelFilters = tokens.filter((t) => t.key === 'label').map((t) => t.value);
  const assigneeFilters = tokens.filter((t) => t.key === 'assignee').map((t) => t.value);

  const labelOptions = useMemo(() => {
    const reachable = items.filter((i) => labelFilters.every((lf) => (i.labels ?? []).some((l) => l.name === lf)));
    const names = Array.from(new Set(reachable.flatMap((i) => (i.labels ?? []).map((l) => l.name))))
      .filter((n) => !labelFilters.includes(n))
      .sort();
    return names.map((name) => ({ value: name, label: name }));
  }, [items, labelFilters]);

  const assigneeOptions = useMemo(() => {
    const reachable = items.filter((i) => assigneeFilters.every((af) => (i.assignees ?? []).includes(af)));
    const all = Array.from(new Set(reachable.flatMap((i) => i.assignees ?? [])))
      .filter((a) => !assigneeFilters.includes(a))
      .sort();
    return all.map((a) => ({ value: a, label: a }));
  }, [items, assigneeFilters]);

  const defs = useMemo(
    () => [
      { key: 'author', label: t('integrations.repository.author'), options: authorOptions },
      { key: 'assignee', label: t('integrations.repository.assignee'), multi: true, options: assigneeOptions },
      { key: 'label', label: t('integrations.repository.label'), multi: true, options: labelOptions },
    ],
    [authorOptions, assigneeOptions, labelOptions, t],
  );

  const filteredItems = useMemo(
    () =>
      items.filter((issue) => {
        if (labelFilters.length > 0 && !labelFilters.every((lf) => (issue.labels ?? []).some((l) => l.name === lf))) return false;
        if (assigneeFilters.length > 0 && !assigneeFilters.every((af) => (issue.assignees ?? []).includes(af))) return false;
        return true;
      }),
    [items, labelFilters, assigneeFilters],
  );

  const filters = <FilterBar tokens={tokens} onTokensChange={setTokens} defs={defs} placeholder={t('integrations.repository.searchIssues')} />;

  return (
    <ListShell title={t('integrations.repository.openIssues')} initial={initial} error={error} empty={filteredItems.length === 0} emptyText={t('integrations.repository.noIssues')} filters={filters}>
      {filteredItems.map((issue) => (
        <Row key={issue.number} icon={<CircleDot className="size-4 text-emerald-500" />} title={issue.title} subtitle={t('integrations.repository.prSubtitle', { number: issue.number, author: issue.author, when: formatAgo(issue.updatedAt, i18n.language) })} labels={issue.labels} url={issue.url} />
      ))}
    </ListShell>
  );
}
