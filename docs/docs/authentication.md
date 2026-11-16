# Authentication

Dynacat supports two authentication methods: username/password and OIDC (Single Sign-On). Both methods can be active at the same time.

## Password Authentication

Protect your dashboard with username and password login. Configure via the top-level `auth` property:

```yaml
auth:
  users:
    admin:
      password: mysecretpassword
    alice:
      password: anotherpassword
```

The easiest way is to skip YAML altogether: in **Settings > People**, set a password for someone and DashDen writes a bcrypt hash for you. Things DashDen handles for you:

- **Usernames ignore capital letters.** `Underwood02` can sign in as `underwood02`. Two users whose names differ only in case are refused when the config loads.
- **Passwords need at least 8 characters.**
- **Stay signed in.** Tick it on the sign-in page and that device stays signed in for 90 days after its last visit, renewed as it's used, so a wall tablet never has to sign in again. Without it, the session ends when the browser closes.
- **`secret-key` is optional.** Leave it out and DashDen makes one and saves it to `config/.env` as `DASHDEN_AUTH_SECRET`, so sessions survive restarts. If you do set it, any random text of 16 or more characters works; upstream's 64-byte base64 keys keep working.
- **Changes apply without a restart.** Editing the config or `config/.env` reloads DashDen, and a reload also clears the "too many tries" pause. After 5 wrong tries from one device, the sign-in page counts down the 5 minute pause.
- **Broken changes are reported.** If the newest config has an error (a second `auth:` block, say), DashDen keeps running the previous one and says so on the sign-in page and in Settings, with the details in the container log.

To generate a secret key yourself:

```sh
./dynacat secret:make
```

Or with Docker:

```sh
docker run --rm panonim/dynacat secret:make
```

### Hashed passwords

Avoid storing plain passwords in your config by hashing them first:

```sh
./dynacat password:hash mysecretpassword
```

Or with Docker:

```sh
docker run --rm panonim/dynacat password:hash mysecretpassword
```

Then use `password-hash` instead of `password`:

```yaml
auth:
  secret-key: ...
  users:
    admin:
      password-hash: $2a$10$o6SXqiccI3DDP2dN4ADumuOeIHET6Q4bUMYZD6rT2Aqt6XQ3DyO.6
```

### Brute-force protection

Dynacat automatically blocks IPs that fail to authenticate 5 times within 5 minutes. For this to work correctly behind a reverse proxy, set:

```yaml
server:
  proxied: true
```

This tells Dynacat to read the real client IP from the `X-Forwarded-For` header.

---

## OIDC Authentication

Integrate with any OpenID Connect identity provider (Authentik, Authelia, PocketID, etc.) using the OAuth2 Authorization Code flow with PKCE.

### Basic configuration

```yaml
auth:
  secret-key: # generate with: ./dynacat secret:make
  oidc:
    issuer-url: https://auth.example.com
    client-id: dynacat
    client-secret: ${secret:oidc_client_secret}
    redirect-url: https://dashboard.example.com/api/oidc/callback
```

### Full OIDC options

```yaml
auth:
  secret-key: ...
  oidc:
    issuer-url: https://auth.example.com       # required: OpenID Connect issuer URL
    client-id: dynacat                          # required
    client-secret: ${OIDC_CLIENT_SECRET}        # required
    redirect-url: https://dashboard.example.com/api/oidc/callback  # required
    scopes:                                     # optional, defaults shown
      - openid
      - profile
      - email
    username-claim: preferred_username          # optional, default: preferred_username
    groups-claim: groups                        # optional, default: groups
    allowed-groups:                             # optional: restrict OIDC login to these IdP groups
      - homelab-users
    allowed-users:                              # optional: restrict OIDC login to these usernames
      - alice
```

The `redirect-url` must match the callback URL configured in your identity provider. The path must end with `/api/oidc/callback`.

### Disabling password auth (OIDC-only mode)

When OIDC is working and you want to remove the password login option entirely:

```yaml
auth:
  secret-key: ...
  disable-password: true
  oidc:
    issuer-url: https://auth.example.com
    client-id: dynacat
    client-secret: ${OIDC_CLIENT_SECRET}
    redirect-url: https://dashboard.example.com/api/oidc/callback
```

### Skipping the login page

With `disable-password: true` set, you can also skip the Dynacat login page and go straight to your identity provider:

```yaml
auth:
  secret-key: ...
  disable-password: true
  auto-redirect-oidc: true
  oidc:
    ...
```

If the provider returns an error, the login page is shown instead so you can see what went wrong.

Logging out also ends your session at the provider, if it supports [RP-initiated logout](https://openid.net/specs/openid-connect-rpinitiated-1_0.html) (it advertises an `end_session_endpoint`), and then brings you back to the Dynacat login page instead of redirecting again. For this to work, add the following as an allowed post-logout redirect URI in your provider, using the same origin as your `redirect-url`:

```
https://dashboard.example.com/login?logged-out=1
```

If you use `server.base-url`, include it before `/login`. Providers without an `end_session_endpoint` only get logged out of Dynacat, and you land on the login page.

### Provider guides

#### Generic OIDC

Register a new OAuth2/OIDC application in your identity provider:
- **Redirect URI**: `https://dashboard.example.com/api/oidc/callback`
- **Grant type**: Authorization Code
- **Scopes**: `openid profile email` (add `groups` if your provider supports it)

Then configure Dynacat with the issuer URL, client ID, and client secret from your provider.

#### Authentik

1. In Authentik, go to **Applications → Providers → Create → OAuth2/OpenID Provider**
2. Set **Redirect URIs** to `https://dashboard.example.com/api/oidc/callback`
3. Set **Scopes** to include `openid`, `profile`, `email`, and optionally `groups`
4. Copy the **Client ID** and **Client Secret**
5. Use the provider's OIDC issuer URL, not the bare Authentik instance URL. In Authentik this usually looks like `https://auth.example.com/application/o/<provider-slug>/` and is the URL Dynacat should use for discovery.

```yaml
auth:
  oidc:
    issuer-url: https://auth.example.com/application/o/<provider-slug>/
    client-id: <client-id-from-authentik>
    client-secret: ${OIDC_CLIENT_SECRET}
    redirect-url: https://dashboard.example.com/api/oidc/callback
    scopes:
      - openid
      - profile
      - email
      - groups
    groups-claim: groups
```

#### Authelia

1. In Authelia's configuration, add a new client under `identity_providers.oidc.clients`:

```yaml
identity_providers:
  oidc:
    clients:
      - client_id: dynacat
        client_secret: '$pbkdf2-sha512$...'  # hashed secret
        redirect_uris:
          - https://dashboard.example.com/api/oidc/callback
        scopes:
          - openid
          - profile
          - email
          - groups
        grant_types:
          - authorization_code
```

2. In Dynacat:

```yaml
auth:
  oidc:
    issuer-url: https://auth.example.com
    client-id: dynacat
    client-secret: ${OIDC_CLIENT_SECRET}
    redirect-url: https://dashboard.example.com/api/oidc/callback
    scopes:
      - openid
      - profile
      - email
      - groups
```

#### PocketID

1. In PocketID, create a new OIDC client
2. Set the **Callback URL** to `https://dashboard.example.com/api/oidc/callback`
3. Copy the client ID and secret

```yaml
auth:
  oidc:
    issuer-url: https://pocketid.example.com
    client-id: <client-id>
    client-secret: ${OIDC_CLIENT_SECRET}
    redirect-url: https://dashboard.example.com/api/oidc/callback
```

---

## Forward-Auth Proxy (Tinyauth, Authelia, Authentik)

If a forward-auth proxy already guards your apps, DashDen can trust the identity it passes along instead of running its own login. Nobody sees a login page or a redirect, and there is no DashDen session to lose on restart.

```yaml
server:
  trusted-proxies:
    - 172.18.0.0/16   # the proxy's address or network; required

auth:
  proxy:
    logout-url: https://auth.example.com/logout
```

That is all Tinyauth needs, because the header names default to Tinyauth's. Every option:

| Option | Default | What it does |
| ------ | ------- | ------------ |
| `user-header` | `Remote-User` | Username. A request without it is not signed in |
| `name-header` | `Remote-Name` | Full name, shown in the header instead of the username |
| `email-header` | `Remote-Email` | Email address |
| `groups-header` | `Remote-Groups` | Comma-separated groups, used by `allowed-groups` on pages and editing |
| `login-url` | | Where `/login` sends someone who arrives without the headers |
| `logout-url` | | Where the logout button sends a proxy user, so the proxy's session ends too |
| `allowed-users`, `allowed-groups` | | Only these people may sign in. Empty allows everyone the proxy lets through |

> [!IMPORTANT]
>
> The headers are only believed when the request comes straight from an address in `server.trusted-proxies`, and DashDen refuses to start with `auth.proxy` and no trusted proxies. Still, make sure DashDen's port is not reachable without going through the proxy.

Tinyauth only sends groups for people who sign in through an OAuth provider that has them, or through LDAP. With Tinyauth's own local users, restrict pages with `allowed-users`.

To give each person a name, avatar, role and home page, see [Household](household.md).

`auth.proxy` works alongside password users and OIDC. When it's the only method, `/login` redirects to `login-url` or answers 401.

## Per-Page Access Control

Restrict individual pages to specific users or groups using `allowed-users` and `allowed-groups`:

```yaml
pages:
  - name: Public Status
    # no allowed-users/groups = accessible to all authenticated users
    columns: ...

  - name: Admin Dashboard
    allowed-users:
      - admin
    allowed-groups:
      - devops
    columns: ...
```

A user can access a restricted page if their username appears in `allowed-users` **or** they belong to any group in `allowed-groups`.

Pages without `allowed-users` or `allowed-groups` are accessible to all authenticated users.

### Guest / public access

Set `require-auth: false` to allow unauthenticated access to unrestricted pages:

```yaml
auth:
  require-auth: false
  secret-key: ...
  users:
    admin:
      password-hash: ...
```

With `require-auth: false`:
- Pages with no `allowed-users`/`allowed-groups` are publicly accessible
- Pages with `allowed-users`/`allowed-groups` still require login + correct access
- A login button appears in the navigation bar for unauthenticated visitors

### Combined example: public status + private admin

```yaml
auth:
  secret-key: ${secret:auth_secret}
  require-auth: false
  oidc:
    issuer-url: https://auth.example.com
    client-id: dynacat
    client-secret: ${secret:oidc_secret}
    redirect-url: https://dashboard.example.com/api/oidc/callback
    groups-claim: groups

pages:
  - name: Status
    # public - no restrictions
    columns:
      - size: full
        widgets:
          - type: monitor
            ...

  - name: Admin
    allowed-groups:
      - admins
    columns:
      - size: full
        widgets:
          - type: server-stats
            ...
```

## Editing Access Control

The web UI editor (drag-and-drop page/widget editing) is enabled by default for all users who can access a page. Restrict who is allowed to use it with `editing-users` and `editing-groups`, under the top-level `server` property:

```yaml
server:
  editing-users:
    - admin
  editing-groups:
    - devops
```

A user can use the web UI editor if their username appears in `editing-users` **or** they belong to any group in `editing-groups`. If neither is set, every user who can log in is allowed to edit.

> [!NOTE]
>
> Password-based users have no groups, groups only ever come from OIDC claims. `editing-groups` has no effect unless [OIDC](#oidc-authentication) is configured.

### Restricting a user to specific pages

To limit which pages a particular password-based user can edit, set `restrict-editing` on that user under `auth.users`:

```yaml
auth:
  secret-key: ...
  users:
    admin:
      password: ...
      # no restrict-editing = can edit any page, per allow-editing

    alice:
      password: ...
      restrict-editing:
        - home
        - homelab
```

When a user has `restrict-editing` set, it takes precedence over `allow-editing` for them: they can only edit the listed pages (identified by `slug`) and cannot add pages or change styling, regardless of `allow-editing`. Users without `restrict-editing` fall back to `allow-editing`.

> [!NOTE]
>
> `restrict-editing` is only available for password-based users configured under `auth.users`, since OIDC users have no corresponding config entry to attach it to.

This is independent of `allow-editing` and `restrict-editing`, which control *which pages* can be edited rather than *who* can edit them, see [Server configuration](configuration.md#editing-users-editing-groups) for details.

---

## Docker / Environment Variables

All OIDC fields support the existing `${ENV_VAR}` and `${secret:name}` syntax:

```yaml
auth:
  secret-key: ${secret:auth_secret}
  oidc:
    issuer-url: ${OIDC_ISSUER_URL}
    client-id: ${OIDC_CLIENT_ID}
    client-secret: ${secret:oidc_client_secret}
    redirect-url: ${OIDC_REDIRECT_URL}
```

Docker Compose example:

```yaml
services:
  dynacat:
    image: panonim/dynacat
    volumes:
      - ./config:/app/config
    environment:
      - OIDC_ISSUER_URL=https://auth.example.com
      - OIDC_CLIENT_ID=dynacat
      - OIDC_REDIRECT_URL=https://dashboard.example.com/api/oidc/callback
    secrets:
      - auth_secret
      - oidc_client_secret

secrets:
  auth_secret:
    file: ./auth_secret.txt
  oidc_client_secret:
    file: ./oidc_secret.txt
```
