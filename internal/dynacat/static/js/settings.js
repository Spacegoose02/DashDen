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
    f.dataset.original = person ? person.username : "";
    document.getElementById("person-rename-note").hidden = !person;
    f.role.replaceChildren(...["", ...peopleData.roles].map((r) => option(r, roleLabels[r] || r)));
    f.home.replaceChildren(option("", "The first page"), ...peopleData.pages.map((p) => option(p.value, p.label)));
    f.theme.replaceChildren(option("", "Their own choice"), ...peopleData.themes.map((t) => option(t.value, t.label)));
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
    const original = f.dataset.original;
    const username = f.username.value.trim();
    const renaming = original && username !== original;
    const taken = renaming && peopleData.people.find((p) => p.username !== original && p.username.toLowerCase() === username.toLowerCase());
    if (taken) { notify(`Someone already has the username ${taken.username}.`, true); return; }
    if (renaming && !confirm(`Change the username ${original} to ${username}?` + (original === peopleData.you ? " You'll stay signed in." : " They sign in with the new name from now on."))) return;
    const body = {
        name: f.name.value, avatar: f.avatar.value, role: f.role.value,
        home: f.home.value, theme: f.theme.value, groups: splitList(f.groups.value),
    };
    if (f.password.value) body.password = f.password.value;
    try {
        await call("PUT", "/people/" + encodeURIComponent(original || username), body);
        let carried = true;
        if (renaming) {
            const result = await call("POST", "/people/" + encodeURIComponent(original) + "/rename", { newUsername: username });
            carried = result.dataCarried !== false;
        }
        f.hidden = true;
        if (!carried) notify("Renamed, but their saved theme, notes or to-dos couldn't be moved. The server log has the details.", true);
        else notify(renaming ? `Renamed ${original} to ${username}, reloading…` : `Saved ${body.name || username}, reloading…`);
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

const oidcForm = document.getElementById("oidc-form");
let oidcData;

function syncOIDCFields() {
    document.getElementById("oidc-role-field").hidden = !oidcForm.createAccounts.checked;
}
oidcForm.createAccounts.addEventListener("change", syncOIDCFields);

function openOIDCForm() {
    const o = oidcData.oidc, f = oidcForm;
    f.hidden = false;
    document.getElementById("oidc-form-title").textContent = o.enabled ? `Edit ${o.buttonName || "sign-in button"}` : "Add a sign-in button";
    f.buttonName.value = o.buttonName || "";
    f.issuerURL.value = o.issuerURL || "";
    f.clientID.value = o.clientID || "";
    f.clientSecret.value = "";
    f.clientSecret.placeholder = o.secretSaved ? "Saved; leave empty to keep it" : "From your provider";
    f.clientSecret.required = !o.secretSaved;
    f.extraScopes.value = joinList(o.extraScopes);
    f.createAccounts.checked = o.createAccounts;
    f.newAccountRole.replaceChildren(...oidcData.roles.map((r) => option(r, roleLabels[r] || r)));
    f.newAccountRole.value = o.newAccountRole;
    f.showOnLogin.checked = o.showOnLogin;
    document.getElementById("oidc-callback").textContent = o.callbackURL;
    syncOIDCFields();
    f.scrollIntoView({ behavior: "smooth", block: "start" });
    f.buttonName.focus();
}

async function loadOIDC() {
    const box = document.getElementById("oidc-summary");
    try {
        oidcData = await call("GET", "/oidc");
        const o = oidcData.oidc;
        box.replaceChildren();
        const p = document.createElement("p");
        p.className = "settings-note";
        if (o.enabled) {
            p.textContent = `${o.buttonName || "Sign-in button"} at ${o.issuerURL}` +
                (o.reachable === false ? ", which can't be reached right now." : ".") +
                (o.showOnLogin ? "" : " Hidden on the sign-in page.") +
                (o.createAccounts ? " Creates profiles for newcomers." : "");
            if (o.reachable === false) p.classList.add("color-negative");
        } else {
            p.textContent = "None yet.";
        }
        const actions = document.createElement("div");
        actions.className = "settings-actions";
        actions.append(button(o.enabled ? "Edit" : "Add a sign-in button", openOIDCForm));
        if (o.enabled) {
            actions.append(button("Remove", async () => {
                if (!confirm(`Remove the ${o.buttonName || "sign-in"} button?`)) return;
                try {
                    await call("DELETE", "/oidc");
                    notify("Removed, reloading…");
                    await waitForConfigReload();
                    loadOIDC();
                } catch (e) {
                    notify(e.message, true);
                }
            }, true));
        }
        box.append(p, actions);
    } catch (e) {
        notify("Could not load the sign-in button: " + e.message, true);
    }
}

document.getElementById("oidc-cancel").addEventListener("click", () => { oidcForm.hidden = true; });
document.getElementById("oidc-test").addEventListener("click", async () => {
    try {
        const r = await call("POST", "/oidc/test", { issuerURL: oidcForm.issuerURL.value });
        notify(r.message, !r.ok);
    } catch (e) {
        notify(e.message, true);
    }
});

oidcForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const f = oidcForm;
    const body = {
        buttonName: f.buttonName.value, issuerURL: f.issuerURL.value, clientID: f.clientID.value,
        clientSecret: f.clientSecret.value, extraScopes: splitList(f.extraScopes.value),
        createAccounts: f.createAccounts.checked, newAccountRole: f.newAccountRole.value,
        showOnLogin: f.showOnLogin.checked,
    };
    try {
        await call("PUT", "/oidc", body);
        f.hidden = true;
        notify("Saved the sign-in button, reloading…");
        await waitForConfigReload();
        loadOIDC();
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

function renderSetup(o) {
    const setup = o.setup;
    const steps = [
        {
            done: setup.hasAdmin,
            text: setup.hasAdmin ? "Someone in the household is an admin." : "Add yourself to the household as an admin, so only admins can change settings.",
            action: setup.hasAdmin ? null : (setup.you && !setup.youAreMember
                ? ["Make me an admin", async () => {
                    try {
                        await call("PUT", "/people/" + encodeURIComponent(setup.you), { role: "admin" });
                        notify("You're an admin now, reloading…");
                        await waitForConfigReload();
                        loadOverview();
                    } catch (e) { notify(e.message, true); }
                }]
                : ["Open People", () => showTab("people")]),
        },
        {
            done: setup.signIn,
            text: setup.signIn ? "Sign-in is set up." : "Set up sign-in with Tinyauth, so DashDen knows who's looking.",
            action: setup.signIn ? null : ["Open Sign-in", () => showTab("signin")],
        },
        {
            done: setup.appsAdded > 0,
            text: setup.appsAdded > 0 ? `${setup.appsAdded} of your apps are on the dashboard.` : "Connect your apps: chores, shopping list, music, games and TV.",
            action: setup.appsAdded > 0 ? null : ["Open Apps", () => showTab("apps")],
        },
        {
            done: !o.missingSecrets,
            text: o.missingSecrets ? `${o.missingSecrets} secrets the config uses aren't set yet.` : "Every secret the config uses is set.",
            action: o.missingSecrets ? ["Open Secrets", () => showTab("secrets")] : null,
        },
    ];
    const list = document.getElementById("setup-steps");
    list.replaceChildren();
    for (const step of steps) {
        const li = document.createElement("li");
        li.className = "settings-step" + (step.done ? " settings-step-done" : "");
        const mark = document.createElement("span");
        mark.className = "settings-step-mark";
        mark.textContent = step.done ? "✓" : "";
        mark.setAttribute("aria-label", step.done ? "Done" : "To do");
        const text = document.createElement("span");
        text.className = "settings-step-text";
        text.textContent = step.text;
        li.append(mark, text);
        if (step.action) li.append(button(step.action[0], step.action[1]));
        list.append(li);
    }
    document.getElementById("setup").hidden = steps.every((s) => s.done);
}

let themesData;

const hsl = (triple) => {
    const [h, s, l] = String(triple).split(/\s+/);
    return `hsl(${h} ${s}% ${l}%)`;
};

function seasonRow(season) {
    const tr = document.createElement("tr");
    const preset = document.createElement("select");
    preset.className = "settings-input";
    preset.setAttribute("aria-label", "Theme");
    preset.append(...themesData.presets.map((k) => option(k, k)));
    preset.value = season.preset || themesData.presets[0] || "";
    const from = document.createElement("input");
    from.className = "settings-input";
    from.placeholder = "12-01";
    from.value = season.from || "";
    from.setAttribute("aria-label", "From");
    const to = document.createElement("input");
    to.className = "settings-input";
    to.placeholder = "12-31";
    to.value = season.to || "";
    to.setAttribute("aria-label", "To");
    for (const el of [preset, from, to]) {
        const td = document.createElement("td");
        td.append(el);
        tr.append(td);
    }
    const actions = document.createElement("td");
    actions.className = "settings-actions";
    actions.append(button("Remove", () => tr.remove(), true));
    tr.append(actions);
    tr.read = () => ({ preset: preset.value, from: from.value.trim(), to: to.value.trim() });
    return tr;
}

async function loadThemes() {
    try {
        themesData = await call("GET", "/themes");
        document.getElementById("themes-picker-off").hidden = !themesData.pickerDisabled;
        const grid = document.getElementById("theme-pack");
        grid.replaceChildren();
        for (const t of themesData.pack) {
            const label = document.createElement("label");
            label.className = "settings-swatch";
            const check = document.createElement("input");
            check.type = "checkbox";
            check.value = t.key;
            check.disabled = t.installed;
            const preview = document.createElement("div");
            preview.className = "settings-swatch-preview";
            preview.style.background = hsl(t.background);
            const bar = document.createElement("div");
            bar.className = "settings-swatch-bar";
            bar.style.background = hsl(t.primary);
            const dots = document.createElement("div");
            dots.className = "settings-swatch-dots";
            for (const c of [t.primary, t.positive, t.negative]) {
                const dot = document.createElement("span");
                dot.style.background = hsl(c);
                dots.append(dot);
            }
            preview.append(bar, dots);
            const text = document.createElement("div");
            text.className = "settings-swatch-text";
            const name = document.createElement("div");
            name.className = "settings-swatch-name";
            name.textContent = t.name + (t.installed ? " ✓" : "");
            const desc = document.createElement("div");
            desc.className = "size-h6 color-subdue";
            desc.textContent = t.installed ? "Added" : t.description + (t.season ? ` Made for ${t.season}.` : "");
            text.append(name, desc);
            label.append(check, preview, text);
            grid.append(label);
        }

        const tbody = document.querySelector("#season-table tbody");
        tbody.replaceChildren(...themesData.seasonal.map(seasonRow));
        document.getElementById("season-now").textContent = themesData.currentSeason
            ? `Right now the default theme is ${themesData.currentSeason}.`
            : "No seasonal theme is active today.";
    } catch (e) {
        notify("Could not load themes: " + e.message, true);
    }
}

document.getElementById("theme-pack-add").addEventListener("click", async () => {
    const keys = [...document.querySelectorAll("#theme-pack input:checked:not(:disabled)")].map((c) => c.value);
    if (!keys.length) {
        notify("Pick at least one theme first.", true);
        return;
    }
    try {
        const r = await call("POST", "/themes/pack", { keys });
        notify(`Added ${r.added.length} themes, reloading…`);
        await waitForConfigReload();
        loadThemes();
    } catch (e) {
        notify(e.message, true);
    }
});

document.getElementById("season-add").addEventListener("click", () => {
    const suggestion = themesData.pack.find((t) => t.season && t.installed);
    document.querySelector("#season-table tbody").append(seasonRow(suggestion
        ? { preset: suggestion.key, from: suggestion.seasonFrom, to: suggestion.seasonTo }
        : {}));
});

document.getElementById("season-save").addEventListener("click", async () => {
    const seasonal = [...document.querySelectorAll("#season-table tbody tr")].map((tr) => tr.read());
    try {
        await call("PUT", "/themes/seasonal", { seasonal });
        notify("Saved seasons, reloading…");
        await waitForConfigReload();
        loadThemes();
    } catch (e) {
        notify(e.message, true);
    }
});

let appsPages = [];

function appForm(app) {
    const form = document.createElement("form");
    form.autocomplete = "off";
    const fields = document.createElement("div");
    fields.className = "settings-fields";
    const field = (label, name, type, set, placeholder) => {
        const l = document.createElement("label");
        l.textContent = label;
        const input = document.createElement("input");
        input.className = "settings-input";
        input.name = name;
        input.type = type;
        input.placeholder = set ? "Already set, leave empty to keep it" : placeholder;
        l.append(input);
        fields.append(l);
        return l;
    };
    field("Address", "url", "url", app.configured[app.prefix + "_URL"], "https://…");
    const tokenLabel = field("Token", "token", "password", app.configured[app.prefix + "_TOKEN"], "Paste the token");
    tokenLabel.title = app.tokenHint;
    if (app.needsHome) field("Home id", "homeId", "text", app.configured[app.prefix + "_HOME_ID"], "From the app's address bar or settings");

    const pageLabel = document.createElement("label");
    pageLabel.textContent = "Add to page";
    const pageSelect = document.createElement("select");
    pageSelect.className = "settings-input";
    pageSelect.name = "page";
    pageSelect.append(...appsPages.map((p) => option(String(p.index), p.title)));
    pageLabel.append(pageSelect);
    const colLabel = document.createElement("label");
    colLabel.textContent = "Column";
    const colSelect = document.createElement("select");
    colSelect.className = "settings-input";
    colSelect.name = "column";
    colLabel.append(colSelect);
    const fillColumns = () => {
        const page = appsPages.find((p) => String(p.index) === pageSelect.value);
        colSelect.replaceChildren(...(page ? page.columns : []).map((c) => option(String(c.index), `${c.index + 1} (${c.size || "small"})`)));
        const full = page?.columns.find((c) => c.size === "full");
        if (full) colSelect.value = String(full.index);
    };
    pageSelect.addEventListener("change", fillColumns);
    const family = appsPages.find((p) => /family/i.test(p.title)) || appsPages.find((p) => /\bden\b/i.test(p.title));
    if (family) pageSelect.value = String(family.index);
    fillColumns();
    fields.append(pageLabel, colLabel);

    const hint = document.createElement("p");
    hint.className = "settings-note color-subdue settings-gap-top";
    hint.textContent = app.tokenHint;

    const result = document.createElement("p");
    result.className = "settings-app-result";
    result.setAttribute("role", "status");

    const actions = document.createElement("div");
    actions.className = "settings-actions settings-gap-top";
    const values = () => ({
        url: form.url.value, token: form.token.value, homeId: form.homeId ? form.homeId.value : "",
        page: Number(pageSelect.value), column: Number(colSelect.value),
    });
    actions.append(button("Test connection", async () => {
        result.textContent = "Testing…";
        result.className = "settings-app-result color-subdue";
        try {
            const r = await call("POST", `/apps/${app.id}/test`, values());
            result.textContent = r.message;
            result.className = "settings-app-result " + (r.ok ? "color-positive" : "color-negative");
        } catch (e) {
            result.textContent = e.message;
            result.className = "settings-app-result color-negative";
        }
    }));
    const add = document.createElement("button");
    add.type = "submit";
    add.className = "settings-button";
    add.textContent = "Add to the dashboard";
    actions.append(add);

    form.append(fields, hint, actions, result);
    form.addEventListener("submit", async (event) => {
        event.preventDefault();
        add.disabled = true;
        try {
            await call("POST", `/apps/${app.id}`, values());
            notify(`Added ${app.name}, reloading…`);
            await waitForConfigReload();
            loadApps();
        } catch (e) {
            notify(e.message, true);
            add.disabled = false;
        }
    });
    return form;
}

async function loadApps() {
    const list = document.getElementById("apps-list");
    try {
        const data = await call("GET", "/apps");
        appsPages = data.pages;
        list.replaceChildren();
        for (const app of data.apps) {
            const card = document.createElement("article");
            card.className = "settings-app";
            const head = document.createElement("div");
            head.className = "settings-app-head";
            const icon = document.createElement("span");
            icon.className = "settings-app-icon";
            icon.setAttribute("aria-hidden", "true");
            icon.textContent = app.icon;
            const text = document.createElement("div");
            const name = document.createElement("div");
            name.className = "settings-app-name color-highlight";
            name.textContent = app.name;
            const desc = document.createElement("div");
            desc.className = "color-subdue";
            desc.textContent = app.description;
            text.append(name, desc);
            const status = document.createElement("span");
            status.className = "settings-app-status " + (app.onPages.length ? "color-positive" : "color-subdue");
            status.textContent = app.onPages.length ? "On " + app.onPages.join(", ") : "Not added";
            head.append(icon, text, status);
            card.append(head);

            let form = null;
            const toggle = button(app.onPages.length ? "Add again or update" : "Connect", () => {
                if (form) {
                    form.remove();
                    form = null;
                    return;
                }
                form = appForm(app);
                card.append(form);
                form.url.focus();
            });
            toggle.classList.add("settings-gap-top");
            card.append(toggle);
            list.append(card);
        }
    } catch (e) {
        notify("Could not load apps: " + e.message, true);
    }
}

const serverForm = document.getElementById("server-form");
let serverPort = 0;

async function loadServer() {
    try {
        const data = await call("GET", "/server");
        const s = data.settings;
        serverPort = s.port;
        serverForm.port.value = s.port;
        serverForm.proxied.checked = s.proxied;
        serverForm.allowEditing.checked = s.allowEditing;
        serverForm.disablePicker.checked = s.disablePicker;
    } catch (e) {
        notify("Could not load server settings: " + e.message, true);
    }
}

serverForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const f = serverForm;
    const port = Number(f.port.value);
    if (port !== serverPort && !confirm(`Move DashDen to port ${port}? This page will try to follow it.`)) return;
    try {
        await call("PUT", "/server", {
            port, proxied: f.proxied.checked, allowEditing: f.allowEditing.checked, disablePicker: f.disablePicker.checked,
        });
        if (port !== serverPort && location.port === String(serverPort)) {
            notify(`Saved. Moving to port ${port}…`);
            setTimeout(() => { location.port = String(port); }, 2500);
            return;
        }
        notify("Saved server settings, reloading…");
        await waitForConfigReload();
        loadServer();
    } catch (e) {
        notify(e.message, true);
    }
});

function timeAgo(unixSeconds) {
    if (!unixSeconds) return "Never";
    const days = Math.floor((Date.now() / 1000 - unixSeconds) / 86400);
    if (days <= 0) return "Today";
    if (days === 1) return "Yesterday";
    if (days < 60) return `${days} days ago`;
    return `${Math.round(days / 30)} months ago`;
}

async function loadPageUse() {
    const card = document.getElementById("page-use-card");
    try {
        const report = await call("GET", "/page-use");
        const tbody = card.querySelector("tbody");
        tbody.replaceChildren();
        const staleAfter = Date.now() / 1000 - 30 * 86400;
        const pages = [...report.pages].sort((a, b) => a.views - b.views);
        let stale = 0;
        for (const p of pages) {
            const tr = document.createElement("tr");
            const isStale = report.countingSince && report.countingSince < staleAfter && p.lastView < staleAfter;
            if (isStale) stale++;
            tr.append(cell(p.name), cell(String(p.views)), cell(timeAgo(p.lastView), isStale ? "color-negative" : ""));
            tbody.append(tr);
        }
        const since = report.countingSince ? new Date(report.countingSince * 1000).toLocaleDateString() : "now";
        document.getElementById("page-use-note").textContent = stale
            ? `Counting since ${since}. ${stale} page${stale === 1 ? " hasn't" : "s haven't"} been opened in 30 days, so ${stale === 1 ? "it's a candidate" : "they're candidates"} to remove.`
            : `Counting since ${since}. Pages are counted when someone opens them, not who.`;
        card.hidden = false;
    } catch (e) {
        card.hidden = true;
    }
}

function renderHealth(o) {
    const card = document.getElementById("health-card");
    const body = document.getElementById("health-body");
    const h = o.health || { warnings: [], widgets: [] };
    const sections = [];

    if (o.configError) {
        sections.push(["Your latest config change wasn't applied. DashDen is still running the version before it.", [o.configError], "color-negative"]);
    }
    if (h.warnings.length) {
        sections.push([`${h.warnings.length} setting${h.warnings.length === 1 ? " is" : "s are"} ignored because DashDen doesn't recognise ${h.warnings.length === 1 ? "it" : "them"}:`, h.warnings]);
    }
    if (h.widgets.length) {
        sections.push([`${h.widgets.length} widget${h.widgets.length === 1 ? "" : "s"} couldn't load ${h.widgets.length === 1 ? "its" : "their"} data last time:`,
            h.widgets.map(w => `${w.title || w.type} (${w.type}): ${w.error}`)]);
    }
    if (o.missingSecrets) {
        sections.push([`${o.missingSecrets} secret${o.missingSecrets === 1 ? " is" : "s are"} used by the config but not set. See the Secrets tab.`, []]);
    }

    body.replaceChildren();
    card.hidden = false;
    if (!sections.length) {
        const p = document.createElement("p");
        p.className = "color-positive";
        p.textContent = "Everything loads and every setting is recognised.";
        body.append(p);
        return;
    }
    for (const [title, items, cls] of sections) {
        const p = document.createElement("p");
        p.textContent = title;
        if (cls) p.className = cls;
        body.append(p);
        if (!items.length) continue;
        const ul = document.createElement("ul");
        ul.className = "settings-health-list";
        for (const item of items) {
            const li = document.createElement("li");
            li.textContent = item;
            ul.append(li);
        }
        body.append(ul);
    }
}

async function loadOverview() {
    const facts = document.getElementById("overview-facts");
    try {
        const o = await call("GET", "/overview");
        renderSetup(o);
        renderHealth(o);
        loadPageUse();
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
    settings: "In config/.env",
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

const loaders = { overview: loadOverview, people: loadPeople, signin: () => { loadSignIn(); loadOIDC(); }, access: loadAccess, apps: loadApps, themes: loadThemes, secrets: loadSecrets, history: loadHistory, server: loadServer };

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
