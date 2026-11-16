// Registers the service worker that makes DashDen installable and lets it
// open offline with the last version that loaded.
(function () {
    if (!("serviceWorker" in navigator)) return;
    const base = document.currentScript?.dataset.baseUrl || "";

    window.addEventListener("load", () => {
        navigator.serviceWorker.register(base + "/sw.js", { scope: base + "/" }).catch(() => {});
    });

    // Signing out forgets the pages kept for offline use on this device.
    document.addEventListener("submit", (event) => {
        const form = event.target;
        if (!(form instanceof HTMLFormElement) || !/\/logout$/.test(form.action)) return;
        navigator.serviceWorker.controller?.postMessage("dashden-forget");
    }, true);

    // A quiet badge while the device is offline.
    const badge = document.createElement("div");
    badge.className = "offline-badge";
    badge.setAttribute("role", "status");
    badge.textContent = "Offline. Showing the last version that loaded.";
    const update = () => {
        badge.hidden = navigator.onLine;
        if (!badge.isConnected && document.body) document.body.append(badge);
    };
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    document.addEventListener("DOMContentLoaded", update);
    if (document.readyState !== "loading") update();
})();
