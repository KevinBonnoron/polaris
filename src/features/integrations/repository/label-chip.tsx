import type { Label } from './types';
import { readableTextColor } from './utils';

interface Props {
  label: Label;
}

export function LabelChip({ label }: Props) {
  const bg = `#${label.color || '6b7280'}`;
  return (
    <span className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium leading-none" style={{ background: bg, color: readableTextColor(label.color) }}>
      {label.name}
    </span>
  );
}
