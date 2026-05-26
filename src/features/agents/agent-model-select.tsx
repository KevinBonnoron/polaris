import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import type { AgentModel } from './agent-kinds';

interface Props {
  models: AgentModel[];
  value: string;
  onChange: (value: string) => void;
  triggerKey?: string;
  triggerClassName?: string;
  disabled?: boolean;
  displayLabel?: string;
}

export function AgentModelSelect({ models, value, onChange, triggerKey, triggerClassName = 'w-[160px] shrink-0', disabled, displayLabel }: Props) {
  return (
    <Select key={triggerKey} value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger size="sm" className={triggerClassName}>
        <SelectValue>{displayLabel}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        {models.map((m) => (
          <SelectItem key={m.value} value={m.value}>
            {m.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
