import { type DragEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useStatusBarBlocks } from '@/providers/theme-accent';

const ALL_STATUS_BAR_BLOCKS = [
  { id: 'model', i18nKey: 'blockModel', dummyKey: 'dummyModel' },
  { id: 'tools', i18nKey: 'blockTools', dummyKey: 'dummyTools' },
  { id: 'tokens', i18nKey: 'blockTokens', dummyKey: 'dummyTokens' },
  { id: 'tools-used', i18nKey: 'blockToolsUsed', dummyKey: 'dummyToolsUsed' },
  { id: 'cost', i18nKey: 'blockCost', dummyKey: 'dummyCost' },
  { id: 'provider', i18nKey: 'blockProvider', dummyKey: 'dummyProvider' },
  { id: 'files', i18nKey: 'blockFiles', dummyKey: 'dummyFiles' },
  { id: 'duration', i18nKey: 'blockDuration', dummyKey: 'dummyDuration' },
  { id: 'usage', i18nKey: 'blockUsage', dummyKey: 'dummyUsage' },
] as const;

const SEP_VARIANTS = ['dot', 'bar', 'dash'] as const;
type SepVariant = (typeof SEP_VARIANTS)[number];
const SEP_GLYPHS: Record<SepVariant, string> = { dot: '·', bar: '|', dash: '—' };

export function isSeparator(id: string) {
  return id.startsWith('sep:');
}

function sepVariant(id: string): SepVariant {
  const v = id.split(':')[2] as SepVariant;
  return SEP_VARIANTS.includes(v) ? v : 'dot';
}

export function sepGlyph(id: string) {
  return SEP_GLYPHS[sepVariant(id)];
}

function makeSepId(variant: SepVariant = 'dot') {
  return `sep:${Date.now()}:${variant}`;
}

const USAGE_MODES = ['remaining', 'used'] as const;
export type UsageMode = (typeof USAGE_MODES)[number];

export function isUsageBlock(id: string) {
  return id === 'usage' || id.startsWith('usage:');
}

export function usageMode(id: string): UsageMode {
  if (id === 'usage') return 'remaining';
  const v = id.split(':')[1] as UsageMode;
  return USAGE_MODES.includes(v) ? v : 'remaining';
}

const USAGE_DUMMY: Record<UsageMode, string> = { remaining: '● 72%', used: '● 28%' };

function StatusBarSettings() {
  const { t } = useTranslation();
  const { blocks, setBlocks } = useStatusBarBlocks();

  return (
    <section className="flex flex-col gap-6">
      <StatusBarConfigurator blocks={blocks} onChange={setBlocks} />
    </section>
  );
}

export function StatusBarConfigurator({ blocks, onChange }: { blocks: string[]; onChange: (blocks: string[]) => void }) {
  const { t } = useTranslation();
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);

  const enabled = blocks.filter((id) => isSeparator(id) || isUsageBlock(id) || ALL_STATUS_BAR_BLOCKS.some((b) => b.id === id));
  const hasUsage = blocks.some(isUsageBlock);
  const disabled = ALL_STATUS_BAR_BLOCKS.filter((b) => (b.id === 'usage' ? !hasUsage : !blocks.includes(b.id)));

  const blockDef = (id: string) => ALL_STATUS_BAR_BLOCKS.find((b) => b.id === id);
  const dummyFor = (id: string) => {
    if (isUsageBlock(id)) {
      const mode = usageMode(id);
      const label = mode === 'remaining' ? t('agents.usage.left') : t('agents.usage.used');
      return `${USAGE_DUMMY[mode]} ${label}`;
    }
    const def = blockDef(id);
    return def ? t(`settings.appearance.${def.dummyKey}`) : id;
  };

  const clearDrag = () => {
    setDraggingId(null);
    setDropTarget(null);
  };

  const cycleSepVariant = (id: string) => {
    const current = sepVariant(id);
    const nextIdx = (SEP_VARIANTS.indexOf(current) + 1) % SEP_VARIANTS.length;
    const newId = `sep:${id.split(':')[1]}:${SEP_VARIANTS[nextIdx]}`;
    onChange(blocks.map((b) => (b === id ? newId : b)));
  };

  const cycleUsageMode = (id: string) => {
    const current = usageMode(id);
    const nextIdx = (USAGE_MODES.indexOf(current) + 1) % USAGE_MODES.length;
    const newId = `usage:${USAGE_MODES[nextIdx]}`;
    onChange(blocks.map((b) => (b === id ? newId : b)));
  };

  const dragHandlers = (id: string) => ({
    draggable: true as const,
    onDragStart: (e: DragEvent) => {
      e.dataTransfer.setData('text/plain', id);
      e.dataTransfer.effectAllowed = 'move' as const;
      setDraggingId(id);
    },
    onDragEnd: clearDrag,
    onDragOver: (e: DragEvent) => {
      if (!draggingId || draggingId === id || !blocks.includes(draggingId)) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move' as const;
      const from = blocks.indexOf(draggingId);
      const to = blocks.indexOf(id);
      if (from >= 0 && to >= 0 && from !== to) {
        const next = [...blocks];
        next.splice(from, 1);
        next.splice(to, 0, draggingId);
        onChange(next);
      }
    },
    onDrop: (e: DragEvent) => e.preventDefault(),
  });

  const badgeClass = (id: string) =>
    cn('cursor-grab select-none text-muted-foreground transition-opacity', draggingId === id && 'opacity-40');

  return (
    <div className="flex flex-col gap-3">
      <div
        onDragOver={(e) => {
          if (!draggingId) return;
          e.preventDefault();
          e.dataTransfer.dropEffect = 'move';
          if (!blocks.includes(draggingId)) setDropTarget('__preview__');
        }}
        onDragLeave={(e) => {
          if (e.currentTarget.contains(e.relatedTarget as Node)) return;
          setDropTarget(null);
        }}
        onDrop={(e) => {
          e.preventDefault();
          if (!draggingId || blocks.includes(draggingId)) {
            clearDrag();
            return;
          }
          if (draggingId === '__sep__') {
            onChange([...blocks, makeSepId()]);
          } else {
            onChange([...blocks, draggingId]);
          }
          clearDrag();
        }}
        className={cn('flex min-h-10 flex-wrap items-center gap-1 rounded-md border border-dashed px-3 py-2 transition-colors', dropTarget === '__preview__' ? 'border-primary/60 bg-primary/5' : 'border-border bg-muted/30')}
      >
        {enabled.length === 0 && <span className="text-xs text-muted-foreground">{t('settings.statusBar.subtitle')}</span>}
        {enabled.map((id, idx) => {
          const key = isSeparator(id) ? `${id}-${idx}` : id;
          const onClick = isSeparator(id) ? () => cycleSepVariant(id) : isUsageBlock(id) ? () => cycleUsageMode(id) : undefined;
          const clickable = !!onClick;

          if (clickable) {
            return (
              <Tooltip key={key}>
                <TooltipTrigger asChild>
                  <Badge variant="outline" {...dragHandlers(id)} onClick={onClick} className={badgeClass(id)}>
                    {isSeparator(id) ? sepGlyph(id) : dummyFor(id)}
                  </Badge>
                </TooltipTrigger>
                <TooltipContent>{t('settings.statusBar.clickCycle')}</TooltipContent>
              </Tooltip>
            );
          }
          return (
            <Badge key={key} variant="outline" {...dragHandlers(id)} className={badgeClass(id)}>
              {dummyFor(id)}
            </Badge>
          );

        })}
      </div>

      <div
        onDragOver={(e) => {
          if (!draggingId || !blocks.includes(draggingId)) return;
          e.preventDefault();
          e.dataTransfer.dropEffect = 'move';
          setDropTarget('__pool__');
        }}
        onDragLeave={(e) => {
          if (e.currentTarget.contains(e.relatedTarget as Node)) return;
          setDropTarget(null);
        }}
        onDrop={(e) => {
          e.preventDefault();
          if (!draggingId || !blocks.includes(draggingId)) {
            clearDrag();
            return;
          }
          onChange(blocks.filter((b) => b !== draggingId));
          clearDrag();
        }}
        className={cn('flex min-h-10 flex-wrap items-center gap-1 rounded-md border border-dashed px-3 py-2 transition-colors', dropTarget === '__pool__' ? 'border-primary/60 bg-primary/5' : 'border-border')}
      >
        {disabled.map((block) => (
          <Badge
            key={block.id}
            variant="outline"
            draggable
            onDragStart={(e) => {
              e.dataTransfer.setData('text/plain', block.id);
              e.dataTransfer.effectAllowed = 'move';
              setDraggingId(block.id);
            }}
            onDragEnd={clearDrag}
            onDrop={(e) => e.preventDefault()}
            className={cn('cursor-grab select-none border-dashed text-muted-foreground transition-opacity', draggingId === block.id && 'opacity-40')}
          >
            {t(`settings.appearance.${block.i18nKey}`)}
          </Badge>
        ))}
        <Badge
          variant="outline"
          draggable
          onDragStart={(e) => {
            e.dataTransfer.setData('text/plain', '__sep__');
            e.dataTransfer.effectAllowed = 'move';
            setDraggingId('__sep__');
          }}
          onDragEnd={clearDrag}
          onDrop={(e) => e.preventDefault()}
          className={cn('cursor-grab select-none border-dashed text-muted-foreground transition-opacity', draggingId === '__sep__' && 'opacity-40')}
        >
          {t('settings.appearance.blockSeparator')}
        </Badge>
      </div>
    </div>
  );
}
