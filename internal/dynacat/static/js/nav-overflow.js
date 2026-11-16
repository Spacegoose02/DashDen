// When there are more pages than fit in the header, the ones that don't fit
// go into a "More" menu instead of scrolling out of sight. The links stay in
// the nav (hidden), so the editor and keyboard shortcuts keep working.
(function () {
    const nav = document.querySelector(".header .nav");
    if (!nav) return;

    const items = Array.from(nav.querySelectorAll(":scope > .nav-item"));
    if (items.length < 2) return;

    const more = document.createElement("div");
    more.className = "nav-more";
    more.hidden = true;
    more.innerHTML = '<button type="button" class="nav-more-button" aria-haspopup="true" aria-expanded="false">More<svg viewBox="0 0 20 20" fill="currentColor" aria-hidden="true"><path fill-rule="evenodd" d="M5.22 8.22a.75.75 0 0 1 1.06 0L10 11.94l3.72-3.72a.75.75 0 1 1 1.06 1.06l-4.25 4.25a.75.75 0 0 1-1.06 0L5.22 9.28a.75.75 0 0 1 0-1.06Z" clip-rule="evenodd"/></svg></button><div class="nav-more-menu" role="menu" hidden></div>';
    nav.after(more);

    const button = more.querySelector(".nav-more-button");
    const menu = more.querySelector(".nav-more-menu");

    const setOpen = (open) => {
        menu.hidden = !open;
        button.setAttribute("aria-expanded", String(open));
    };
    button.addEventListener("click", (e) => {
        e.stopPropagation();
        setOpen(menu.hidden);
    });
    document.addEventListener("click", (e) => {
        if (!more.contains(e.target)) setOpen(false);
    });
    document.addEventListener("keydown", (e) => {
        if (e.key === "Escape") setOpen(false);
    });

    function layout() {
        items.forEach((item) => item.classList.remove("nav-item-overflow"));
        menu.replaceChildren();

        if (document.body.classList.contains("editing")) {
            more.hidden = true;
            return;
        }

        more.hidden = true;
        if (nav.scrollWidth <= nav.clientWidth + 1) return;

        more.hidden = false;
        const limit = nav.getBoundingClientRect().right;
        const current = items.find((item) => item.classList.contains("nav-item-current"));

        let overflow = items.filter((item) => item.getBoundingClientRect().right > limit + 1);
        // Keep the page you're on in view: hide the last visible one instead.
        if (current && overflow.includes(current)) {
            const visible = items.filter((item) => !overflow.includes(item));
            const swap = visible[visible.length - 1];
            if (swap) overflow = [swap, ...overflow.filter((i) => i !== current)];
        }

        for (const item of overflow) {
            item.classList.add("nav-item-overflow");
            const link = document.createElement("a");
            link.href = item.href;
            link.role = "menuitem";
            link.className = "nav-more-item";
            link.textContent = item.textContent.trim();
            const keybind = item.querySelector(".nav-keybind-hint");
            if (keybind) link.textContent = link.textContent.replace(new RegExp(keybind.textContent + "$"), "").trim();
            menu.append(link);
        }
    }

    let frame = 0;
    const schedule = () => {
        cancelAnimationFrame(frame);
        frame = requestAnimationFrame(layout);
    };
    window.addEventListener("resize", schedule);
    document.fonts?.ready.then(schedule);
    new MutationObserver(schedule).observe(document.body, { attributes: true, attributeFilter: ["class"] });
    layout();
})();
