/**
 * Best-effort error description extractor.
 *
 * Handles plain strings, Error instances, and HTTP-style error objects carrying
 * `status`, `response.message`, or per-field validation details under `data`.
 */
export function describeError(err: unknown): string {
  if (!err) {
    return 'Unknown error';
  }

  if (typeof err === 'string') {
    return err;
  }

  if (typeof err !== 'object') {
    return String(err);
  }

  const e = err as Record<string, unknown>;
  const parts: string[] = [];

  const status = e.status;
  if (typeof status === 'number' && status > 0) {
    parts.push(`HTTP ${status}`);
  }

  const responseMessage = (e.response as { message?: unknown } | undefined)?.message;
  if (typeof responseMessage === 'string' && responseMessage) {
    parts.push(responseMessage);
  }

  const data = e.data;
  if (data && typeof data === 'object') {
    for (const [field, info] of Object.entries(data as Record<string, unknown>)) {
      const infoMessage = (info as { message?: unknown } | null | undefined)?.message;
      if (typeof infoMessage === 'string') {
        parts.push(`${field}: ${infoMessage}`);
      }
    }
  }

  if (parts.length === 0) {
    const message = e.message;
    if (typeof message === 'string' && message) {
      parts.push(message);
    }
  }

  const original = e.originalError;
  if (original && typeof original === 'object') {
    const inner = describeError(original);
    if (inner && !parts.includes(inner)) {
      parts.push(`(cause: ${inner})`);
    }
  }

  return parts.length > 0 ? parts.join(' · ') : 'Unknown error';
}

/**
 * Dump the full error structure for clipboard / log purposes.
 */
export function dumpError(err: unknown): string {
  try {
    const seen = new WeakSet<object>();
    return JSON.stringify(
      err,
      (_key, value) => {
        if (typeof value === 'object' && value !== null) {
          if (seen.has(value as object)) {
            return '[Circular]';
          }
          seen.add(value as object);
        }
        if (value instanceof Error) {
          return {
            name: value.name,
            message: value.message,
            stack: value.stack,
            ...(value as unknown as Record<string, unknown>),
          };
        }
        return value;
      },
      2,
    );
  } catch {
    return String(err);
  }
}
