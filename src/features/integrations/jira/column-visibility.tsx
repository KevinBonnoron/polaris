import { Columns3 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';

interface Props {
  columns: string[];
  hidden: Set<string>;
  onToggle: (name: string) => void;
}

export function ColumnVisibilityMenu({ columns, hidden, onToggle }: Props) {
  const { t } = useTranslation();
  const visibleCount = columns.length - hidden.size;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm">
          <Columns3 className="size-3.5" />
          {t('integrations.jira.columns', { visible: visibleCount, total: columns.length })}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        {columns.map((name) => (
          <DropdownMenuCheckboxItem key={name} checked={!hidden.has(name)} onCheckedChange={() => onToggle(name)} onSelect={(e) => e.preventDefault()}>
            {name}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
