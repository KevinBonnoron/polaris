import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { ReadFileBase64 } from '@/wailsjs/go/main/App';
import { basename, ImageZoom, mimeForPath } from './user-message';

export type Attachment = { path: string; preview: string };

export async function loadAttachment(path: string): Promise<Attachment> {
  try {
    const b64 = await ReadFileBase64(path);
    return { path, preview: `data:${mimeForPath(path)};base64,${b64}` };
  } catch {
    return { path, preview: '' };
  }
}

export function AttachmentPreviews({ attachments, onRemove }: { attachments: Attachment[]; onRemove: (index: number) => void }) {
  const { t } = useTranslation();
  if (attachments.length === 0) {
    return null;
  }
  return (
    <div className="mb-2 flex flex-wrap gap-2">
      {attachments.map((att, i) => (
        <div key={`${att.path}:${i}`} className="group relative">
          {att.preview ? <ImageZoom src={att.preview} name={basename(att.path)} /> : <div className="flex h-20 w-24 items-center justify-center rounded-md border bg-muted text-xs text-muted-foreground">{basename(att.path)}</div>}
          <button
            type="button"
            aria-label={t('agents.detail.removeAttachment', { name: basename(att.path) })}
            onClick={() => onRemove(i)}
            className="absolute -right-1.5 -top-1.5 z-10 flex size-5 items-center justify-center rounded-full bg-destructive text-destructive-foreground opacity-0 shadow-sm transition-opacity group-hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <X className="size-3" />
          </button>
        </div>
      ))}
    </div>
  );
}
