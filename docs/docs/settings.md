# Settings

The gear in the header opens Settings, the part of DashDen for everything around the dashboard. Pages, widgets, themes and branding are still edited on the dashboard itself with the pencil button (see [UI Editor](ui-editor.md)). Everything is stored in your YAML files, so hand edits keep working.

## Who can open it

If any [household](household.md) member has `role: admin`, only admins can open Settings. Otherwise, the same people who can use the editor can (`server.allow-editing`, `editing-users`, `editing-groups`). `ENABLE_EDITOR=false` turns Settings off too.

## Overview

This tab covers the sign-in methods in use, the household, the number of pages, how many secrets the config uses without a value, how many saved versions there are, and where the config file lives.

## Getting started

Until the basics are done, the Overview tab starts with a short checklist: make yourself a household admin (one click if you're signed in and not in the household yet), set up sign-in, connect your apps, and fill in any missing secrets. It disappears once everything is ticked.

**Config health** lists settings DashDen ignores (with file, line and what you probably meant), widgets whose last update failed, a config change that didn't load, and secrets that aren't set.

**Page use** counts how often each page is opened (not by whom) and when it was last opened. Pages nobody has opened in 30 days are marked as candidates to remove.

## People

Add, edit and remove [household](household.md) members: username, name, avatar (an emoji, initials or an image URL), role, home page, theme and extra groups. The username has to match what the person signs in with.

If password sign-in is in use, you can also set a person's password here. Only a bcrypt hash is written to the config (`auth.users.<username>.password-hash`). If the config has no `auth.secret-key` yet, one is generated, saved as `DASHDEN_AUTH_SECRET` in `config/.env`, and referred to as `${DASHDEN_AUTH_SECRET}`.

To stop you locking yourself out, Settings won't let you remove yourself or take admin away from yourself.

## Sign-in

Turn [forward-auth sign-in](authentication.md) (Tinyauth, Authelia, Authentik and others) on or off, and set the trusted proxy addresses, the sign-in and sign-out pages, which users or groups may come in, and the header names if your proxy uses different ones. The tab also shows the address your own request came from, which is the one to trust.

Settings refuses a change that would sign you out: turning the proxy off when it's the only way in, leaving yourself out of the allowed users and groups, or dropping the address your request came through from the trusted proxies.

## Access

Who can see each page, as users and groups (`allowed-users` and `allowed-groups` on the page). Groups include each person's role, so `adult` covers adults and admins. Leaving both empty means everyone who can sign in. Settings won't let you hide a page from yourself. Pages that live in an included file are shown, but you change them in that file.

## Apps

Connect the household's own apps and add their widgets without writing YAML:

| App | Adds |
| --- | ---- |
| Den Duty | A `den-app` chores widget with Done and Approve buttons (approval limited to `adult` when you have a household) |
| Nighttime Ninja | A `den-app` shopping list |
| Wavelength | A `den-app` music controller |
| LootDen, BingeBandit | A source in the column's "Now in the Den" (`den-now`) widget, creating it if needed |

Fill in the address, token and, where needed, the home id. **Test connection** checks them against the app before anything is saved. **Add to the dashboard** saves them as secrets named `DENDUTY_URL`, `DENDUTY_TOKEN`, `DENDUTY_HOME_ID` (and likewise `NINJA_`, `WAVELENGTH_`, `LOOTDEN_`, `BINGEBANDIT_`), then adds the widget to the page and column you picked, referring to them as `${NAME}`. Fields that are already set, for example from your `.env`, can be left empty.

## Server

The port, whether DashDen is behind a reverse proxy, whether the dashboard can be edited from the browser, and whether the theme picker is shown. Changing the port moves DashDen; the page follows it when you opened it on that port directly. If there's no household admin, Settings won't let you turn off editing, because that would close Settings too.

## Recovery

If a change locks everyone out, for example a proxy that no longer lets anyone through, set `DASHDEN_RECOVERY` on the container to a code of at least 16 characters and restart it. Then open `/settings/recover` and type the code. That browser can use Settings for an hour without signing in, and a banner says it's in recovery mode. Remove the variable once you're back in; changing the code ends every recovery session.

```yaml
services:
  dashden:
    environment:
      - DASHDEN_RECOVERY=pick-a-long-random-code
```

With the variable set, Settings is available even when `ENABLE_EDITOR=false`.

## Secrets

API keys and passwords your config reads as `${NAME}` (or `${env:NAME}`). The list shows every name the config uses and where its value comes from:

| Shown as | Meaning |
| -------- | ------- |
| Saved here | Set from Settings |
| Container or env file | Comes from Docker's `env_file`, the container environment or `--env-file` |
| Missing | The config uses it but nothing sets it |

Secrets live in the `.env` file next to your main config file (`config/.env` in Docker). DashDen loads it at start-up, so you don't need to pass it to the container, and its values win over the container's own environment. Saving or removing a secret here edits just that line of the file and keeps your comments and order. A new file is created with `0600` permissions. Values are never sent back to the browser, and saving or removing a secret reloads the dashboard so widgets pick it up straight away.

Secrets saved by earlier DashDen builds in `dashden-secrets.env` are still read, and are moved into `.env` the next time you save one. Keep `.env` out of your config repository with `.gitignore`.

## History

Every save from the editor or from Settings first copies the files it's about to change into `.dashden-history/` next to your config. The History tab lists those saves, newest first. **Restore** puts the files back as they were just before that save. A restore is checked like any other save, and it is itself saved to history, so you can undo it. The newest 50 saves are kept.

In the editor, each save also shows **Undo** for ten seconds. It puts back what that one change touched, as long as nobody has saved anything since. After that, use the History tab.
