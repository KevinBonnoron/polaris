import { Box, Boxes, Database, type LucideIcon } from 'lucide-react';

export const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  done: 'default',
  running: 'secondary',
  error: 'destructive',
  cancelled: 'outline',
  idle: 'outline',
};

const DATABASE_TYPES = new Set(['postgres', 'mysql', 'mariadb', 'mongo', 'redis']);

function isDatabase(type: string): boolean {
  return DATABASE_TYPES.has(type);
}

export function isDeployable(type: string): boolean {
  return type === 'application' || type === 'compose';
}

export function serviceIcon(type: string): LucideIcon {
  if (isDatabase(type)) {
    return Database;
  }
  return type === 'compose' ? Boxes : Box;
}

function isTerminal(status: string): boolean {
  return status === 'done' || status === 'error' || status === 'cancelled';
}
