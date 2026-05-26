import type { AnyFieldApi } from '@tanstack/react-form';
import { cn } from '@/lib/utils';

export function FieldError({ field, className }: { field: AnyFieldApi; className?: string }) {
  const errors = field.state.meta.errors;
  const touched = field.state.meta.isTouched;
  if (!touched || errors.length === 0) {
    return null;
  }
  const message = errors.find((e) => typeof e === 'string') as string | undefined;
  if (!message) {
    return null;
  }
  return <p className={cn('text-xs text-destructive', className)}>{message}</p>;
}

export function isInvalid(field: AnyFieldApi): boolean {
  return field.state.meta.isTouched && field.state.meta.errors.length > 0;
}
