import { CircleDot } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { getIssuesEntry } from '@/db/gh-repo';
import { ListShell } from './list-shell';
import { RepoListFilters } from './repo-list-filters';
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
  const { items, loading, initial, error, reload, currentUser, defaultAuthor, effectiveAuthor, query, setQuery, setAuthor, authors } = useGhListTab(entry);
  useRegisterReload(onRegister, { reload, loading });

  const filters = <RepoListFilters query={query} onQueryChange={setQuery} selectValue={effectiveAuthor} onSelectChange={setAuthor} selectOptions={authors} currentUser={currentUser} defaultSelectValue={defaultAuthor} searchPlaceholderKey="integrations.repository.searchIssues" />;

  return (
    <ListShell title={t('integrations.repository.openIssues')} initial={initial} error={error} empty={items.length === 0} emptyText={t('integrations.repository.noIssues')} filters={filters}>
      {items.map((issue) => (
        <Row key={issue.number} icon={<CircleDot className="size-4 text-emerald-500" />} title={issue.title} subtitle={t('integrations.repository.prSubtitle', { number: issue.number, author: issue.author, when: formatAgo(issue.updatedAt, i18n.language) })} labels={issue.labels} url={issue.url} />
      ))}
    </ListShell>
  );
}
