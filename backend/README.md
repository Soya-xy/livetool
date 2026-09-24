# LiveTool license and update service

This is a standalone Node.js service. The desktop app only validates a card and uses user authorization endpoints; it has no administrator UI, administrator credentials, card pepper, or card management token. Operators manage cards through the `/v1/admin/*` API.

## Run

Requires Node.js 20 or newer. From the repository root, configure the service environment and run `npm --prefix backend start`:

| Variable | Required | Purpose |
| --- | --- | --- |
| `CARD_ADMIN_USERNAME` | yes | Service administrator account |
| `CARD_ADMIN_PASSWORD` | yes | Administrator password, at least 12 characters |
| `CARD_KEY_PEPPER` | yes | Long-lived secret, at least 32 bytes; back it up securely |
| `CARD_HOST` | no | Bind address; defaults to `127.0.0.1` |
| `CARD_PORT` | no | HTTP port; defaults to `8787` |
| `CARD_DATA_FILE` | no | Card and audit JSON file; defaults to `license-data/cards.json` beside the service data directory |
| `UPDATE_MANIFEST_FILE` | no | Signed Wails update manifest; defaults to `updates/manifest.json` beside the card data file |
| `UPDATE_ARTIFACT_DIR` | no | Signed update binaries; defaults to `updates/artifacts` beside the card data file |

Example values for a local service:

```sh
export CARD_ADMIN_USERNAME='admin'
export CARD_ADMIN_PASSWORD='replace-with-a-long-unique-password'
export CARD_KEY_PEPPER='replace-with-at-least-32-random-bytes'
export CARD_DATA_FILE='/var/lib/livetool/cards.json'
npm --prefix backend start
```

The service speaks HTTP. For remote use, bind it to loopback behind a trusted HTTPS reverse proxy; do not expose its HTTP port directly to the Internet. The desktop client permits plain HTTP only for loopback addresses.

## Cards and authorization

The admin API supports login, issuing 1–1000 cards, listing and inspecting cards, updating status, extending expiry, resetting device bindings, and reading audit entries. Card plaintext is shown only in the create response. The database stores HMAC digests using `CARD_KEY_PEPPER`.

Device binding uses the desktop app's random installation ID, not a hardware fingerprint. Clearing or moving application data changes that ID. User authorization sessions are short-lived and held in server memory; service restarts require clients to log in again.

Main endpoints:

- `POST /v1/admin/login`, `GET /v1/admin/me`
- `GET/POST /v1/admin/cards`, `GET /v1/admin/cards/:id`
- `PATCH /v1/admin/cards/:id/status`, `DELETE /v1/admin/cards/:id/bindings`, `POST /v1/admin/cards/:id/extend`
- `GET /v1/admin/audit`
- `POST /v1/auth/login`, `GET /v1/auth/status`, `POST /v1/auth/logout`
- `POST /v1/auth/safe-code`, `POST /v1/auth/unbind`
- `GET /v1/updates/check`, `GET /v1/updates/artifacts/:filename`
- `GET /health`

## Signed desktop updates

The desktop updater checks for updates after card authorization. It prompts the user before downloading and installing, then prompts before restart. The server serves a manifest and artifacts only to a valid user session.

1. Keep the Wails signing private key outside version control. A keypair can be created with `wails3 updater genkey -out backend/private/wails-updater.key`; put its public key in `updater-public-key.pem` and rebuild the app. The same public key must remain embedded for every release in that update line.
2. Build the platform artifact from the matching release version.
3. Generate a signed manifest:

   ```sh
   wails3 updater manifest \
     -version 0.2.1 \
     -channel stable \
     -key backend/private/wails-updater.key \
     -url-prefix https://license.example.com/v1/updates/artifacts/ \
     -output manifest.json \
     bin/livetool.exe
   ```

4. Copy the generated manifest to `UPDATE_MANIFEST_FILE` and the artifact to `UPDATE_ARTIFACT_DIR` using the filename included in its artifact URL. Publish both atomically so clients never see a manifest before its files are present.

The updater signature is verified by the desktop client before installation. The service does not build, sign, or publish releases by itself. Back up `CARD_DATA_FILE`, `CARD_KEY_PEPPER`, and the signing key independently; losing the pepper invalidates stored card digests, and replacing the signing key requires shipping a new app with its public key.

## Storage limits

The current card store is a single-process JSON file. Run one service instance per data file. Move to a transactional shared database before deploying multiple instances. Restrict access to the card file, pepper, administrator credentials, manifest, and update artifacts.
