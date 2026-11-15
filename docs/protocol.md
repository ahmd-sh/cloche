# cloche protocol

JSON over HTTP/1.1, between the phone and `cloche serve` on the computer's tailnet
address, port 41778. Types are in `internal/wire` and
`android/.../link/Wire.kt`; `testdata/golden.json` is a delivery the computer
signed, which both ends' tests check.

The phone makes every request. Every request is checked against
Tailscale (`tailscale whois` on its source address). Pairing needs a node of
the computer's own Tailscale user, or a friend's code; everything else needs a
paired phone's node and its secret, as `Authorization: Bearer <secret>`.

A computer keeps a list of phones, and a phone a list of computers. Each
pairing is its own: its own key on the phone, its own secret and cursor.

## Pairing

`cloche pair -android` opens pairing for 15 minutes with a one-time code (12
characters of Crockford base32, `XXXX-XXXX-XXXX`; five wrong tries close it)
and shows a link:

```
https://ahmd-sh.github.io/cloche/pair/#a=100.101.102.103%3A41778&c=K7Q2-M9XD-3FPA&f=<sha256 of the key, base64url>&h=my-laptop&n=Sam%27s+builds
```

`n` is the name the computer offers (`cloche setup -name`), when it has one.
`cloche pair -android -friend sam` opens it for 24 hours for any node that
can reach the computer, and adds `friend=1`. The page (`site/pair`) reads the
fragment, which stays in the browser, and opens Cloche with the same
parameters as `cloche://pair?a=…&c=…&f=…&h=…`.

```
POST /v1/pair  {"code": "K7Q2-M9XD-3FPA", "device": "Pixel 10 Pro"}
→ 200 {"host": "my-laptop", "name": "Sam's builds", "addr": "100.101.102.103:41778", "key": "<PKIX DER, base64>",
       "secret": "<32 bytes, base64url>", "head": 11}
```

The phone checks the key against `f` when the link carried it, and keeps the
key (pinned: every manifest from that computer must be signed by it) and the
secret (sealed with a key in Android's keystore). Pairing again with the same
key refreshes that computer. The computer keeps the phone's node ID, the
secret's SHA-256, and the friend's name for a friend's phone. Pairing again
from the same node replaces that phone. `head` is where the phone starts:
after everything but the last hour's deliveries.

## Polling

```
GET /v1/inbox?after=11&wait=270
→ 200 {"head": 12, "name": "Sam's builds", "deliveries": [{"manifest": "<base64>", "sig": "<base64>"}]}
```

`name` is the name the computer offers now, empty when it has none.

Every request carries `User-Agent: Cloche/<versionName> (<versionCode>)`.
The computer keeps the latest, shows it in `cloche status`, and sends the
phone its own release's Cloche when the phone's is older.

Held open up to `wait` seconds (at most 300) until there's a delivery for
this phone after `after`, then answered with all of them, oldest first. The
phone picks `wait` to end its polls to every computer at the same moment, on
a 270-second clock of its own. A
push goes to the owner's phones, or to the friends and phones `-to` names. The phone moves its
cursor to `head`, and polls again at once. A cursor past `head` (the computer's
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

`apk` is what the computer's build tools read, when they're installed: the phone
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
The phone sends each stage until the computer has heard it. The computer
keeps them for each phone. `cloche push -wait` waits on these.

## Errors

`{"error": "…"}` with 400, 403 (pairing refused), 404 (no such delivery),
401 (not a paired phone, or a secret the computer doesn't know: the phone
forgets that computer and says so), or 503 (Tailscale couldn't say who
called).

## iPhones

An iPhone has no app: `cloche serve` serves it a web app over HTTPS on
port 41780 of the computer's tailnet address, as `https://<name>.<tailnet>.ts.net:41780`,
with the certificate `tailscale cert` gives the computer. Requests are checked
with `tailscale whois` like the phone's; the node is the credential, with no
secret, since iOS fetches builds itself without the web app's cookies or
headers. POSTs from the web app must carry its own `Origin`.

```
GET  /api/me                      paired or not; if paired, its builds; the computer's offered name
POST /api/pair       {"code"}     with a code from cloche pair -ios, or -ios -friend
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
(RFC 8291, aes128gcm) and signed with the computer's VAPID key (RFC 8292,
`webpush.pem`), through Apple's push service. Their payload is
`{"title", "body", "url", "tag"}`.

Every delivery shares one sequence, and each phone's inbox has only those
for it. An iPhone build's `result` for each iPhone is set by the computer from
what iOS fetches:
`asked` (the manifest), `installing` (the download began), `downloaded`
(all of it, which is final: iOS doesn't report installs).

