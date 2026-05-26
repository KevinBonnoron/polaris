import { Pencil, RotateCcw } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { type ShortcutId, DEFAULT_SHORTCUTS, SHORTCUT_IDS, shortcutDisplayKeys } from '@/lib/shortcuts';
import { useShortcuts } from '@/providers/shortcuts';

interface ShortcutRowProps {
  id: ShortcutId;
  label: string;
}

function ShortcutRow({ id, label }: ShortcutRowProps) {
  const { shortcuts, isMac, modifierSymbol, isRecording, setRecording, updateShortcut, resetShortcut } = useShortcuts();
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);

  const def = shortcuts[id];
  const keys = shortcutDisplayKeys(def, modifierSymbol);
  const isDefault = def.key === DEFAULT_SHORTCUTS[id].key && def.meta === DEFAULT_SHORTCUTS[id].meta;

  const startEdit = () => {
    setRecording(true);
    setEditing(true);
  };

  const cancelEdit = () => {
    setRecording(false);
    setEditing(false);
  };

  useEffect(() => {
    if (!editing) {
      return;
    }

    const handler = (e: KeyboardEvent) => {
      e.preventDefault();
      e.stopImmediatePropagation();

      if (e.key === 'Escape') {
        setRecording(false);
        setEditing(false);
        return;
      }

      if (e.key === 'Meta' || e.key === 'Control' || e.key === 'Shift' || e.key === 'Alt') {
        return;
      }

      const modifierHeld = isMac ? e.metaKey : e.ctrlKey;
      updateShortcut(id, { key: e.key, meta: modifierHeld });
      setRecording(false);
      setEditing(false);
    };

    window.addEventListener('keydown', handler, { capture: true });
    return () => window.removeEventListener('keydown', handler, { capture: true });
  }, [editing, isMac, id, updateShortcut, setRecording]);

  return (
    <div className="flex items-center justify-between rounded-md bg-muted/40 px-3 py-2 text-sm">
      <span>{label}</span>
      <div className="flex items-center gap-1.5">
        {editing ? (
          <span className="animate-pulse rounded border border-border bg-background px-2 py-0.5 font-mono text-xs text-muted-foreground">{t('settings.shortcuts.pressKey')}</span>
        ) : (
          <div className="flex gap-1">
            {keys.map((key) => (
              <kbd key={key} className="rounded border border-border bg-background px-1.5 py-0.5 font-mono text-xs">
                {key}
              </kbd>
            ))}
          </div>
        )}
        <Button variant="ghost" size="icon" className="size-6" onClick={editing ? cancelEdit : startEdit} disabled={isRecording && !editing} title={editing ? t('settings.shortcuts.cancelEdit') : t('settings.shortcuts.editShortcut')}>
          <Pencil className="size-3" />
        </Button>
        {!isDefault && !editing && (
          <Button variant="ghost" size="icon" className="size-6" onClick={() => resetShortcut(id)} title={t('settings.shortcuts.reset')}>
            <RotateCcw className="size-3" />
          </Button>
        )}
      </div>
    </div>
  );
}

export function ShortcutsSettings() {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        {SHORTCUT_IDS.map((id) => (
          <ShortcutRow key={id} id={id} label={t(`settings.shortcuts.${id}` as const)} />
        ))}
      </div>
    </section>
  );
}
