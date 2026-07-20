import { Search, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

export const FILTER_ALL = '__all__';
export const FILTER_ME = '__me__';

interface Props {
  query: string;
  onQueryChange: (value: string) => void;
  selectValue: string;
  selectOptions: string[];
  selectPlaceholder: string;
  allOptionLabel: string;
  searchPlaceholder: string;
  onSelectChange: (value: string) => void;
  currentUser?: string | null;
  meLabel?: string;
  defaultSelectValue?: string;
  selectSlot?: ReactNode;
}

export function ListFilters({ query, onQueryChange, selectValue, selectOptions, selectPlaceholder, allOptionLabel, searchPlaceholder, onSelectChange, currentUser, meLabel, defaultSelectValue = FILTER_ALL, selectSlot }: Props) {
  const { t } = useTranslation();
  const active = query !== '' || selectValue !== defaultSelectValue;
  const filteredOptions = currentUser ? selectOptions.filter((opt) => opt !== currentUser) : selectOptions;

  return (
    <div className="flex flex-wrap items-center gap-2 pb-3">
      <div className="relative min-w-0 flex-1">
        <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input value={query} onChange={(e) => onQueryChange(e.target.value)} placeholder={searchPlaceholder} className="pl-9" />
      </div>
      {selectSlot ?? (
        <Select value={selectValue} onValueChange={onSelectChange}>
          <SelectTrigger className="w-48">
            <SelectValue placeholder={selectPlaceholder} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={FILTER_ALL}>{allOptionLabel}</SelectItem>
            {currentUser && meLabel && <SelectItem value={FILTER_ME}>{meLabel}</SelectItem>}
            {filteredOptions.map((opt) => (
              <SelectItem key={opt} value={opt}>
                {opt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {active && (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            onQueryChange('');
            onSelectChange(defaultSelectValue);
          }}
        >
          <X className="size-3.5" />
          {t('integrations.repository.clearFilters')}
        </Button>
      )}
    </div>
  );
}
