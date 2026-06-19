import { useEffect, useRef } from 'react';

const STICK_THRESHOLD = 24;

export function useTerminalAutoscroll(scroller: HTMLElement | null) {
  const stickRef = useRef(true);

  useEffect(() => {
    if (!scroller) {
      return;
    }
    const pin = () => {
      const max = scroller.scrollHeight - scroller.clientHeight;
      if (stickRef.current && scroller.scrollTop < max) {
        scroller.scrollTop = max;
      }
    };
    const onScroll = () => {
      stickRef.current = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight <= STICK_THRESHOLD;
    };
    scroller.addEventListener('scroll', onScroll, { passive: true });
    const mo = new MutationObserver(pin);
    mo.observe(scroller, { childList: true, subtree: true, characterData: true });
    const ro = new ResizeObserver(pin);
    ro.observe(scroller);
    pin();
    return () => {
      scroller.removeEventListener('scroll', onScroll);
      mo.disconnect();
      ro.disconnect();
    };
  }, [scroller]);
}
