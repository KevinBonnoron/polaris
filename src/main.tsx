import { createRouter, RouterProvider } from '@tanstack/react-router';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@wterm/react/css';
import './lib/i18n';
import { wailsReady } from './lib/wails-ready';
import { AppearanceProvider } from './providers/appearance';
import { routeTree } from './routeTree.gen';
import './styles.css';

const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('missing #root');
}

const hideSplash = () => {
  const splash = document.getElementById('polaris-splash');
  if (!splash) return;
  splash.classList.add('is-hidden');
  splash.addEventListener(
    'transitionend',
    () => {
      splash.remove();
    },
    { once: true },
  );
};

const mount = () => {
  createRoot(rootEl).render(
    <StrictMode>
      <AppearanceProvider>
        <RouterProvider router={router} />
      </AppearanceProvider>
    </StrictMode>,
  );
  requestAnimationFrame(hideSplash);
};

wailsReady()
  .catch((err) => {
    console.warn('wails bridge not ready, rendering anyway:', err);
  })
  .finally(mount);
