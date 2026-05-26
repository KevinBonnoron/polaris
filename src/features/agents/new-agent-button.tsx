import { ChevronDown, Plus } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { useAgentClis } from '@/state/agent-clis';
import type { AgentKind } from '@/types';
import { AgentDetailModal } from './agent-detail-modal';

type Size = 'default' | 'sm';

interface Props {
  size?: Size;
  variant?: 'default' | 'outline';
  className?: string;
  onSelect?: (kindId: AgentKind) => void;
}

export function NewAgentButton({ size = 'sm', variant = 'default', className, onSelect }: Props) {
  const { t } = useTranslation();
  const { kinds } = useAgentClis();
  const installed = kinds.filter((k) => k.installed);
  const [pending, setPending] = useState<AgentKind | null>(null);

  if (installed.length === 0) {
    return (
      <Button size={size} variant={variant} disabled className={className}>
        <Plus className="size-3.5" />
        {t('agents.page.newAgent')}
      </Button>
    );
  }

  if (installed.length === 1) {
    const only = installed[0];
    if (onSelect) {
      return (
        <Button size={size} variant={variant} className={className} onClick={() => onSelect(only.id)}>
          <Plus className="size-3.5" />
          {t('agents.page.newAgent')}
        </Button>
      );
    }
    return (
      <AgentDetailModal pending={{ kindId: only.id }}>
        <Button size={size} variant={variant} className={className}>
          <Plus className="size-3.5" />
          {t('agents.page.newAgent')}
        </Button>
      </AgentDetailModal>
    );
  }

  const handleSelect = onSelect ?? ((id: AgentKind) => setTimeout(() => setPending(id), 0));

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size={size} variant={variant} className={className}>
            <Plus className="size-3.5" />
            {t('agents.page.newAgent')}
            <ChevronDown className="size-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" sideOffset={4} className="w-56">
          {installed.map((k) => {
            const Icon = k.icon;
            return (
              <DropdownMenuItem key={k.id} onSelect={() => handleSelect(k.id)} className="gap-2">
                <span className={cn('flex size-5 shrink-0 items-center justify-center rounded', k.iconClass)}>
                  <Icon className="size-3" />
                </span>
                <span>{k.label}</span>
              </DropdownMenuItem>
            );
          })}
        </DropdownMenuContent>
      </DropdownMenu>
      {!onSelect && pending && <AgentDetailModal pending={{ kindId: pending }} open={true} onOpenChange={(o) => !o && setPending(null)} />}
    </>
  );
}
