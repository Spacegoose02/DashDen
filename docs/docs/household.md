# Household

The `household` block gives each person a profile: a display name, an avatar, a role, the page they land on and a default theme. People are matched by the username their sign-in reports, so with [Tinyauth forward auth](authentication.md#forward-auth-proxy-tinyauth-authelia-authentik) it's their Tinyauth username.

```yaml
household:
  members:
    mom:
      name: Mom
      avatar: /assets/avatars/mom.png
      role: admin
      home: mom
    ava:
      name: Ava
      avatar: "🦊"
      role: kid
      home: ava
      theme: pink-matte
      groups: [gamers]
```

| Option | What it does |
| ------ | ------------ |
| `name` | Shown as "Hi, Ava" in the header. Wins over the name the sign-in method sends |
| `avatar` | An image path or URL, or a short text such as an emoji or initials |
| `role` | `admin`, `adult`, `kid` or `guest`. The role is added to the person's groups, and `admin` also counts as `adult` |
| `home` | Slug of the page opened when they visit the dashboard's root address |
| `theme` | The theme they start on, a preset or a [built-in theme](themes.md#built-in-themes). Kids without one start on Playroom. Once they pick a theme in the theme picker, their choice is saved for them and follows them to every device |
| `groups` | Extra groups, added to the ones the sign-in method sends |
| `linked-accounts` | Other sign-in names or email addresses that sign in as this person. See [Linked accounts](#linked-accounts) |

Roles and groups work with `allowed-groups` on pages and with `editing-groups`, so this works even when the sign-in method has no groups of its own, such as Tinyauth's local users:

```yaml
pages:
  - name: Ava
    allowed-users: [ava]
  - name: Grown-ups
    allowed-groups: [adult]
server:
  editing-groups: [admin]
```

DashDen refuses to start if a `home` isn't a page slug, a `theme` isn't a preset or built-in theme, or a `role` isn't one of the four above. People who sign in but aren't listed still work as before, with no profile.

## Linked accounts

When someone's Tinyauth username isn't the same as their DashDen username, or one person has two accounts, link them. Signing in as any linked name (or, for Tinyauth and OIDC, with a linked email address) signs in as that person, with their role, pages, theme and saved data:

```yaml
household:
  members:
    mom:
      role: admin
      linked-accounts: [jane, jane@example.com, mom-old]
```

Links work for every sign-in method, so the example also signs the local account `mom-old` in as `mom`. Names ignore capital letters. A name can be linked to only one person, and not to someone who has a profile of their own.

There are two ways to set them up without editing the config:

- **Yourself, from Your account.** Click your name in the header. If you're signed in through Tinyauth (or OIDC) as someone without a profile, the page offers to link it: type your DashDen username and password once, and from then on Tinyauth signs you in as that account. The same page lists your links and can remove them. This needs DashDen's own password sign-in; without it, ask an admin.
- **Admins, from Settings, People.** Edit a person and fill in *Linked accounts*. People who recently signed in through Tinyauth or OIDC without a profile are listed under *Signed in without a profile*, each with a *Link to…* menu. Linking a name that has its own profile, such as folding someone's second account into an admin account, asks first and then replaces that profile. Their password, if they have one, keeps working and signs them in as the person it's linked to.

## Themes that follow the person

When anyone signs in, the theme they pick is stored in DashDen's database (`server.db-path`, by default `dynacat.db` in the assets folder) under their username. On any device they sign in from, DashDen uses, in order: their saved choice, their household `theme` (Playroom for kids without one), then the browser's own choice. Visitors who aren't signed in keep the per-browser behaviour.

## Kiosk mode

For a wall tablet or an old laptop in the kitchen. Open any page with `?kiosk=1` and DashDen hides the header, navigation and footer, keeps the screen awake where the browser allows it, and hides the cursor when nobody is using it. The choice is remembered in that browser, so the tablet stays in kiosk mode after a restart.

- `?kiosk=1&rotate=60` also moves to the next page every 60 seconds. Rotation waits while someone is touching the screen, and moves on once the screen has been left alone for that long. Use 10 seconds or more.
- To leave kiosk mode, double-tap the top-right corner of the screen, or open the page with `?kiosk=0`.

Kiosk mode only changes what that one browser shows. It doesn't sign anyone in, so pair it with a household member such as `kitchen` whose `allowed-groups` rules decide what the tablet can see.

## A page for kids

Kids get bigger text and rounder, brighter tiles from the kid themes (Playroom, Rainbow, Dino, Under the Sea, Space Explorer and Unicorn), and start on Playroom unless you set their `theme`. A bento page with big tiles and only the things they use keeps it simple:

```yaml
household:
  members:
    ava:
      name: Ava
      avatar: "🦊"
      role: kid
      home: ava
      theme: unicorn

pages:
  - name: Ava
    allowed-groups: [kid, adult]
    layout: bento
    columns:
      - size: full
        widgets:
          - type: clock
            tile: wide
          - type: den-app
            app: den-duty
            url: https://chores.example.com
            token: ${DENDUTY_TOKEN}
            home-id: ${DENDUTY_HOME_ID}
            title: My chores
            kid: Ava
            tile: big
          - type: to-do
            title: My list
            id: ava
            storage: server
            per-person: true
          - type: notice-board
            id: family
            tile: wide
          - type: calendar
```

Kids see only the pages their role or username is allowed on, so grown-up pages such as Docker or finances stay out of sight.
