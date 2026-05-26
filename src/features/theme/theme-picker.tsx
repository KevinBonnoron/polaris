import { useMemo, type ReactNode } from 'react';
import { THEMES, type Theme } from '@/lib/themes';
import { useTheme, useThemePreview } from '@/providers/theme-accent';
import { ThemeChip } from './theme-chip';

interface Props {
  value: string;
  onChange: (key: string) => void;
  children?: ReactNode;
}

export function ThemePicker({ value, onChange, children }: Props) {
  const { setPreview } = useThemePreview();
  const { customThemes } = useTheme();
  const pick = (key: string) => {
    onChange(key);
    setPreview(key);
  };

  const sorted = useMemo(() => {
    const customAsThemes: Theme[] = customThemes.map((c) => ({
      key: c.key,
      name: c.name,
      mode: c.mode === 'light' ? 'light' : 'dark',
    }));
    return [...THEMES, ...customAsThemes].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
  }, [customThemes]);

  return (
    <div className="flex flex-wrap items-center gap-2 p-1">
      {sorted.map((theme) => (
        <ThemeChip key={theme.key} selected={value === theme.key} onPick={() => pick(theme.key)} name={theme.name} themeKey={theme.key} />
      ))}
      {children}
    </div>
  );
}
