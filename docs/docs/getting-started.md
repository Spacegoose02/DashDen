# Getting started with DashDen

This is the short path from a fresh install to a dashboard the whole family uses. The [README](../../README.md) covers installing on Unraid.

## 1. Make yourself the admin

Open **Settings > People**, add yourself with the username you sign in with, and pick the **admin** role. Once there's an admin, only admins can open Settings. Usernames match without regard to case.

## 2. Turn on sign-in

DashDen works best behind Tinyauth or another forward-auth proxy, so everyone is signed in by the time they reach it. **Settings > Sign-in** walks you through it, and [Authentication](authentication.md) has the details. Then add the rest of the household in **Settings > People** and give each person a home page.

## 3. Add secrets

API keys and passwords go in **Settings > Secrets** (they're saved to `config/.env`) and are used in the config as `${NAME}`. Never paste a key straight into a widget. Settings shows any secret the config uses that isn't set.

## 4. Lay out the config

A config that grows past a few hundred lines is easier to live with split up:

```
config/
  dynacat.yml           # branding, theme, server, auth, and the list of pages
  pages/home.yml        # one file per page
  pages/news.yml
  widgets/weather.yml   # widgets used on more than one page
  widgets/shared/subreddit.yml
  .env                  # secrets, never committed
  .env.example          # the names of every secret, safe to commit
```

`dynacat.yml` lists the pages:

```yaml
pages:
  - $include: pages/home.yml
  - $include: pages/news.yml
```

and each page file starts with its own `- name:`. Paths in an include are relative to the file they're in, so a page pulls in widgets with `../widgets/...`.

When several widgets differ only in a title, a feed or an ID, write one file and pass the values with `with:`. See [Reusing a widget file with different values](configuration.md#reusing-a-widget-file-with-different-values).

## 5. Check it

```sh
docker exec dashden dashden check
```

This lists every setting DashDen ignores, with its file and line and what you probably meant, and every `${VARIABLE}` that isn't set. The same problems show in a banner on the dashboard for admins, and in **Settings > Overview > Config health**.

If a change breaks the config, DashDen keeps showing the last version that worked. If the config is broken when DashDen starts, the dashboard's address shows what's wrong until you fix it.

## 6. Put it on the wall

Open any page with `?kiosk=1` on a wall tablet for a full-screen view without the header. See [Household](household.md).
