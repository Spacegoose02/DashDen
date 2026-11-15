const API = pageData.baseURL + "/api/settings";

const toast = document.getElementById("settings-toast");
let toastTimer;

function notify(message, isError = false) {
    toast.textContent = message;
    toast.classList.toggle("settings-toast-error", isError);
    toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { toast.hidden = true; }, 5000);
}

async function call(method, path, body) {
    const options = { method, headers: {} };
    if (body !== undefined) {
        options.headers["Content-Type"] = "application/json";
        options.body = JSON.stringify(body);
    }
    const response = await fetch(API + path, options);
    let data = {};
    try { data = await response.json(); } catch (e) {}
    if (!response.ok) throw new Error(data.error || response.statusText);
    return data;
}

function cell(text, className) {
    const td = document.createElement("td");
    td.textContent = text;
    if (className) td.className = className;
    return td;
}

function button(label, onClick, danger = false) {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "settings-button settings-button-small" + (danger ? " settings-button-danger" : "");
    b.textContent = label;
    b.addEventListener("click", onClick);
    return b;
}

// Wait for the server to come back after a reload-triggering change.
async function waitForReload() {
    for (let i = 0; i < 40; i++) {
        await new Promise((r) => setTimeout(r, 500));
        try {
            const r = await fetch(pageData.baseURL + "/api/healthz", { cache: "no-store" });
            if (r.ok) return;
        } catch (e) {}
    }
}

async function loadOverview() {
    const facts = document.getElementById("overview-facts");
    try {
        const o = await call("GET", "/overview");
        const rows = [
            ["Sign-in", o.signIn.length ? o.signIn.join(", ") : "None, anyone who can reach DashDen can see it"],
            ["Household", o.members ? `${o.members} people, admins: ${o.admins.length ? o.admins.join(", ") : "none yet"}` : "No household profiles yet"],
            ["Pages", String(o.pages)],
            ["Secrets", o.missingSecrets ? `${o.missingSecrets} used by the config but not set` : "All set"],
            ["History", `${o.snapshots} saved versions`],
            ["Config file", o.configPath],
            ["Version", o.version],
        ];
        facts.replaceChildren();
        for (const [k, v] of rows) {
            const dt = document.createElement("dt");
            dt.textContent = k;
            const dd = document.createElement("dd");
            dd.textContent = v;
            facts.append(dt, dd);
        }
    } catch (e) {
        notify("Could not load the overview: " + e.message, true);
    }
}

const sourceLabels = {
    settings: "Saved here",
    environment: "Container or env file",
    missing: "Missing",
};

async function loadSecrets() {
    const tbody = document.querySelector("#secrets-table tbody");
    try {
        const data = await call("GET", "/secrets");
        document.getElementById("secrets-file").textContent = "Stored in " + data.file;
        tbody.replaceChildren();
        if (!data.secrets.length) {
            const tr = document.createElement("tr");
            tr.append(cell("Nothing yet.", "color-subdue"));
            tbody.append(tr);
        }
        for (const s of data.secrets) {
            const tr = document.createElement("tr");
            tr.append(cell(s.name, "settings-mono"));
            tr.append(cell(sourceLabels[s.source] + (s.usedByConfig ? "" : ", not used by the config"), s.source === "missing" ? "color-negative" : ""));
            const actions = document.createElement("td");
            actions.className = "settings-actions";
            actions.append(button(s.source === "missing" ? "Set" : "Replace", () => {
                const form = document.getElementById("secret-form");
                form.name.value = s.name;
                form.value.focus();
            }));
            if (s.source === "settings") {
                actions.append(button("Remove", async () => {
                    if (!confirm(`Remove ${s.name}? Widgets that use it will stop working.`)) return;
                    try {
                        await call("DELETE", "/secrets/" + encodeURIComponent(s.name));
                        notify(`Removed ${s.name}, reloading…`);
                        await waitForReload();
                        loadSecrets();
                    } catch (e) {
                        notify(e.message, true);
                    }
                }, true));
            }
            tr.append(actions);
            tbody.append(tr);
        }
    } catch (e) {
        notify("Could not load secrets: " + e.message, true);
    }
}

document.getElementById("secret-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.target;
    const name = form.name.value.trim();
    try {
        await call("PUT", "/secrets/" + encodeURIComponent(name), { value: form.value.value });
        form.reset();
        notify(`Saved ${name}, reloading…`);
        await waitForReload();
        loadSecrets();
    } catch (e) {
        notify(e.message, true);
    }
});

async function loadHistory() {
    const tbody = document.querySelector("#history-table tbody");
    try {
        const data = await call("GET", "/history");
        tbody.replaceChildren();
        if (!data.snapshots.length) {
            const tr = document.createElement("tr");
            tr.append(cell("No saves yet. Changes made with the editor or Settings will show up here.", "color-subdue"));
            tbody.append(tr);
        }
        for (const s of data.snapshots) {
            const tr = document.createElement("tr");
            tr.append(cell(new Date(s.created).toLocaleString()));
            tr.append(cell(s.files.map((f) => f.path).join(", "), "settings-mono"));
            const actions = document.createElement("td");
            actions.className = "settings-actions";
            actions.append(button("Restore", async () => {
                if (!confirm("Put these files back as they were just before this save?")) return;
                try {
                    await call("POST", `/history/${encodeURIComponent(s.id)}/restore`, {});
                    notify("Restored, reloading…");
                    await waitForReload();
                    loadHistory();
                } catch (e) {
                    notify(e.message, true);
                }
            }));
            tr.append(actions);
            tbody.append(tr);
        }
    } catch (e) {
        notify("Could not load history: " + e.message, true);
    }
}

const loaders = { overview: loadOverview, secrets: loadSecrets, history: loadHistory };

function showTab(name) {
    for (const tab of document.querySelectorAll(".settings-tab")) {
        tab.setAttribute("aria-selected", String(tab.dataset.tab === name));
    }
    for (const panel of document.querySelectorAll(".settings-panel")) {
        panel.hidden = panel.dataset.panel !== name;
    }
    if (location.hash !== "#" + name) history.replaceState(null, "", "#" + name);
    loaders[name]?.();
}

for (const tab of document.querySelectorAll(".settings-tab")) {
    tab.addEventListener("click", () => showTab(tab.dataset.tab));
}

showTab(loaders[location.hash.slice(1)] ? location.hash.slice(1) : "overview");
