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
| `theme` | The theme preset they start on |
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
