interface Props {
  selected: boolean;
  onPick: () => void;
  name: string;
  themeKey: string;
}

export function ThemeChip({ selected, onPick, name, themeKey }: Props) {
  return (
    <button
      type="button"
      onClick={onPick}
      aria-label={`Pick theme ${name}`}
      aria-pressed={selected}
      className={`theme-${themeKey} inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-medium transition ${selected ? 'ring-2 ring-foreground ring-offset-2 ring-offset-background' : 'opacity-80 hover:opacity-100'}`}
      style={{ background: 'var(--background)', color: 'var(--foreground)', borderColor: 'var(--border)' }}
    >
      <span className="inline-block h-2.5 w-2.5 rounded-full" style={{ background: 'var(--primary)' }} />
      <span className="max-[400px]:hidden">{name}</span>
    </button>
  );
}
