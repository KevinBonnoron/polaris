import { GitPullRequest } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { getPullRequestsEntry } from '@/db/gh-repo';
import { cn } from '@/lib/utils';
import { ListShell } from './list-shell';
import { RepoListFilters } from './repo-list-filters';
import { ReviewStatusBadge } from './review-status-badge';
import { Row } from './row';
import { useGhListTab } from './use-gh-list-tab';
import { type ReloadRegister, useRegisterReload } from './use-register-reload';
import { formatAgo } from './utils';

interface Props {
  owner: string;
  repo: string;
  onRegister?: ReloadRegister;
}

export function PullRequestsTab({ owner, repo, onRegister }: Props) {
  const { t, i18n } = useTranslation();
  const entry = useMemo(() => getPullRequestsEntry(owner, repo), [owner, repo]);
  const { items, loading, initial, error, reload, currentUser, defaultAuthor, effectiveAuthor, query, setQuery, setAuthor, authors } = useGhListTab(entry);
  useRegisterReload(onRegister, { reload, loading });

  const filters = <RepoListFilters query={query} onQueryChange={setQuery} selectValue={effectiveAuthor} onSelectChange={setAuthor} selectOptions={authors} currentUser={currentUser} defaultSelectValue={defaultAuthor} searchPlaceholderKey="integrations.repository.searchPullRequests" />;

  return (
    <ListShell title={t('integrations.repository.openPullRequests')} initial={initial} error={error} empty={items.length === 0} emptyText={t('integrations.repository.noPullRequests')} filters={filters}>
      {items.map((pr) => (
        <Row
          key={pr.number}
          icon={<GitPullRequest className={cn('size-4', pr.draft ? 'text-muted-foreground' : 'text-emerald-500')} />}
          title={pr.title}
          subtitle={t('integrations.repository.prSubtitle', { number: pr.number, author: pr.author, when: formatAgo(pr.updatedAt, i18n.language) })}
          labels={pr.labels}
          url={pr.url}
          meta={<ReviewStatusBadge decision={pr.reviewDecision} />}
        />
      ))}
    </ListShell>
  );
}
