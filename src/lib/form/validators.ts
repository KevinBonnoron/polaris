import i18n from '@/lib/i18n';

export type ValidatorResult = string | undefined;

const t = (key: string, opts?: Record<string, unknown>) => i18n.t(key, opts) as string;

export const required =
  (key = 'common.validation.required') =>
  ({ value }: { value: unknown }): ValidatorResult => {
    if (value === undefined || value === null) {
      return t(key);
    }
    if (typeof value === 'string' && value.trim().length === 0) {
      return t(key);
    }
    if (Array.isArray(value) && value.length === 0) {
      return t(key);
    }
    return undefined;
  };

export const url =
  (key = 'common.validation.invalidUrl') =>
  ({ value }: { value: string | undefined | null }): ValidatorResult => {
    const v = (value ?? '').trim();
    if (!v) {
      return undefined;
    }
    try {
      const parsed = new URL(v);
      if (!parsed.protocol.startsWith('http')) {
        return t(key);
      }
      return undefined;
    } catch {
      return t(key);
    }
  };

const min =
  (n: number, key = 'common.validation.tooSmall') =>
  ({ value }: { value: number | undefined | null }): ValidatorResult => {
    if (value === undefined || value === null || Number.isNaN(value)) {
      return undefined;
    }
    return value < n ? t(key, { min: n }) : undefined;
  };

export const combine =
  <T>(...fns: Array<(arg: { value: T }) => ValidatorResult>) =>
  (arg: { value: T }): ValidatorResult => {
    for (const fn of fns) {
      const result = fn(arg);
      if (result) {
        return result;
      }
    }
    return undefined;
  };
