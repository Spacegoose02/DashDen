// A feed thumbnail that fails to load used to collapse to a dot, so that item's
// text jumped left of the others. The failed picture now gives way to the same
// placeholder an item without a picture gets, and the row keeps its shape.
(function () {
    const PLACEHOLDER = '<svg class="scale-half hide-on-mobile" stroke="var(--color-text-subdue)" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="m2.25 15.75 5.159-5.159a2.25 2.25 0 0 1 3.182 0l5.159 5.159m-1.5-1.5 1.409-1.409a2.25 2.25 0 0 1 3.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 0 0 1.5-1.5V6a1.5 1.5 0 0 0-1.5-1.5H3.75A1.5 1.5 0 0 0 2.25 6v12a1.5 1.5 0 0 0 1.5 1.5Zm10.5-11.25h.008v.008h-.008V8.25Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Z" /></svg>';

    function isThumbnail(img) {
        return img instanceof HTMLImageElement &&
            (img.classList.contains("thumbnail") || img.classList.contains("reddit-card-thumbnail"));
    }

    function replaceFailed(img) {
        if (img.dataset.thumbnailFailed) return;
        img.dataset.thumbnailFailed = "1";

        const detailed = img.closest(".rss-detailed-thumbnail");
        if (detailed) {
            const holder = document.createElement("template");
            holder.innerHTML = PLACEHOLDER;
            img.replaceWith(holder.content.firstChild);
            return;
        }

        // Cards keep their image box, just without the broken-image icon.
        img.style.visibility = "hidden";
    }

    document.addEventListener("error", (event) => {
        if (isThumbnail(event.target)) replaceFailed(event.target);
    }, true);

    // Images that failed before this script ran.
    for (const img of document.querySelectorAll("img.thumbnail, img.reddit-card-thumbnail")) {
        if (img.complete && img.getAttribute("src") && img.naturalWidth === 0) replaceFailed(img);
    }
})();
