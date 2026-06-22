import { toast } from 'sonner';
import { describeError, dumpError } from './errors';
import i18n from './i18n';

interface Options {
  title: string;
  err: unknown;
}

/**
 * Show an error toast with a description extracted from the underlying error,
 * plus a "Copy details" action that copies the full structured error to the
 * clipboard for bug-reporting / debugging.
 */
export function toastError({ title, err }: Options): void {
  const description = describeError(err);
  console.error(title, err);
  toast.error(title, {
    description,
    duration: 8000,
    action: {
      label: i18n.t('common.copyDetails'),
      onClick: () => {
        const blob = dumpError(err);
        navigator.clipboard?.writeText(blob).then(
          () => toast.success(i18n.t('common.detailsCopied')),
          () => toast.error(i18n.t('common.clipboardUnavailable')),
        );
      },
    },
  });
}
