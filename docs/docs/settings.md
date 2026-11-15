# Settings

The gear in the header opens Settings, the part of DashDen for everything around the dashboard. Pages, widgets, themes and branding are still edited on the dashboard itself with the pencil button (see [UI Editor](ui-editor.md)). Everything is stored in your YAML files, so hand edits keep working.

## Who can open it

If any [household](household.md) member has `role: admin`, only admins can open Settings. Otherwise, the same people who can use the editor can (`server.allow-editing`, `editing-users`, `editing-groups`). `ENABLE_EDITOR=false` turns Settings off too.

## Overview

This tab covers the sign-in methods in use, the household, the number of pages, how many secrets the config uses without a value, how many saved versions there are, and where the config file lives.

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
