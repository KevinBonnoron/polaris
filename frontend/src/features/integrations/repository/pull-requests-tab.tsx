import { GitPullRequest } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FilterBar, type FilterToken } from '@/components/atoms/filter-bar';
import { getPullRequestsEntry } from '@/collections/github.repository.collection';
import { cn } from '@/lib/utils';
import { FILTER_ME } from './list-filters';
import { ListShell } from './list-shell';
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

  const labelOptions = useMemo(() => {
    const reachable = items.filter((pr) => labelFilters.every((lf) => (pr.labels ?? []).some((l) => l.name === lf)));
    const names = Array.from(new Set(reachable.flatMap((pr) => (pr.labels ?? []).map((l) => l.name))))
      .filter((n) => !labelFilters.includes(n))
      .sort();
    return names.map((name) => ({ value: name, label: name }));
  }, [items, labelFilters]);

  const draftOptions = useMemo(
    () => [
      { value: 'true', label: t('integrations.repository.draft') },
      { value: 'false', label: t('integrations.repository.readyForReview') },
    ],
    [t],
  );

  const reviewOptions = useMemo(
    () => [
      { value: 'APPROVED', label: t('integrations.repository.reviewApproved') },
      { value: 'CHANGES_REQUESTED', label: t('integrations.repository.reviewChangesRequested') },
      { value: 'REVIEW_REQUIRED', label: t('integrations.repository.reviewRequired') },
    ],
    [t],
  );

  const defs = useMemo(
    () => [
      { key: 'author', label: t('integrations.repository.author'), options: authorOptions },
      { key: 'label', label: t('integrations.repository.label'), multi: true, options: labelOptions },
      { key: 'draft', label: t('integrations.repository.draftFilter'), options: draftOptions },
      { key: 'review', label: t('integrations.repository.reviewStatus'), options: reviewOptions },
    ],
    [authorOptions, labelOptions, draftOptions, reviewOptions, t],
  );

  const draftFilter = tokens.find((t) => t.key === 'draft')?.value;
  const reviewFilter = tokens.find((t) => t.key === 'review')?.value;

  const filteredItems = useMemo(
    () =>
      items.filter((pr) => {
        if (labelFilters.length > 0 && !labelFilters.every((lf) => (pr.labels ?? []).some((l) => l.name === lf))) return false;
        if (draftFilter !== undefined && pr.draft !== (draftFilter === 'true')) return false;
        if (reviewFilter && pr.reviewDecision !== reviewFilter) return false;
        return true;
      }),
    [items, labelFilters, draftFilter, reviewFilter],
  );

  const filters = <FilterBar tokens={tokens} onTokensChange={setTokens} defs={defs} placeholder={t('integrations.repository.searchPullRequests')} />;

  return (
    <ListShell title={t('integrations.repository.openPullRequests')} initial={initial} error={error} empty={filteredItems.length === 0} emptyText={t('integrations.repository.noPullRequests')} filters={filters}>
      {filteredItems.map((pr) => (
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
