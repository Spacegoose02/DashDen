# Settings

The gear in the header opens Settings, the part of DashDen for everything around the dashboard. Pages, widgets, themes and branding are still edited on the dashboard itself with the pencil button (see [UI Editor](ui-editor.md)). Everything is stored in your YAML files, so hand edits keep working.

## Who can open it

If any [household](household.md) member has `role: admin`, only admins can open Settings. Otherwise, the same people who can use the editor can (`server.allow-editing`, `editing-users`, `editing-groups`). `ENABLE_EDITOR=false` turns Settings off too.

## Overview

This tab covers the sign-in methods in use, the household, the number of pages, how many secrets the config uses without a value, how many saved versions there are, and where the config file lives.

## People

Add, edit and remove [household](household.md) members: username, name, avatar (an emoji, initials or an image URL), role, home page, theme and extra groups. The username has to match what the person signs in with.

If password sign-in is in use, you can also set a person's password here. Only a bcrypt hash is written to the config (`auth.users.<username>.password-hash`). If the config has no `auth.secret-key` yet, one is generated, saved as `DASHDEN_AUTH_SECRET` in the secrets file, and referred to as `${DASHDEN_AUTH_SECRET}`.

To stop you locking yourself out, Settings won't let you remove yourself or take admin away from yourself.

## Sign-in

Turn [forward-auth sign-in](authentication.md) (Tinyauth, Authelia, Authentik and others) on or off, and set the trusted proxy addresses, the sign-in and sign-out pages, which users or groups may come in, and the header names if your proxy uses different ones. The tab also shows the address your own request came from, which is the one to trust.

Settings refuses a change that would sign you out: turning the proxy off when it's the only way in, leaving yourself out of the allowed users and groups, or dropping the address your request came through from the trusted proxies.

## Access

Who can see each page, as users and groups (`allowed-users` and `allowed-groups` on the page). Groups include each person's role, so `adult` covers adults and admins. Leaving both empty means everyone who can sign in. Settings won't let you hide a page from yourself. Pages that live in an included file are shown, but you change them in that file.

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

Values saved here go into `dashden-secrets.env` next to your main config file, with `0600` permissions. That file is loaded at start-up after `--env-file`, so a value saved here wins over an older one from your `.env`. Values are never sent back to the browser. Saving or removing a secret reloads the dashboard so widgets pick it up straight away.

Add `dashden-secrets.env` to your config repository's `.gitignore`.

## History

Every save from the editor or from Settings first copies the files it's about to change into `.dashden-history/` next to your config. The History tab lists those saves, newest first. **Restore** puts the files back as they were just before that save. A restore is checked like any other save, and it is itself saved to history, so you can undo it. The newest 50 saves are kept.
