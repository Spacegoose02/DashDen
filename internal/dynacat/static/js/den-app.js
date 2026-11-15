// Buttons on den-app widgets: send the action, then swap in the fresh widget
// the server returns.
(function () {
    const base = (typeof pageData !== 'undefined' && pageData.baseURL) || '';

    async function send(root, action, body, controls) {
        const widgetId = root.dataset.denWidget;
        const error = root.querySelector('.den-app-error');
        controls.forEach((c) => { c.disabled = true; });
        try {
            const resp = await fetch(base + '/api/widgets/' + widgetId + '/action/' + action, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include',
                body: JSON.stringify(body),
            });
            const text = await resp.text();
            if (!resp.ok) throw new Error(text.trim() || resp.statusText);

            const tmp = document.createElement('div');
            tmp.innerHTML = text;
            const fresh = tmp.querySelector('.den-app');
            if (fresh) root.replaceWith(fresh);
            if (typeof window.dynacatSetupCollapsibleLists === 'function') window.dynacatSetupCollapsibleLists();
        } catch (e) {
            controls.forEach((c) => { c.disabled = false; });
            if (error) {
                error.textContent = e.message;
                error.hidden = false;
                setTimeout(() => { error.hidden = true; }, 6000);
            }
        }
    }

    document.addEventListener('click', (event) => {
        const button = event.target.closest('button[data-den-action]');
        if (!button) return;
        const root = button.closest('.den-app');
        if (!root) return;
        const d = button.dataset;
        const body = {};
        for (const key of ['kid', 'chore', 'id', 'op']) if (d[key]) body[key] = d[key];
        send(root, d.denAction, body, [button]);
    });

    document.addEventListener('submit', (event) => {
        const form = event.target.closest('form[data-den-action]');
        if (!form) return;
        event.preventDefault();
        const root = form.closest('.den-app');
        const input = form.elements.text;
        if (!root || !input.value.trim()) return;
        send(root, form.dataset.denAction, { text: input.value }, [...form.elements]);
    });
})();
