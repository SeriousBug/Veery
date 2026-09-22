# Single sign-on (OIDC)

Veery can delegate sign-in to an external OpenID Connect provider instead of, or
alongside, passkeys. The feature is generic: anything that speaks standard OIDC
discovery with the authorization-code flow works, [pocket-id](https://pocket-id.org)
being the reference setup below. Nothing in the code or config is specific to
one provider.

Passkeys stay enabled. An instance with OIDC configured still offers the passkey
button, and admins can keep minting enrollment links. OIDC only adds a second
door.

## How it works

1. The login page calls `GET /auth/providers`; if OIDC is configured it also
   renders "Sign in with `<name>`", which sends the browser to
   `GET /auth/oidc/start`.
2. Veery discovers the provider from `{issuer}/.well-known/openid-configuration`,
   mints `state`, `nonce` and a PKCE verifier, and redirects to the provider's
   authorization endpoint.
3. The provider authenticates the user and redirects back to
   `GET /auth/oidc/callback` with a code. Veery exchanges the code, verifies the
   ID token (signature, issuer, audience, expiry, nonce) and reads the claims.
4. The identity `(issuer, subject)` is looked up in `oidc_identities`. A known
   identity signs in; an unknown one is provisioned as a new user. Then a normal
   Veery session cookie is issued, exactly as for a passkey login.

State, nonce and the PKCE verifier never leave the server; the state is also
echoed in a short-lived cookie so a flow started in another browser cannot be
completed here (login CSRF).

## Configuration

OIDC turns on when both the issuer and client id are set. All settings are
environment variables, so the provider config is not editable from the UI.

| Env var                     | Default                                | Meaning                                                                                     |
| --------------------------- | -------------------------------------- | ------------------------------------------------------------------------------------------- |
| `VEERY_OIDC_ISSUER`         | (unset)                                | Provider issuer URL, e.g. `https://id.example.com`. Discovery is read from here.             |
| `VEERY_OIDC_CLIENT_ID`      | (unset)                                | Client id registered with the provider.                                                      |
| `VEERY_OIDC_CLIENT_SECRET`  | (unset)                                | Client secret. Leave empty for a public client that relies on PKCE.                          |
| `VEERY_OIDC_REDIRECT_URL`   | `${VEERY_ORIGIN}/auth/oidc/callback`   | Callback URL. Must be registered with the provider and match exactly.                        |
| `VEERY_OIDC_SCOPES`         | `openid profile email`                 | Space- or comma-separated scopes. `groups` is appended automatically when admin groups are set. |
| `VEERY_OIDC_ADMIN_GROUPS`   | (unset)                                | Comma-separated provider group names that grant Veery admin.                                 |
| `VEERY_OIDC_NAME`           | `SSO`                                  | Label on the login button, e.g. `Pocket ID`.                                                 |

Setting only one of `VEERY_OIDC_ISSUER` / `VEERY_OIDC_CLIENT_ID` is a fatal
config error, so a typo cannot silently disable SSO. Reaching the provider is
deferred to the first login, so an identity provider that is down at boot does
not stop Veery from serving passkey logins.

## Accounts and admins

- **Provisioning.** The first sign-in from an unknown `(issuer, subject)` creates
  a user. The display name comes from the `name` claim, falling back to
  `preferred_username`, `email`, then `sub`. The email is stored on the link for
  reference; it is never used to match an existing account (an unverified email
  must not be able to take over an account).
- **Admin by group.** When `VEERY_OIDC_ADMIN_GROUPS` is set, every OIDC login
  re-syncs the account's admin flag against the `groups` claim, so adding or
  removing someone in the provider takes effect on their next login. The last
  admin is never demoted, so a wrong group mapping cannot lock you out.
- **Without admin groups.** The flag is left alone. On a fresh instance the first
  user to arrive, passkey or OIDC, is the admin (the same bootstrap rule as the
  passkey enrollment link).
- **`sub` is the key.** Renaming a user in the provider keeps their Veery
  account; deleting and recreating them there creates a new Veery account.

## Provider requirements

Veery needs, from discovery:

- `authorization_endpoint` and `token_endpoint` supporting the authorization-code
  flow (`response_type=code`), with PKCE (S256).
- `jwks_uri` to verify the ID token signature, and an `issuer` that matches the
  configured URL.
- An `id_token` in the token response carrying `sub`.
- `name` and/or `email` for a readable display name.
- A `groups` array when `VEERY_OIDC_ADMIN_GROUPS` is used. If a provider only
  releases groups from the `userinfo` endpoint, Veery merges that response too
  (best-effort; ID-token claims win).
- The client must be allowed to use the redirect URL
  `https://<your-host>/auth/oidc/callback`. Restrict who may use the client with
  the provider's own group/access policy.

## Deploying with pocket-id

Pocket ID is a passkey-based OIDC provider, so the end result is passkeys all the
way down: users authenticate to Pocket ID with a passkey, and Pocket ID vouches
for them to Veery.

### 1. Run Pocket ID

If you do not have it already, bring up Pocket ID behind TLS (it needs a secure
context for its own passkeys). A minimal compose setup:

```yaml
services:
  pocket-id:
    image: ghcr.io/pocket-id/pocket-id:latest
    environment:
      APP_URL: https://id.example.com
      TRUST_PROXY: "true"
    volumes:
      - pocket-id-data:/app/data
    restart: unless-stopped

volumes:
  pocket-id-data:
```

Finish its first-run setup at `https://id.example.com`, create your user, and
enroll a passkey. Put it behind your TLS proxy like any other service.

### 2. Create an OIDC client

In Pocket ID open **Administration → OIDC Clients → Add OIDC Client**:

- **Name**: `Veery`
- **Callback URL**: `https://veery.example.com/auth/oidc/callback`
- Leave PKCE enabled (Veery uses S256).
- If you want Veery to manage admins by group, create a Pocket ID group (e.g.
  `Veery Admins`), add the right users, and make sure the client may return the
  `groups` claim. Veery requests the `groups` scope automatically when you set
  `VEERY_OIDC_ADMIN_GROUPS`.

Save, then copy the **Client ID** and **Client Secret**.

### 3. Point Veery at Pocket ID

Add the OIDC variables to Veery (alongside its existing config):

```yaml
services:
  veery:
    image: ghcr.io/seriousbug/veery:latest
    environment:
      VEERY_RP_ID: veery.example.com
      VEERY_ORIGIN: https://veery.example.com
      VEERY_OIDC_ISSUER: https://id.example.com
      VEERY_OIDC_CLIENT_ID: <client id from Pocket ID>
      VEERY_OIDC_CLIENT_SECRET: <client secret from Pocket ID>
      VEERY_OIDC_NAME: Pocket ID
      # Optional: members of this Pocket ID group become Veery admins.
      VEERY_OIDC_ADMIN_GROUPS: Veery Admins
    volumes:
      - veery-data:/data
      - /var/run/docker.sock:/var/run/docker.sock
      # plus the host /proc and /sys mounts and --group-add from the README

volumes:
  veery-data:
```

`VEERY_OIDC_REDIRECT_URL` only needs setting if the callback is not
`${VEERY_ORIGIN}/auth/oidc/callback`.

### 4. Sign in

Open `https://veery.example.com`. The login page now shows **Sign in with Pocket
ID** above the passkey button (or instead of it, if you have no passkeys yet).
The first user to sign in on a fresh instance becomes admin. If you set
`VEERY_OIDC_ADMIN_GROUPS`, add yourself to that group in Pocket ID before signing
in.

You can still enroll a passkey for the same person: have an admin mint an invite
from **Invites**, open it while signed in, and add a passkey. That account then
has both an OIDC link and a passkey.

## Troubleshooting

- **"could not reach the identity provider"** on sign-in: Veery could not fetch
  the discovery document. Check that `VEERY_OIDC_ISSUER` is reachable from the
  Veery container (Docker networking, DNS, TLS trust).
- **`redirect_uri` mismatch** from the provider: `VEERY_OIDC_REDIRECT_URL` (or
  the default) must match the callback URL registered with the provider byte for
  byte, including scheme, host and trailing path.
- **Sign-in loops back to `/login?error=oidc_state`**: the login took longer than
  10 minutes, or the state cookie was not sent (e.g. the callback host differs
  from `VEERY_ORIGIN`).
- **Signed in but not an admin** when using groups: the ID token did not carry a
  `groups` claim. Confirm the provider releases groups for this client and that
  the names in `VEERY_OIDC_ADMIN_GROUPS` match exactly (matching is
  case-insensitive).
- **The SSO button is missing**: `VEERY_OIDC_ISSUER` or `VEERY_OIDC_CLIENT_ID`
  is unset. Check the logs for `oidc: login enabled via ...`.
