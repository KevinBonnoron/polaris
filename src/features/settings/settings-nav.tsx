import type { LucideIcon } from 'lucide-react';
import { Bell, Bot, Info, Keyboard, Palette, Settings, User } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';

export type SettingsSection = 'general' | 'appearance' | 'agents' | 'notifications' | 'shortcuts' | 'account' | 'about';

interface NavEntry {
  id: SettingsSection;
  icon: LucideIcon;
}

const ENTRIES: NavEntry[] = [
  { id: 'general', icon: Settings },
  { id: 'agents', icon: Bot },
  { id: 'appearance', icon: Palette },
  { id: 'notifications', icon: Bell },
  { id: 'shortcuts', icon: Keyboard },
  { id: 'account', icon: User },
  { id: 'about', icon: Info },
];

interface Props {
  current: SettingsSection;
  onSelect: (id: SettingsSection) => void;
}

export function SettingsNav({ current, onSelect }: Props) {
  const { t } = useTranslation();
  return (
    <nav className="flex w-44 shrink-0 flex-col gap-1">
      {ENTRIES.map((entry) => {
        const Icon = entry.icon;
        const active = current === entry.id;
        return (
          <button key={entry.id} type="button" onClick={() => onSelect(entry.id)} className={cn('flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors', active ? 'bg-accent text-accent-foreground font-medium' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground')}>
            <Icon className="size-4" />
            {t(`settings.nav.${entry.id}` as const)}
          </button>
        );
      })}
    </nav>
  );
}
