import { useEffect, useRef, useState } from 'react';

interface Props {
  scroller: HTMLElement | null;
}

export function TerminalScrollbar({ scroller }: Props) {
  const [thumb, setThumb] = useState<{ top: number; height: number } | null>(null);
  const dragRef = useRef<{ startY: number; startScroll: number; track: number; range: number } | null>(null);

  useEffect(() => {
    if (!scroller) {
      return;
    }
    const update = () => {
      const { scrollTop, scrollHeight, clientHeight } = scroller;
      const range = scrollHeight - clientHeight;
      if (range <= 1) {
        setThumb(null);
        return;
      }
      const height = Math.max(24, (clientHeight / scrollHeight) * clientHeight);
      const top = (scrollTop / range) * (clientHeight - height);
      setThumb({ top, height });
    };
    update();
    scroller.addEventListener('scroll', update, { passive: true });
    const ro = new ResizeObserver(update);
    ro.observe(scroller);
    const mo = new MutationObserver(update);
    mo.observe(scroller, { childList: true, subtree: true, characterData: true });
    return () => {
      scroller.removeEventListener('scroll', update);
      ro.disconnect();
      mo.disconnect();
    };
  }, [scroller]);

  const onPointerDown = (e: React.PointerEvent) => {
    if (!scroller || !thumb) {
      return;
    }
    e.preventDefault();
    dragRef.current = {
      startY: e.clientY,
      startScroll: scroller.scrollTop,
      track: scroller.clientHeight - thumb.height,
      range: scroller.scrollHeight - scroller.clientHeight,
    };
    const onMove = (ev: PointerEvent) => {
      const d = dragRef.current;
      if (!d || d.track <= 0) {
        return;
      }
      scroller.scrollTop = d.startScroll + ((ev.clientY - d.startY) / d.track) * d.range;
    };
    const onUp = () => {
      dragRef.current = null;
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  };

  if (!thumb) {
    return null;
  }

  return (
    <div className="pointer-events-none absolute top-0 right-0 z-10 h-full w-2.5 p-0.5">
      <div
        className="pointer-events-auto absolute right-0.5 w-1.5 rounded-full bg-border/60 transition-colors hover:bg-border"
        style={{ top: thumb.top, height: thumb.height }}
        onPointerDown={onPointerDown}
      />
    </div>
  );
}
