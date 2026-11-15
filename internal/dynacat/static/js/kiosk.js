// Kiosk mode for a wall tablet: no header, navigation or footer, the screen
// stays on, the cursor hides when idle, and with ?rotate=N the pages take
// turns every N seconds. Double-tap the top-right corner to leave.
(function () {
    const root = document.documentElement;
    if (!root.classList.contains("kiosk")) return;

    let lock = null;
    async function keepAwake() {
        try {
            if ("wakeLock" in navigator && document.visibilityState === "visible") {
                lock = await navigator.wakeLock.request("screen");
            }
        } catch (e) {
            // Not allowed here (for example over plain http); the screen may sleep.
        }
    }
    keepAwake();
    document.addEventListener("visibilitychange", () => {
        if (document.visibilityState === "visible" && (!lock || lock.released)) keepAwake();
    });

    let idleTimer;
    const wake = () => {
        root.classList.remove("kiosk-idle");
        clearTimeout(idleTimer);
        idleTimer = setTimeout(() => root.classList.add("kiosk-idle"), 3000);
    };
    for (const ev of ["mousemove", "pointerdown", "keydown"]) document.addEventListener(ev, wake, { passive: true });
    wake();

    const exit = document.createElement("button");
    exit.type = "button";
    exit.className = "kiosk-exit";
    exit.setAttribute("aria-label", "Leave kiosk mode (double-tap)");
    let lastTap = 0;
    exit.addEventListener("click", () => {
        const now = Date.now();
        if (now - lastTap < 600) {
            try { localStorage.setItem("dashden-kiosk", "0"); } catch (e) {}
            root.classList.remove("kiosk", "kiosk-idle");
            exit.remove();
            clearTimeout(rotateTimer);
        }
        lastTap = now;
    });
    document.addEventListener("DOMContentLoaded", () => document.body.append(exit));
    if (document.readyState !== "loading") document.body.append(exit);

    let rotateTimer;
    let seconds = 0;
    try { seconds = parseInt(localStorage.getItem("dashden-kiosk-rotate") || "0", 10) || 0; } catch (e) {}
    if (seconds >= 10) {
        let quietSince = Date.now();
        document.addEventListener("pointerdown", () => { quietSince = Date.now(); }, { passive: true });
        const next = () => {
            // Wait while someone is using the screen.
            if (Date.now() - quietSince < seconds * 1000) {
                rotateTimer = setTimeout(next, seconds * 1000 - (Date.now() - quietSince));
                return;
            }
            const links = [...document.querySelectorAll(".nav .nav-item")];
            if (links.length < 2) return;
            const current = links.findIndex((a) => a.classList.contains("nav-item-current"));
            location.href = links[(current + 1) % links.length].href;
        };
        rotateTimer = setTimeout(next, seconds * 1000);
    }
})();
