import { Search, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

export type Filters = {
  query: string;
  assignee: string;
};

export const ASSIGNEE_ALL = '__all__';
export const ASSIGNEE_ME = '__me__';
const ASSIGNEE_UNASSIGNED = '__none__';

interface Props {
  filters: Filters;
  assignees: string[];
  currentUserEmail?: string;
  onChange: (next: Filters) => void;
  boardSelector?: ReactNode;
  columnsMenu?: ReactNode;
}

export function BoardFilters({ filters, assignees, currentUserEmail, onChange, boardSelector, columnsMenu }: Props) {
  const { t } = useTranslation();
  const defaultAssignee = currentUserEmail ? ASSIGNEE_ME : ASSIGNEE_ALL;
  const active = filters.query !== '' || filters.assignee !== defaultAssignee;

  return (
    <div className="flex flex-wrap items-center gap-2">
      {boardSelector}
      <div className="relative min-w-0 flex-1">
        <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input value={filters.query} onChange={(e) => onChange({ ...filters, query: e.target.value })} placeholder={t('integrations.tickets.searchPlaceholder')} className="pl-9" />
      </div>
      <Select value={filters.assignee} onValueChange={(value) => onChange({ ...filters, assignee: value })}>
        <SelectTrigger className="w-48">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ASSIGNEE_ALL}>{t('integrations.tickets.allAssignees')}</SelectItem>
          {currentUserEmail && <SelectItem value={ASSIGNEE_ME}>{t('integrations.tickets.me')}</SelectItem>}
          <SelectItem value={ASSIGNEE_UNASSIGNED}>{t('integrations.tickets.unassigned')}</SelectItem>
          {assignees.map((a) => (
            <SelectItem key={a} value={a}>
              {a}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {active && (
        <Button variant="ghost" size="sm" onClick={() => onChange({ query: '', assignee: defaultAssignee })}>
          <X className="size-3.5" />
          {t('integrations.tickets.clearFilters')}
        </Button>
      )}
      {columnsMenu}
    </div>
  );
}

export function matchesFilters(issue: { summary: string; key: string; assignee: string; assigneeEmail?: string }, filters: Filters, currentUserEmail?: string): boolean {
  if (filters.assignee === ASSIGNEE_UNASSIGNED && issue.assignee) {
    return false;
  }
  if (filters.assignee === ASSIGNEE_ME) {
    if (!currentUserEmail || issue.assigneeEmail?.toLowerCase() !== currentUserEmail.toLowerCase()) {
      return false;
    }
  } else if (filters.assignee !== ASSIGNEE_ALL && filters.assignee !== ASSIGNEE_UNASSIGNED && issue.assignee !== filters.assignee) {
    return false;
  }
  const q = filters.query.trim().toLowerCase();
  if (!q) {
    return true;
  }
  return issue.summary.toLowerCase().includes(q) || issue.key.toLowerCase().includes(q);
}
