<p align="center"><img width="250px" src="docs/dashden-logo.png"></p>
<h1 align="center">DashDen</h1>
<p align="center">
  <a href="docs/docs/getting-started.md">Getting started</a> •
  <a href="docs/docs/configuration.md">Configuration</a> •
  <a href="docs/docs/settings.md">Settings</a> •
  <a href="docs/docs/themes.md">Themes</a> •
  <a href="https://github.com/Panonim/dynawidgets">Dynawidgets</a>
</p>

<p align="center">Self-hosted dashboard built for people who want their information in one place.<br>DashDen is a personal fork of <a href="https://github.com/Panonim/dynacat">Dynacat</a> by Panonim, itself forked from Glance. It focuses on dynamic content updates and seamless integration with external applications.</p>

![](docs/docs/images/readme-main-image.png)

## Features List
### Various widgets
* RSS feeds
* Subreddit posts
* Hacker News posts
* Weather forecasts
* YouTube channel uploads
* Twitch channels
* Market prices
* Docker containers status
* Server stats
* Custom widgets
* [and many more...](docs/docs/configuration.md)

### Fast and lightweight
* Low memory usage
* Few dependencies
* Minimal vanilla JS
* Single <20mb binary available for multiple OSs & architectures and just as small Docker container
* Uncached pages usually load within ~1s (depending on internet speed and number of widgets)

### Tons of customizability
* Different layouts
* As many pages/tabs as you need
* Numerous configuration options for each widget
* Multiple styles for some widgets
* Custom CSS

### Optimized for mobile devices
Because you'll want to take it with you on the go.

![](docs/docs/images/mobile-preview.png)

### Themeable
Easily create your own theme by tweaking a few numbers or choose from one of the [already available themes](docs/docs/themes.md).

![](docs/docs/images/themes-example.png)

<br>

### Other features worth mentioning
* **UI editor** - add pages, drag widgets between columns and change their options from the dashboard itself, DashDen writes it back into your YAML
* **Widget editor** - a visual builder for `custom-api` widgets, point it at an API and bind blocks to fields instead of writing the template by hand
* **Dynawidgets** - ready-made widgets from a community repository, fetched and cached from a single slug
* **JSON API** - read the data your widgets already fetched from scripts or other apps
* **Dynamic updates** - widgets refresh on their own, no manual reload
* **OIDC** - log in through your own identity provider
* **More widgets** - docker controller, currently playing, latest media, torrenting, speedtest, stopwatch and Sonarr/Radarr releases in the calendar

<br>

## Running DashDen on Unraid

Every push to `main` publishes `ghcr.io/spacegoose02/dashden:main`. Your files live in `/mnt/user/appdata/dashden/`:

```
/mnt/user/appdata/dashden/
  config/dynacat.yml   # your config, plus any included files
  config/.env          # your secrets, referenced in the config as ${NAME}
  assets/              # images, custom CSS, and the database DashDen creates
```

DashDen reads `config/.env` itself at start-up, so you don't enter any variables in Unraid. Secrets you add or change in Settings > Secrets are saved to the same file, and those values win over anything the container was started with. Settings like `ENABLE_EDITOR=true` and `DASHDEN_RECOVERY` can go in `.env` too.

**Docker template.** Copy [`unraid/dashden.xml`](unraid/dashden.xml) to `/boot/config/plugins/dockerMan/templates-user/my-DashDen.xml`, then pick DashDen under Docker > Add Container > Template. It only asks for the port and the two folders.

**Compose Manager.** Or add a stack called `dashden` in the Compose Manager plugin and paste [`unraid/docker-compose.yml`](unraid/docker-compose.yml).

Either way, put DashDen behind Tinyauth in your reverse proxy (see [Authentication](docs/docs/authentication.md)), then open Settings to finish setting up people and apps. If the `:main` image is private, make the package public on GitHub or run `docker login ghcr.io` on Unraid first.

## Other ways to run it

Outside Unraid, use [`docker-compose.dashden.yml`](docker-compose.dashden.yml): it runs the same image with the same `config/` and `assets/` folders.

## Configuration
There are two ways to configure DashDen, and they work on the same files:

* **UI editor** - drag widgets into columns, change their options and pick your colors from the dashboard itself. DashDen writes the result back into your YAML. See the [UI editor documentation](docs/docs/ui-editor.md).
* **YAML by hand** - edit the config files directly. To learn more about how the layout works, how to add more pages and how to configure widgets, see the [configuration documentation](docs/docs/configuration.md). [Getting started](docs/docs/getting-started.md) covers splitting the config into one file per page and sharing widget files.

The editor is an addition to the config file, not a replacement, so hand-written config keeps working exactly as before.

<details>
<summary><strong>Preview example configuration file</strong></summary>
<br>

```yaml
  - name: Home
    columns:
      - size: small
        widgets:
          - type: calendar
            first-day-of-week: monday

          - type: rss
            limit: 10
            collapse-after: 3
            cache: 12h
            feeds:
              - url: https://selfh.st/rss/
                title: selfh.st
                limit: 4
              - url: https://ciechanow.ski/atom.xml
              - url: https://www.joshwcomeau.com/rss.xml
                title: Josh Comeau
              - url: https://samwho.dev/rss.xml
              - url: https://ishadeed.com/feed.xml
                title: Ahmad Shadeed

          - type: twitch-channels
            channels:
              - theprimeagen
              - j_blow
              - piratesoftware
              - cohhcarnage
              - christitustech
              - EJ_SA

      - size: full
        widgets:
          - type: group
            widgets:
              - type: hacker-news
              - type: lobsters

          - type: videos
            channels:
              - UCXuqSBlHAE6Xw-yeJA0Tunw # Linus Tech Tips
              - UCR-DXc1voovS8nhAvccRZhg # Jeff Geerling
              - UCsBjURrPoezykLs9EqgamOA # Fireship
              - UCBJycsmduvYEL83R_U4JriQ # Marques Brownlee
              - UCHnyfMqiRRG1u-2MsSQLbXA # Veritasium

          - type: group
            widgets:
              - type: reddit
                subreddit: technology
                show-thumbnails: true
              - type: reddit
                subreddit: selfhosted
                show-thumbnails: true

      - size: small
        widgets:
          - type: weather
            location: London, United Kingdom
            units: metric
            hour-format: 12h

          - type: markets
            markets:
              - symbol: SPY
                name: S&P 500
              - symbol: BTC-USD
                name: Bitcoin
              - symbol: NVDA
                name: NVIDIA
              - symbol: AAPL
                name: Apple
              - symbol: MSFT
                name: Microsoft

          - type: releases
            cache: 1d
            repositories:
              - panonim/dynacat
              - go-gitea/gitea
              - immich-app/immich
              - syncthing/syncthing
```
</details>

## Common issues
<details>
<summary><strong>A setting doesn't seem to do anything</strong></summary>

DashDen ignores settings it doesn't recognise, but it tells you: there's a banner at the top of the dashboard for admins, a Config health card in Settings, and `docker exec dashden dashden check` lists every problem with its file and line.
</details>

<details>
<summary><strong>The dashboard says it can't load its config</strong></summary>

The page shows the error with its file and line. Fix it and save; the page turns into the dashboard on its own within a few seconds. A broken edit while DashDen is running never takes it down: it keeps the last working version and the banner says why the new one didn't load.
</details>

<details>
<summary><strong>Requests timing out</strong></summary>

The most common cause of this is when using Pi-Hole, AdGuard Home or other ad-blocking DNS services, which by default have a fairly low rate limit. Depending on the number of widgets you have in a single page, this limit can very easily be exceeded. To fix this, increase the rate limit in the settings of your DNS service.

If using Podman, in some rare cases the timeout can be caused by an unknown issue, in which case it may be resolved by adding the following to the bottom of your `docker-compose.yml` file:
```yaml
networks:
  podman:
    external: true
```
</details>

<details>
<summary><strong>Broken layout for markets, bookmarks or other widgets</strong></summary>

This is almost always caused by the browser extension Dark Reader. To fix this, disable dark mode for the domain where DashDen is hosted.
</details>

<details>
<summary><strong>cannot unmarshal !!map into []dynacat.page</strong></summary>

The most common cause of this is having a `pages` key in your `dynacat.yml` and then also having a `pages` key inside one of your included pages. To fix this, remove the `pages` key from the top of your included pages.

</details>

<details>
<summary><strong>Cannot embed DashDen in an iframe</strong></summary>

By default DashDen only allows itself to be embedded on the same origin (`frame-ancestors 'self'`). To allow embedding from another host such as Homepage, add the `allowed-embed-hosts` option under `server` in your `dynacat.yml`:

```yaml
server:
  allowed-embed-hosts:
    - https://homepage.mydomain.com
```

You can list multiple origins. Each entry must be a full origin including the scheme (e.g. `https://`).

</details>
