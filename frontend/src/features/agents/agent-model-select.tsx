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

export function AgentModelSelect({ models, value, onChange, triggerKey, triggerClassName = 'w-fit min-w-[150px] max-w-[280px] shrink-0', disabled, displayLabel }: Props) {
  const selectedLabel = displayLabel ?? models.find((m) => m.value === value)?.label;
  return (
    <Select key={triggerKey} value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger size="sm" className={triggerClassName}>
        <SelectValue>{selectedLabel}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        {models.map((m) => (
          <SelectItem key={m.value} value={m.value}>
            {m.description ? (
              <div className="flex flex-col gap-0.5">
                <span>{m.label}</span>
                <span className="text-xs text-muted-foreground">{m.description}</span>
              </div>
            ) : (
              m.label
            )}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
