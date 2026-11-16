// DashDen: packs the tiles of bento pages (see page-bento.go). Each widget
// spans its tile width in columns, and as many small rows as its height
// needs, so tiles of different heights fit together without gaps.

const MAX_SPAN = 4;

function px(value) {
    return parseFloat(value) || 0;
}

function layoutGrid(grid) {
    if (document.body.classList.contains("editing")) return;

    const widgets = grid.querySelectorAll(":scope > .page-column > .widget");
    // Wide tiles add implicit columns when they don't fit, and those show up
    // in the computed track list, so count the tracks without them.
    widgets.forEach((w) => (w.style.gridColumn = ""));
    const style = getComputedStyle(grid);
    const columns = style.gridTemplateColumns.split(" ").filter(Boolean).length || 1;
    const unit = px(style.gridAutoRows) || 4;
    const gap = px(style.columnGap);

    widgets.forEach((widget) => {
        const wanted = Math.min(parseInt(widget.dataset.tileW || "1", 10) || 1, MAX_SPAN);
        widget.style.gridColumn = `span ${Math.min(wanted, columns)}`;

        const height = widget.getBoundingClientRect().height;
        const rows = Math.max(1, Math.ceil((height + gap) / unit));
        widget.style.gridRow = `span ${rows}`;
    });
}

let scheduled = false;
function layoutAll() {
    if (scheduled) return;
    scheduled = true;
    requestAnimationFrame(() => {
        scheduled = false;
        document.querySelectorAll(".page-columns-bento").forEach(layoutGrid);
    });
}

const resizes = new ResizeObserver(layoutAll);
const watched = new WeakSet();

function watch() {
    document.querySelectorAll(".page-columns-bento:not([data-bento-watched])").forEach((grid) => {
        grid.dataset.bentoWatched = "1";
        resizes.observe(grid);
        new MutationObserver((mutations) => {
            // Only new widgets matter; a ticking clock changes nothing here.
            if (mutations.some((m) => [...m.addedNodes].some((n) => n.classList?.contains("widget")))) {
                watchWidgets(grid);
                layoutAll();
            }
        }).observe(grid, { childList: true, subtree: true });
        watchWidgets(grid);
        layoutAll();
    });
}

function watchWidgets(grid) {
    grid.querySelectorAll(":scope > .page-column > .widget").forEach((widget) => {
        if (!watched.has(widget)) {
            watched.add(widget);
            resizes.observe(widget);
        }
    });
}

// The page content arrives after load, and again on page switches.
new MutationObserver(() => {
    if (document.querySelector(".page-columns-bento:not([data-bento-watched])")) watch();
}).observe(document.body, { childList: true, subtree: true });
window.addEventListener("load", watch);
watch();

// Wide view: one click fills the whole screen on any page, remembered on
// this device.
document.addEventListener("click", (event) => {
    const toggle = event.target.closest("[data-wide-toggle]");
    if (!toggle) return;
    const wide = document.documentElement.classList.toggle("dd-wide");
    try {
        localStorage.setItem("dashden-wide", wide ? "1" : "0");
    } catch (e) {}
    toggle.setAttribute("aria-pressed", wide ? "true" : "false");
    layoutAll();
});

function syncWideToggles() {
    const wide = document.documentElement.classList.contains("dd-wide");
    document.querySelectorAll("[data-wide-toggle]").forEach((t) => t.setAttribute("aria-pressed", wide ? "true" : "false"));
}
syncWideToggles();
