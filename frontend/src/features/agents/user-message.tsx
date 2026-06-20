import { ImageOff } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { ReadFileBase64 } from '@/wailsjs/go/main/App';
import { highlightSegments } from './file-mentions';

const IMAGE_EXT_RE = /\.(png|jpe?g|gif|webp|bmp|svg|avif)$/i;

export function basename(path: string): string {
  return path.split(/[\\/]/).pop() ?? path;
}

function isImagePath(line: string): boolean {
  const token = line.trim().replace(/^@/, '');
  if (!token) {
    return false;
  }
  if (!(token.startsWith('/') || /^[A-Za-z]:[\\/]/.test(token))) {
    return false;
  }
  return IMAGE_EXT_RE.test(token);
}

export function mimeForPath(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase();
  switch (ext) {
    case 'jpg':
    case 'jpeg':
      return 'image/jpeg';
    case 'gif':
      return 'image/gif';
    case 'webp':
      return 'image/webp';
    case 'bmp':
      return 'image/bmp';
    case 'svg':
      return 'image/svg+xml';
    case 'avif':
      return 'image/avif';
    default:
      return 'image/png';
  }
}

export function ImageZoom({ src, name, className }: { src: string; name: string; className?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={`group relative overflow-hidden rounded-md border ${className ?? ''}`} title={name}>
        <img src={src} alt={name} className="h-20 max-w-[12rem] object-cover transition-transform group-hover:scale-105" />
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-[90vw] sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle className="truncate font-mono text-sm font-normal">{name}</DialogTitle>
          </DialogHeader>
          <img src={src} alt={name} className="max-h-[75vh] w-full rounded-md object-contain" />
        </DialogContent>
      </Dialog>
    </>
  );
}

function MessageImage({ path }: { path: string }) {
  const { t } = useTranslation();
  const [src, setSrc] = useState<string | null>(null);
  const [missing, setMissing] = useState(false);
  const name = basename(path);

  useEffect(() => {
    let cancelled = false;
    setSrc(null);
    setMissing(false);
    ReadFileBase64(path)
      .then((b64) => {
        if (!cancelled) {
          setSrc(`data:${mimeForPath(path)};base64,${b64}`);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setMissing(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [path]);

  if (missing) {
    return (
      <div className="flex h-20 w-24 flex-col items-center justify-center gap-1 rounded-md border border-dashed bg-muted px-1 text-center text-[10px] leading-tight text-muted-foreground" title={name}>
        <ImageOff className="size-4" />
        <span className="line-clamp-2">{t('agents.detail.imageUnavailable')}</span>
      </div>
    );
  }

  if (!src) {
    return <Skeleton className="h-20 w-24 rounded-md" />;
  }

  return <ImageZoom src={src} name={name} />;
}

export function UserMessageContent({ content }: { content: string }) {
  const lines = content.split('\n');
  const images = lines.filter(isImagePath).map((line) => line.trim().replace(/^@/, ''));
  const text = lines
    .filter((line) => !isImagePath(line))
    .join('\n')
    .trim();

  return (
    <div className="col-span-2 my-1 space-y-2 rounded-md bg-muted/50 px-3 py-2 text-sm text-foreground/80">
      {text && <div className="whitespace-pre-wrap break-words">{highlightSegments(text, { requireAt: true })}</div>}
      {images.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {images.map((path) => (
            <MessageImage key={path} path={path} />
          ))}
        </div>
      )}
    </div>
  );
}
