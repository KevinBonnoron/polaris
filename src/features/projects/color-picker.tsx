import { Check } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { PROJECT_COLORS } from './project-colors';

interface Props {
  value: string;
  onChange: (color: string) => void;
}

export function ColorPicker({ value, onChange }: Props) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-2">
      {PROJECT_COLORS.map((color) => {
        const active = value === color;
        return (
          <button
            key={color}
            type="button"
            onClick={() => onChange(color)}
            className={cn('flex size-8 items-center justify-center rounded-md transition-transform', active ? 'ring-2 ring-foreground/80 ring-offset-2 ring-offset-background' : 'hover:scale-105')}
            style={{ background: color }}
            aria-label={t('projects.colorAriaLabel', { color })}
          >
            {active && <Check className="size-4 text-white drop-shadow" />}
          </button>
        );
      })}
    </div>
  );
}
