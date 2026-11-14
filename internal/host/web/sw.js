'use strict';

const CACHE = 'cloche-files';
const SHELL = ['/', '/app.js', '/style.css', '/manifest.webmanifest', '/icon-180.png', '/icon-192.png', '/fonts/plex_mono_regular.ttf', '/fonts/plex_mono_semibold.ttf'];

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()));
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

const isPage = (path) => path === '/' || path === '/pair' || /^\/b\/[a-z0-9]+$/.test(path);

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (e.request.method !== 'GET' || url.origin !== location.origin) return;
  const page = e.request.mode === 'navigate' && isPage(url.pathname);
  if (!page && !SHELL.includes(url.pathname)) return;
  e.respondWith(
    fetch(e.request)
      .then((res) => {
        const html = (res.headers.get('Content-Type') || '').startsWith('text/html');
        if (res.ok && (!page || html)) {
          const copy = res.clone();
          caches.open(CACHE).then((c) => c.put(page ? '/' : e.request, copy));
        }
        return res;
      })
      .catch(() => caches.match(page ? '/' : e.request).then((r) => r || Response.error())),
  );
});

self.addEventListener('push', (e) => {
  let d = {};
  try { d = e.data.json(); } catch { d = { title: 'Cloche', body: e.data ? e.data.text() : '' }; }
  e.waitUntil(self.registration.showNotification(d.title || 'Cloche', {
    body: d.body || '',
    tag: d.tag || undefined,
    icon: '/icon-192.png',
    data: { url: d.url || '/' },
  }));
});

self.addEventListener('notificationclick', (e) => {
  e.notification.close();
  const url = (e.notification.data && e.notification.data.url) || '/';
  e.waitUntil((async () => {
    const wins = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    for (const w of wins) {
      if (new URL(w.url).origin === location.origin) {
        await w.focus();
        w.postMessage({ type: 'go', url });
        return;
      }
    }
    await self.clients.openWindow(url);
  })());
});
