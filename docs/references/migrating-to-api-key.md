# Migrating from API Token to API Key

Earlier versions of the documentation and of `substreams auth` had you exchange your API key for a JWT and store it as `SUBSTREAMS_API_TOKEN`. That JWT is short-lived and the clients never refresh it, so it eventually stops working.

**If you don't know why you use `SUBSTREAMS_API_TOKEN`, and you only set it because you followed the previous documentation, migrate to an API key.** Nothing else changes: the same endpoints and the same commands.

JWTs remain available for [advanced use cases](./authentication.md#jwt-advanced) where the API key must not be shared. Everyone else should use the API key.

## Migrate

1. Get an API key. The easiest is to run:

   ```bash
   substreams auth
   ```

   This replaces the contents of `.substreams.env` with `export SUBSTREAMS_API_KEY=…`. You can also create or copy a key from [The Graph Market](https://thegraph.market) and run `substreams auth --paste`.

2. Remove `SUBSTREAMS_API_TOKEN` everywhere you set it: your shell profile, `.env` files, CI secrets, Docker Compose files, Kubernetes manifests and deployment scripts. Replace it with `SUBSTREAMS_API_KEY`.

   ```diff
   - export SUBSTREAMS_API_TOKEN="eyJ…"
   + export SUBSTREAMS_API_KEY="server_…"
   ```

3. Reload your environment (`. ./.substreams.env`, or open a new terminal) and check that the stream starts:

   ```bash
   substreams gui ethereum-common@v0.3.1 all_events --start-block=15000000
   ```

## Notes

- If both variables are set, only `SUBSTREAMS_API_KEY` is used, so adding the key is safe even before you remove the token.
- Custom environment variable names passed with `--api-token-envvar` should move to `--api-key-envvar`.
- Code that calls the issuer to get a JWT before every run can stop doing so, unless it really needs a JWT.
- Existing JWTs you already hold keep working until their own expiry.

See [Authentication Reference](./authentication.md) for when each credential is the right one.
