// Service worker: makes the app itself (HTML, JS, CSS, icons) open offline.
//
// Data is NOT handled here: /api requests always go to the network, and the
// app keeps its own offline copy of the list in IndexedDB (src/sync).
//
// Strategies:
//   navigation (index.html)  network first, cached copy when offline
//   /assets/*                cache first — Vite gives them content-hashed
//                            names, so a cached file never goes stale
//   everything else          cached copy at once, refreshed in background
//
// Bump VERSION only when this file's caching logic changes; app updates
// need no bump (new index.html → new hashed asset names).

const VERSION = "v1";
const SHELL = `fm-shell-${VERSION}`;
const ASSETS = `fm-assets-${VERSION}`;
const MAX_ASSETS = 60;

self.addEventListener("install", (event) => {
  event.waitUntil(precacheShell());
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      for (const key of await caches.keys()) {
        if (key !== SHELL && key !== ASSETS) await caches.delete(key);
      }
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/api/")) return;

  if (req.mode === "navigate") {
    event.respondWith(networkFirstPage(req));
  } else if (url.pathname.startsWith("/assets/")) {
    event.respondWith(cacheFirst(req));
  } else {
    event.respondWith(staleWhileRevalidate(req));
  }
});

// The page and everything it references, so the very first visit already
// leaves a working offline copy (the SW isn't controlling that first load,
// so its asset requests wouldn't be cached otherwise).
async function precacheShell() {
  const res = await fetch("/", { cache: "no-store" });
  if (!res.ok) return;
  const html = await res.clone().text();
  const shell = await caches.open(SHELL);
  await shell.put("/", res);
  await shell.addAll(["/manifest.webmanifest", "/icons/icon-192.png", "/icons/icon.svg"]);
  const assets = [...html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g)].map((m) => m[1]);
  await (await caches.open(ASSETS)).addAll(assets);
}

async function networkFirstPage(req) {
  const shell = await caches.open(SHELL);
  try {
    const res = await fetch(req);
    // All routes are the same SPA page, so it's cached under one key.
    if (res.ok) await shell.put("/", res.clone());
    return res;
  } catch {
    return (await shell.match("/")) ?? Response.error();
  }
}

async function cacheFirst(req) {
  const cache = await caches.open(ASSETS);
  const hit = await cache.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res.ok) {
    await cache.put(req, res.clone());
    await trim(cache);
  }
  return res;
}

async function staleWhileRevalidate(req) {
  const cache = await caches.open(SHELL);
  const hit = await cache.match(req);
  const fresh = fetch(req)
    .then((res) => {
      if (res.ok) void cache.put(req, res.clone());
      return res;
    })
    .catch(() => hit ?? Response.error());
  return hit ?? fresh;
}

// Old builds' assets pile up across deploys; keep the newest MAX_ASSETS.
async function trim(cache) {
  const keys = await cache.keys();
  for (const key of keys.slice(0, Math.max(0, keys.length - MAX_ASSETS))) await cache.delete(key);
}
