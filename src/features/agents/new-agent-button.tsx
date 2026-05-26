import { useLiveQuery } from '@tanstack/react-db';
import { useNavigate } from '@tanstack/react-router';
import { Bot, ChevronDown, Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { useAgentClis } from '@/state/agent-clis';
import { useCurrentProject } from '@/state/projects';
import type { SpawnTarget } from '@/types';
import { startDraftAgent } from './start-draft-agent';

type Size = 'default' | 'sm';

interface Props {
  size?: Size;
  variant?: 'default' | 'outline' | 'ghost';
  className?: string;
}

export function NewAgentButton({ size = 'sm', variant = 'default', className }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { projectId } = useCurrentProject();
  const { kinds, opencode, opencodeInstalled } = useAgentClis();
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));
  const installed = [...kinds.filter((k) => k.installed), ...(opencode.installed ? [opencode] : [])];

  const totalItems = installed.length + providers.length;
  const enabledCount = installed.length + (opencodeInstalled ? providers.length : 0);

  const handleSelect = (target: SpawnTarget) => {
    if (!projectId) {
      return;
    }
    void startDraftAgent(projectId, target);
    void navigate({ to: '/' });
  };

  if (totalItems === 0) {
    return (
      <Button size={size} variant={variant} disabled className={className}>
        <Plus className="size-3.5" />
        {t('agents.page.newAgent')}
      </Button>
    );
  }

  if (totalItems === 1 && enabledCount === 1) {
    const target: SpawnTarget = installed.length ? { kindId: installed[0].id } : { providerId: providers[0].id };
    return (
      <Button size={size} variant={variant} className={className} onClick={() => handleSelect(target)}>
        <Plus className="size-3.5" />
        {t('agents.page.newAgent')}
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size={size} variant={variant} className={className}>
          <Plus className="size-3.5" />
          {t('agents.page.newAgent')}
          <ChevronDown className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4} className="w-56" onCloseAutoFocus={(e) => e.preventDefault()}>
        {installed.map((k) => {
          const Icon = k.icon;
          return (
            <DropdownMenuItem key={k.id} onSelect={() => handleSelect({ kindId: k.id })} className="gap-2">
              <span className={cn('flex size-5 shrink-0 items-center justify-center rounded', k.iconClass)}>
                <Icon className="size-3" />
              </span>
              <span>{k.label}</span>
            </DropdownMenuItem>
          );
        })}
        {providers.length > 0 && (
          <>
            {installed.length > 0 && <DropdownMenuSeparator />}
            {!opencodeInstalled && <DropdownMenuLabel className="text-[11px] font-normal text-muted-foreground">{t('agents.new.opencodeRequired')}</DropdownMenuLabel>}
            {providers.map((p) => (
              <DropdownMenuItem key={p.id} disabled={!opencodeInstalled} onSelect={() => handleSelect({ providerId: p.id })} className="gap-2">
                <span className="flex size-5 shrink-0 items-center justify-center rounded text-white" style={{ background: p.color }}>
                  <Bot className="size-3" />
                </span>
                <span>{p.name}</span>
              </DropdownMenuItem>
            ))}
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
