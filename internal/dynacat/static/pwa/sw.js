// DashDen service worker. Makes the dashboard installable and lets it open
// without a connection, showing the last version that loaded.
const VERSION = "__VERSION__";
const BASE = "__BASE__";
const SHELL_CACHE = "dashden-shell-" + VERSION;
const PAGE_CACHE = "dashden-pages";

self.addEventListener("install", (event) => {
    self.skipWaiting();
});

self.addEventListener("activate", (event) => {
    event.waitUntil((async () => {
        for (const key of await caches.keys()) {
            if (key.startsWith("dashden-shell-") && key !== SHELL_CACHE) await caches.delete(key);
        }
        await self.clients.claim();
    })());
});

self.addEventListener("message", (event) => {
    // Sent on sign-out so the next person on this device doesn't see the
    // previous person's pages offline.
    if (event.data === "dashden-forget") {
        event.waitUntil(caches.delete(PAGE_CACHE));
    }
});

function isStatic(url) {
    return url.pathname.startsWith(BASE + "/static/");
}

function isCachablePage(request, url) {
    if (request.method !== "GET" || url.origin !== self.location.origin) return false;
    if (request.mode === "navigate") return !url.pathname.startsWith(BASE + "/settings") && !url.pathname.startsWith(BASE + "/login");
    return /\/api\/pages\/[^/]+\/content\/?$/.test(url.pathname);
}

self.addEventListener("fetch", (event) => {
    const request = event.request;
    const url = new URL(request.url);
    if (request.method !== "GET" || url.origin !== self.location.origin) return;

    // Static files carry a hash in their path, so a cached copy never goes stale.
    if (isStatic(url)) {
        event.respondWith((async () => {
            const cache = await caches.open(SHELL_CACHE);
            const hit = await cache.match(request);
            if (hit) return hit;
            const response = await fetch(request);
            if (response.ok) cache.put(request, response.clone());
            return response;
        })());
        return;
    }

    // Pages and their content: always the network first, the last copy when offline.
    if (isCachablePage(request, url)) {
        event.respondWith((async () => {
            const cache = await caches.open(PAGE_CACHE);
            try {
                const response = await fetch(request);
                if (response.ok && !response.redirected) cache.put(request, response.clone());
                return response;
            } catch (err) {
                const hit = await cache.match(request, { ignoreSearch: request.mode === "navigate" });
                if (hit) {
                    if (request.mode !== "navigate") return hit;
                    return hit;
                }
                if (request.mode === "navigate") {
                    return new Response(
                        '<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><title>DashDen</title>' +
                        '<body style="margin:0;display:grid;place-items:center;min-height:100vh;background:#15141a;color:#d6d4dc;font:15px system-ui,sans-serif">' +
                        '<div style="text-align:center;padding:24px"><p style="font-size:18px;color:#fff">You\'re offline</p>' +
                        '<p>DashDen will load as soon as this device can reach it again.</p></div>',
                        { status: 503, headers: { "Content-Type": "text/html; charset=utf-8" } });
                }
                throw err;
            }
        })());
    }
});
