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

// Config changes are picked up by the file watcher about half a second later.
async function waitForConfigReload() {
    await new Promise((r) => setTimeout(r, 1200));
    await waitForReload();
}

const splitList = (text) => text.split(",").map((v) => v.trim()).filter(Boolean);
const joinList = (list) => (list || []).join(", ");

function option(value, label) {
    const o = document.createElement("option");
    o.value = value;
    o.textContent = label;
    return o;
}

function avatar(person) {
    const span = document.createElement("span");
    span.className = "settings-avatar";
    const a = person.avatar || "";
    if (a.startsWith("/") || a.startsWith("http://") || a.startsWith("https://")) {
        const img = document.createElement("img");
        img.src = a;
        img.alt = "";
        span.append(img);
    } else {
        span.textContent = a || (person.name || person.username).slice(0, 1).toUpperCase();
    }
    return span;
}

const roleLabels = { admin: "Admin", adult: "Adult", kid: "Kid", guest: "Guest", "": "No role" };

let peopleData;
const personForm = document.getElementById("person-form");

function openPersonForm(person) {
    const f = personForm;
    f.hidden = false;
    f.reset();
    document.getElementById("person-form-title").textContent = person ? `Edit ${person.name || person.username}` : "Add a person";
    f.username.readOnly = !!person;
    f.role.replaceChildren(...["", ...peopleData.roles].map((r) => option(r, roleLabels[r] || r)));
    f.home.replaceChildren(option("", "The first page"), ...peopleData.pages.map((p) => option(p.value, p.label)));
    f.theme.replaceChildren(option("", "Their own choice"), ...peopleData.themes.map((t) => option(t.value, t.label)));
    document.getElementById("person-password-field").hidden = !peopleData.passwordSignIn;
    if (person) {
        f.username.value = person.username;
        f.name.value = person.name || "";
        f.avatar.value = person.avatar || "";
        f.role.value = person.role || "";
        f.home.value = person.home || "";
        f.theme.value = person.theme || "";
        f.groups.value = joinList(person.groups);
    } else {
        f.role.value = "adult";
    }
    f.scrollIntoView({ behavior: "smooth", block: "start" });
    (person ? f.name : f.username).focus();
}

document.getElementById("person-add").addEventListener("click", () => openPersonForm(null));
document.getElementById("person-cancel").addEventListener("click", () => { personForm.hidden = true; });

personForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const f = personForm;
    const username = f.username.value.trim();
    const body = {
        name: f.name.value, avatar: f.avatar.value, role: f.role.value,
        home: f.home.value, theme: f.theme.value, groups: splitList(f.groups.value),
    };
    if (f.password.value) body.password = f.password.value;
    try {
        await call("PUT", "/people/" + encodeURIComponent(username), body);
        f.hidden = true;
        notify(`Saved ${body.name || username}, reloading…`);
        await waitForConfigReload();
        loadPeople();
    } catch (e) {
        notify(e.message, true);
    }
});

async function loadPeople() {
    const tbody = document.querySelector("#people-table tbody");
    try {
        peopleData = await call("GET", "/people");
        const pageNames = Object.fromEntries(peopleData.pages.map((p) => [p.value, p.label]));
        tbody.replaceChildren();
        if (!peopleData.people.length) {
            const tr = document.createElement("tr");
            const td = cell("No one yet. Add yourself first and make yourself an admin.", "color-subdue");
            td.colSpan = 5;
            tr.append(td);
            tbody.append(tr);
        }
        for (const p of peopleData.people) {
            const tr = document.createElement("tr");
            const av = document.createElement("td");
            av.append(avatar(p));
            tr.append(av);
            const who = cell("");
            const name = document.createElement("div");
            name.className = "color-highlight";
            name.textContent = (p.name || p.username) + (p.username === peopleData.you ? " (you)" : "");
            const sub = document.createElement("div");
            sub.className = "size-h6 color-subdue settings-mono";
            sub.textContent = p.username + (p.groups?.length ? " · " + p.groups.join(", ") : "") + (p.hasPassword ? " · has a password" : "");
            who.append(name, sub);
            tr.append(who);
            tr.append(cell(roleLabels[p.role || ""] || p.role));
            tr.append(cell(p.home ? (pageNames[p.home] || p.home) : "First page", p.home ? "" : "color-subdue"));
            const actions = document.createElement("td");
            actions.className = "settings-actions";
            actions.append(button("Edit", () => openPersonForm(p)));
            if (p.username !== peopleData.you) {
                actions.append(button("Remove", async () => {
                    if (!confirm(`Remove ${p.name || p.username} from the household? They can still sign in, they just lose their profile.` + (p.hasPassword ? " Their local password is removed too." : ""))) return;
                    try {
                        await call("DELETE", "/people/" + encodeURIComponent(p.username) + (p.hasPassword ? "?password=remove" : ""));
                        notify("Removed, reloading…");
                        await waitForConfigReload();
                        loadPeople();
                    } catch (e) {
                        notify(e.message, true);
                    }
                }, true));
            }
            tr.append(actions);
            tbody.append(tr);
        }
    } catch (e) {
        notify("Could not load people: " + e.message, true);
    }
}

const signinForm = document.getElementById("signin-form");

function syncSigninFields() {
    const on = signinForm.enabled.checked;
    for (const input of signinForm.querySelectorAll(".settings-fields input")) {
        if (input.name !== "trustedProxies") input.disabled = !on;
    }
}
signinForm.enabled.addEventListener("change", syncSigninFields);

async function loadSignIn() {
    try {
        const data = await call("GET", "/signin");
        const s = data.settings;
        const f = signinForm;
        f.enabled.checked = s.proxy.enabled;
        f.trustedProxies.value = joinList(s.trustedProxies);
        f.loginURL.value = s.proxy.loginURL || "";
        f.logoutURL.value = s.proxy.logoutURL || "";
        f.allowedUsers.value = joinList(s.proxy.allowedUsers);
        f.allowedGroups.value = joinList(s.proxy.allowedGroups);
        f.userHeader.value = s.proxy.userHeader || "";
        f.groupsHeader.value = s.proxy.groupsHeader || "";
        syncSigninFields();

        const facts = document.getElementById("signin-facts");
        const rows = [
            ["This request came from", data.peer + (data.peerIsTrusted ? ", a trusted proxy" : "")],
            ["You signed in", data.recovery ? "With the recovery code" : data.signedInByProxy ? "Through the proxy" : "Some other way"],
            ["Other sign-in methods", [data.password && "Password", data.oidc && "OIDC"].filter(Boolean).join(", ") || "None"],
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
        notify("Could not load sign-in settings: " + e.message, true);
    }
}

signinForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const f = signinForm;
    const body = {
        trustedProxies: splitList(f.trustedProxies.value),
        proxy: {
            enabled: f.enabled.checked,
            loginURL: f.loginURL.value, logoutURL: f.logoutURL.value,
            allowedUsers: splitList(f.allowedUsers.value), allowedGroups: splitList(f.allowedGroups.value),
            userHeader: f.userHeader.value, groupsHeader: f.groupsHeader.value,
        },
    };
    try {
        await call("PUT", "/signin", body);
        notify("Saved sign-in, reloading…");
        await waitForConfigReload();
        loadSignIn();
    } catch (e) {
        notify(e.message, true);
    }
});

async function loadAccess() {
    const tbody = document.querySelector("#access-table tbody");
    try {
        const data = await call("GET", "/access");
        document.getElementById("access-noauth").hidden = data.authEnabled;
        document.getElementById("access-known").textContent =
            `People: ${data.users.join(", ") || "none yet"}. Groups: ${data.groups.join(", ")}.`;
        tbody.replaceChildren();
        for (const p of data.pages) {
            const tr = document.createElement("tr");
            tr.append(cell(p.title));
            const users = document.createElement("input");
            users.className = "settings-input";
            users.value = joinList(p.allowedUsers);
            users.placeholder = "Everyone";
            users.setAttribute("aria-label", `Users who can see ${p.title}`);
            const groups = document.createElement("input");
            groups.className = "settings-input";
            groups.value = joinList(p.allowedGroups);
            groups.placeholder = "Everyone";
            groups.setAttribute("aria-label", `Groups who can see ${p.title}`);
            const tdU = document.createElement("td");
            tdU.append(users);
            const tdG = document.createElement("td");
            tdG.append(groups);
            tr.append(tdU, tdG);
            const actions = document.createElement("td");
            actions.className = "settings-actions";
            if (p.editable) {
                actions.append(button("Save", async () => {
                    try {
                        await call("PUT", "/access/" + p.index, { allowedUsers: splitList(users.value), allowedGroups: splitList(groups.value) });
                        notify(`Saved who can see ${p.title}, reloading…`);
                        await waitForConfigReload();
                        loadAccess();
                    } catch (e) {
                        notify(e.message, true);
                    }
                }));
            } else {
                users.disabled = groups.disabled = true;
                const note = document.createElement("span");
                note.className = "color-subdue";
                note.textContent = "In an included file";
                actions.append(note);
            }
            tr.append(actions);
            tbody.append(tr);
        }
    } catch (e) {
        notify("Could not load page access: " + e.message, true);
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

const loaders = { overview: loadOverview, people: loadPeople, signin: loadSignIn, access: loadAccess, secrets: loadSecrets, history: loadHistory };

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
