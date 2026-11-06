# Authentication Reference

Substreams endpoints accept two kinds of credentials: an **API key** and a **JWT**. For almost everyone, the answer is the API key.

| | API key | JWT |
| --- | --- | --- |
| Looks like | `server_…` | `eyJ…` (three dot-separated parts) |
| Sent as | `x-api-key` header | `authorization: Bearer <jwt>` header |
| Environment variable | `SUBSTREAMS_API_KEY` | `SUBSTREAMS_API_TOKEN` |
| Lifetime | Until you revoke it | Short-lived, ends at its `expires_at` |
| Refresh | Nothing to do | You must issue a new one before it expires |
| Use it for | Everything: CLI, sinks, services, CI | Advanced cases only (see below) |

## API key (recommended)

An API key is created in [The Graph Market](https://thegraph.market) and sent unchanged on every request. It never expires on its own, so a long-running sink or service keeps working without any token logic on your side.

The simplest way to get one on your machine:

```bash
substreams auth
```

This opens a browser, lets you pick an API key, and writes it to `.substreams.env` as `SUBSTREAMS_API_KEY`. See [Substreams CLI Authentication](../how-to-guides/cli/authentication.md).

To set it yourself, for example in CI or a service:

```bash
export SUBSTREAMS_API_KEY="<YOUR-API-KEY>"
```

Treat an API key like a password: keep it out of source control (add `.substreams.env` to `.gitignore`), and revoke and replace it from The Graph Market if it leaks.

## JWT (advanced)

A JWT is a signed, short-lived token issued from an API key. Reach for one only when you must **not** hand the API key itself to the party making the requests, for example:

- A browser or mobile app that streams directly from an endpoint, where shipping a long-lived key would expose it.
- A backend that gives a limited-time credential to a third party or to an untrusted process.

If you control the machine making the requests, use the API key instead: there is nothing to issue, cache or refresh.

### Issuing a JWT

Exchange an API key for a token with the issuer:

```bash
curl -s -X POST https://auth.thegraph.market/v1/auth/issue \
  -H 'Content-Type: application/json' \
  -d '{"api_key": "'"$SUBSTREAMS_API_KEY"'"}'
```

The response carries the token and the moment it stops being valid:

```json
{"token": "eyJ…", "expires_at": 1767225600}
```

`expires_at` is a Unix timestamp in seconds. Do this on a server you control, never from the client that will use the token: the exchange needs the API key.

### Using a JWT

```bash
export SUBSTREAMS_API_TOKEN="<YOUR-JWT>"
substreams run …
```

The Substreams clients send the token as-is and **do not refresh it**. When it expires, requests fail with an authentication error until you provide a new one. For a long-running stream, reconnect with a fresh token.

### Rules for handling JWTs

- Do not assume a fixed lifetime. Read `expires_at` from the issuer response.
- Issue a new token **before** `expires_at` (leave some margin), not after the first failure.
- Do not cache a token for longer than its `expires_at`, and do not write it to long-lived files such as `.substreams.env`.
- Do not request an artificially long lifetime to avoid refreshing. If you need a credential that does not expire, you want an API key.

## When both are set

If both `SUBSTREAMS_API_KEY` and `SUBSTREAMS_API_TOKEN` are set, **only the API key is sent**. The token is ignored, even if it is still valid.

The `substreams` commands also load `.substreams.env` automatically when it is next to the manifest or in the current directory. Values in that file **override** variables already set in your shell.

The environment variable names can be changed with `--api-key-envvar` and `--api-token-envvar` on the sink commands (`--substreams-api-key-envvar` and `--substreams-api-token-envvar` on `substreams tools`).

## Troubleshooting

- **Authentication errors after it used to work**: if you use a JWT (`SUBSTREAMS_API_TOKEN`), it has likely expired. Switch to an API key, see [Migrating from API token to API key](./migrating-to-api-key.md).
- **Still using an old token after `substreams auth`**: an old `SUBSTREAMS_API_TOKEN` is harmless when an API key is set, but remove it to avoid confusion.
- **`this looks like a JWT, not an API key`** from `substreams auth --paste`: you pasted a JWT. Paste a `server_…` API key instead.
