import { Check, CircleDashed, MessageSquareWarning } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

interface Props {
  decision: string;
}

const META: Record<string, { labelKey: string; className: string; Icon: typeof Check }> = {
  APPROVED: { labelKey: 'integrations.repository.reviewApproved', className: 'text-emerald-500', Icon: Check },
  CHANGES_REQUESTED: { labelKey: 'integrations.repository.reviewChangesRequested', className: 'text-destructive', Icon: MessageSquareWarning },
  REVIEW_REQUIRED: { labelKey: 'integrations.repository.reviewRequired', className: 'text-amber-500', Icon: CircleDashed },
};

export function ReviewStatusBadge({ decision }: Props) {
  const { t } = useTranslation();
  const meta = META[decision];
  if (!meta) {
    return null;
  }
  const { labelKey, className, Icon } = meta;
  const label = t(labelKey);
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={cn('inline-flex items-center', className)} aria-label={label}>
            <Icon className="size-3.5" />
          </span>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
