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
| `theme` | The theme preset they start on. Once they pick a theme in the theme picker, their choice is saved for them and follows them to every device |
| `groups` | Extra groups, added to the ones the sign-in method sends |

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

DashDen refuses to start if a `home` isn't a page slug, a `theme` isn't a preset, or a `role` isn't one of the four above. People who sign in but aren't listed still work as before, with no profile.

## Themes that follow the person

When anyone signs in, the theme they pick is stored in DashDen's database (`server.db-path`, by default `dynacat.db` in the assets folder) under their username. On any device they sign in from, DashDen uses, in order: their saved choice, their household `theme`, then the browser's own choice. Visitors who aren't signed in keep the per-browser behaviour.

## Kiosk mode

For a wall tablet or an old laptop in the kitchen. Open any page with `?kiosk=1` and DashDen hides the header, navigation and footer, keeps the screen awake where the browser allows it, and hides the cursor when nobody is using it. The choice is remembered in that browser, so the tablet stays in kiosk mode after a restart.

- `?kiosk=1&rotate=60` also moves to the next page every 60 seconds. Rotation waits while someone is touching the screen, and moves on once the screen has been left alone for that long. Use 10 seconds or more.
- To leave kiosk mode, double-tap the top-right corner of the screen, or open the page with `?kiosk=0`.

Kiosk mode only changes what that one browser shows. It doesn't sign anyone in, so pair it with a household member such as `kitchen` whose `allowed-groups` rules decide what the tablet can see.
