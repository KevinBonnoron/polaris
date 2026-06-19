import { Check, ChevronsUpDown, GitBranch } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { getBranchesEntry } from '@/collections/github.repository.collection';
import { cn } from '@/lib/utils';
import { useGhEntry } from './use-gh-entry';

interface Props {
  owner: string;
  repo: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
}

export function BranchCombobox({ owner, repo, value, onChange, placeholder = 'main', disabled }: Props) {
  const [open, setOpen] = useState(false);

  return (
    <Popover open={open} onOpenChange={setOpen} modal>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" role="combobox" aria-expanded={open} disabled={disabled} className="h-9 w-full justify-between font-normal">
          <span className="flex min-w-0 items-center gap-2">
            <GitBranch className="size-3.5 shrink-0 text-muted-foreground" />
            <span className={cn('truncate', !value && 'text-muted-foreground')}>{value || placeholder}</span>
          </span>
          <ChevronsUpDown className="size-3.5 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start" onCloseAutoFocus={(e) => e.preventDefault()}>
        {open && <BranchList owner={owner} repo={repo} value={value} onChange={onChange} onClose={() => setOpen(false)} />}
      </PopoverContent>
    </Popover>
  );
}

function BranchList({ owner, repo, value, onChange, onClose }: { owner: string; repo: string; value: string; onChange: (v: string) => void; onClose: () => void }) {
  const { t } = useTranslation();
  const entry = useMemo(() => getBranchesEntry(owner, repo), [owner, repo]);
  const { data, initial, error } = useGhEntry(entry);
  const names = useMemo(() => data.map((b) => b.name), [data]);
  const showCreate = value.trim() && !names.includes(value.trim());

  return (
    <Command>
      <CommandInput placeholder={t('integrations.repository.searchBranchTag')} value={value} onValueChange={onChange} />
      <CommandList>
        {initial && <div className="py-6 text-center text-sm text-muted-foreground">{t('integrations.repository.loadingBranches')}</div>}
        {error && !initial && <div className="py-6 text-center text-sm text-destructive">{error}</div>}
        {!initial && !error && (
          <>
            <CommandEmpty>{t('integrations.repository.noBranchMatch')}</CommandEmpty>
            {showCreate && (
              <CommandGroup heading={t('integrations.repository.customGroup')}>
                <CommandItem
                  value={`__use__${value}`}
                  onSelect={() => {
                    onChange(value.trim());
                    onClose();
                  }}
                >
                  <GitBranch className="size-3.5" />
                  {t('integrations.repository.useValue', { value: value.trim() })}
                </CommandItem>
              </CommandGroup>
            )}
            {names.length > 0 && (
              <CommandGroup heading={t('integrations.repository.branchesGroup')}>
                {names.map((branch) => (
                  <CommandItem
                    key={branch}
                    value={branch}
                    onSelect={() => {
                      onChange(branch);
                      onClose();
                    }}
                  >
                    <GitBranch className="size-3.5" />
                    <span className="truncate">{branch}</span>
                    <Check className={cn('ml-auto size-3.5', value === branch ? 'opacity-100' : 'opacity-0')} />
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </>
        )}
      </CommandList>
    </Command>
  );
}
