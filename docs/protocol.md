# cloche protocol

JSON over HTTP/1.1, between the phone and `cloche serve` on the Mac's tailnet
address, port 41778. Types are in `internal/wire` and
`android/.../link/Wire.kt`; `testdata/golden.json` is a delivery the Mac
signed, which both ends' tests check.

The phone always calls; the Mac never does. Every request is checked against
Tailscale (`tailscale whois` on its source address). Pairing needs a node of
the Mac's own Tailscale user; everything else needs the paired phone's node
and its secret, as `Authorization: Bearer <secret>`.

## Pairing

`cloche pair` opens pairing for 15 minutes with a one-time code (12
characters of Crockford base32, `XXXX-XXXX-XXXX`; five wrong tries close it)
and shows a link:

```
cloche://pair?a=100.101.102.103:41778&c=K7Q2-M9XD-3FPA&f=<sha256 of the key, base64url>&h=my-laptop
```

```
POST /v1/pair  {"code": "K7Q2-M9XD-3FPA", "device": "Pixel 10 Pro"}
→ 200 {"host": "my-laptop", "addr": "100.101.102.103:41778", "key": "<PKIX DER, base64>",
       "secret": "<32 bytes, base64url>", "head": 11}
```

The phone checks the key against `f` when the link carried it, and keeps the
key (pinned: every manifest must be signed by it) and the secret (sealed
with a key in Android's keystore). The Mac keeps the phone's node ID and the
secret's SHA-256. `head` is where the phone starts: after everything but the
last hour's deliveries.

## Polling

```
GET /v1/inbox?after=11&wait=270
→ 200 {"head": 12, "deliveries": [{"manifest": "<base64>", "sig": "<base64>"}]}
```

Held open up to `wait` seconds (at most 300) until there's a delivery after
`after`, then answered with all of them, oldest first. The phone moves its
cursor to `head`, and polls again at once. A cursor past `head` (the Mac's
state was reset) is answered at once, and the phone starts again from
`head`.

`manifest` is the JSON below, as bytes; `sig` is ECDSA P-256 over its
SHA-256, ASN.1 DER (Java's `SHA256withECDSA`). The phone verifies the bytes
before it parses them, and drops a delivery that doesn't verify.

```json
{
  "v": 1, "id": "cpy5wdfz", "seq": 12, "host": "my-laptop", "pushed_at": "2026-11-04T18:18:24Z",
  "file": {"name": "app-debug.apk", "size": 8457299, "sha256": "…"},
  "apk": {"package": "sh.ahmd.wallflower", "version_code": 1, "version_name": "0.1.0", "label": "wallflower", "signer": "…"},
  "note": "Sessions: rename inline",
  "source": {"dir": "/Users/…/wallflower", "repo": "wallflower", "branch": "main", "commit": "020c4dd", "dirty": true}
}
```

`apk` is what the Mac's build tools read, when they're installed: the phone
shows it until it has the file, then reads its own. `source` is read from
git by `cloche push`. `note` is the pusher's own words.

## The file

```
GET /v1/apk/{id}        Range: bytes=N-  to resume
→ 200 or 206, application/vnd.android.package-archive
```

The phone hashes it against `file.sha256` before anything else.

## Results

```
POST /v1/result/{id}  {"status": "installed", "detail": "", "package": "…", "version_code": 1, "version_name": "…", "signer": "…"}
→ 204
```

Statuses: `ready` (fetched, checked, notified), `blocked` (refused, with
why), `installing`, `installed`, `failed`, `declined` (cancelled at
Android's prompt), `superseded` (a newer build of the app arrived first).
The phone sends each stage until the Mac has heard it. `cloche push -wait`
waits on these.

## Errors

`{"error": "…"}` with 400, 403 (pairing refused), 404 (no such delivery),
401 (not the paired phone, or a secret the Mac doesn't know: the phone
stops and asks to pair again), or 503 (Tailscale couldn't say who called).

## iPhones

An iPhone has no app: `cloche serve` serves it a web app over HTTPS on
port 41780 of the Mac's tailnet address, as `https://<name>.<tailnet>.ts.net:41780`,
with the certificate `tailscale cert` gives the Mac. Requests are checked
with `tailscale whois` like the phone's; the node is the credential, with no
secret, since iOS fetches builds itself without the web app's cookies or
headers. POSTs from the web app must carry its own `Origin`.

```
GET  /api/me                      paired or not; if paired, its builds
POST /api/pair       {"code"}     with a code from cloche pair -ios / -friend
POST /api/notify     <PushSubscription JSON>, answered once a first notification's sent
POST /api/unpair
GET  /api/builds/{id}             a build (marks it looked at)
POST /api/builds/{id}/clear
GET  /udid.mobileconfig           a Profile Service profile, signed with the web app's certificate
POST /udid/{token}                iOS posts the UDID there (a CMS-signed plist); 301 back to the app
GET  /m/{id}/manifest.plist       what itms-services:// asks for: Install was tapped
GET  /m/{id}/app.ipa              the build, as iOS downloads it (?download=1 to save it)
GET  /m/{id}/icon.png
```

Notifications are Web Push (RFC 8030), encrypted for the subscription
(RFC 8291, aes128gcm) and signed with the Mac's VAPID key (RFC 8292,
`webpush.pem`), through Apple's push service. Their payload is
`{"title", "body", "url", "tag"}`.

An iPhone's deliveries share the sequence with the phone's, but the phone's
inbox skips them. Their `result` is set by the Mac from what iOS fetches:
`asked` (the manifest), `installing` (the download began), `downloaded`
(all of it, which is final: iOS doesn't report installs).

