import { useTranslation } from 'react-i18next';
import { ListFilters } from './list-filters';

interface Props {
  query: string;
  onQueryChange: (v: string) => void;
  selectValue: string;
  onSelectChange: (v: string) => void;
  selectOptions: string[];
  currentUser: string | null | undefined;
  defaultSelectValue: string;
  searchPlaceholderKey: 'integrations.repository.searchIssues' | 'integrations.repository.searchPullRequests';
}

export function RepoListFilters({ query, onQueryChange, selectValue, onSelectChange, selectOptions, currentUser, defaultSelectValue, searchPlaceholderKey }: Props) {
  const { t } = useTranslation();
  return (
    <ListFilters
      query={query}
      onQueryChange={onQueryChange}
      selectValue={selectValue}
      onSelectChange={onSelectChange}
      selectOptions={selectOptions}
      selectPlaceholder={t('integrations.repository.author')}
      allOptionLabel={t('integrations.repository.allAuthors')}
      searchPlaceholder={t(searchPlaceholderKey)}
      currentUser={currentUser}
      meLabel={t('integrations.repository.me')}
      defaultSelectValue={defaultSelectValue}
    />
  );
}
