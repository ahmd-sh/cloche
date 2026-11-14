# cloche

Cloche delivers app builds to your phone over Tailscale. An agent on your
computer pushes an APK, the phone downloads and checks it, and a tap
installs it. iPhones get builds through a web app.

```
cloche push ──unix socket──▶ cloche serve ◀──tailnet (HTTP, long poll)── Cloche (Android)
  (agent)                     (computer)                                 notifies; a tap installs
```

## Use

```sh
cloche push app/build/outputs/apk/debug/app-debug.apk -m "Sessions: rename inline"
cloche push app-debug.apk -wait   # until it's installed (0), refused/failed/declined (3), or 30m (4)
cloche status                     # the phone, and what became of recent builds
```

`cloche setup` adds a skill that tells Claude Code how to use cloche.

## Set up

Cloche needs [Tailscale](https://tailscale.com/download) on the computer and
the phone, signed in to the same account.

macOS and Linux:

```sh
curl -fsSL https://ahmd-sh.github.io/cloche/install.sh | sh
```

Windows, in PowerShell:

```powershell
irm https://ahmd-sh.github.io/cloche/install.ps1 | iex
```

The script downloads cloche from the latest release, checks it against the
release's checksums, puts it in `~/.local/bin` (on Windows,
`%LOCALAPPDATA%\Programs\cloche`), and runs `cloche setup`. With Go:
`go install github.com/ahmd-sh/cloche/cmd/cloche@latest && cloche setup`.

`cloche setup` runs `cloche serve` with launchd on macOS, a systemd user
service on Linux, or a Task Scheduler task on Windows, and adds the agent
skill. `cloche uninstall` removes both.

Then pair the phone:

```sh
cloche pair     # scan the code with the phone's camera, or type it into Cloche
```

The code opens a page that helps install Tailscale and Cloche on the phone,
then pairs them.

To build the app yourself: `cd android && ./gradlew assembleRelease`. Cloche
updates itself like any other build:
`cloche push android/app/build/outputs/apk/release/app-release.apk`.

Settings in the app lists what a reliable connection needs: notifications,
permission to install apps, unrestricted battery use, and Tailscale
connected.

For an emulator, without Tailscale:

```sh
cloche serve -dev                    # loopback only, no Tailscale checks (CLOCHE_HOME for a scratch state)
adb reverse tcp:41778 tcp:41778
```

## iPhones

iPhones use a web app. iOS installs an app from an `itms-services://` link
when the app is signed for that iPhone. `cloche serve` runs the web app over
HTTPS on the computer's tailnet name, with a certificate from Tailscale.

```sh
cloche pair -ios                 # an iPhone of yours: scan the code, or open the link in Safari
cloche pair -friend sam          # a friend's iPhone, on their own Tailscale account
cloche push App.ipa -m "note"    # or the .app from xcodebuild; -to sam for the friend's
```

After pairing, the web app:

1. offers a profile that sends the iPhone's UDID to the computer. `cloche
   pair` and `cloche status` show it, for registering in the Apple developer
   account that signs the builds;
2. asks to be added to the Home Screen;
3. turns on notifications from the Home Screen, through Apple's push
   service, encrypted for the iPhone.

Each push sends a notification. Tapping it opens the build, and Install
hands it to iOS, which asks, downloads it and installs it. iOS doesn't
report whether the install finished. `cloche status` shows progress up to
downloaded.

Requirements:

- **HTTPS certificates on for the tailnet**, on the admin console's [DNS
  page](https://login.tailscale.com/admin/dns). Let's Encrypt issues the
  certificate, which puts the computer's tailnet name
  (`name.tailnet.ts.net`) in public certificate logs. `cloche status` says
  when the web app is down and why.
- **A build signed for the iPhone**: development or ad hoc, with a
  provisioning profile listing its UDID. Anyone else's iPhone needs a paid
  Apple Developer account. `cloche push` refuses builds iOS would refuse and
  says why. An unsigned build goes as a download, for SideStore or AltStore
  to sign with the iPhone owner's Apple ID.
- **For a friend**: Tailscale on their iPhone, and this computer shared with
  them (admin console, the computer's menu, Share). Sharing gives them this
  computer alone, with every port on it open unless the access controls
  limit them. Limit share recipients to cloche's port, and keep them out of
  the allow-all rule:

  ```json
  "grants": [
    {"src": ["autogroup:member"], "dst": ["*"], "ip": ["*"]},
    {"src": ["autogroup:shared"], "dst": ["*"], "ip": ["tcp:41780"]}
  ]
  ```

  Their iPhone sees only the builds pushed to it with `-to`.

The web app's port is 41780. For the iOS Simulator, `cloche serve -dev`
serves it at `https://localhost:41780` with a local CA. Trust the CA with
`xcrun simctl keychain booted add-root-cert <home>/dev-ca.pem`.

## How it works

`cloche serve` listens only on the computer's tailnet address. The phone
keeps a long poll open from a foreground service, and a push reaches it as
soon as it's made. The server holds each poll for up to 270 seconds, which
bounds how long a dead connection goes unnoticed. Tailnet addresses stay the
same as the phone moves between networks. [docs/protocol.md](docs/protocol.md)
has the wire format.

## Security

- **Who can reach the computer.** Every request is checked with `tailscale
  whois`. Only a node of the computer's own Tailscale user can pair, and
  only with the one-time code from `cloche pair`. A computer that is a
  tagged node has no owner, and pairs only friends' iPhones. After pairing,
  only the paired phone's node, with its secret, can poll or fetch. Both
  servers answer only to the computer's own address and names, which blocks
  DNS rebinding.
- **Who the phone trusts.** Pairing pins the computer's signing key, and the
  QR code carries its fingerprint. Every delivery's manifest is signed with
  that key, and every file is checked against the manifest's hash. The phone
  pairs only with tailnet addresses and connects only through Tailscale's
  VPN. A link can't point it at the internet through an exit node, or at
  another app on the phone. Pairing with a new computer shows which one it
  replaces.
- **What installs.** Nothing installs without a tap. The phone reads each
  APK itself and goes by what it reads. It refuses a build signed with a
  different key from the installed app, or from the key it pinned when it
  first installed that app, which it keeps after an uninstall. It refuses a
  build older than the installed one. An app it hasn't seen before gets no
  one-tap Install. Only Cloche's own notifications can open a build's
  Install. Updates to Cloche itself also go through Android's install
  prompt.
- **iPhones.** The web app answers only on the computer's tailnet name.
  Tailscale identifies the node behind each request. A paired iPhone sees
  only its own builds. Requests that change anything must come from the web
  app's own origin, and no other page can frame it. A friend's iPhone pairs
  only with a `-friend` code and gets only builds that name it. iPhones
  don't receive file paths from the computer.
- **Where the controls are.** Pins, pairing and the tap live on the phone.
  An agent on the computer can push, but it can't trust a new signer or
  install anything. That holds when Cloche is signed with its own key: set
  `CLOCHE_KEYSTORE` and its passwords in the environment when building it.
  With the debug key, any program running as you can sign a replacement
  Cloche, and Android's install prompt is the last check.

Releases are built by GitHub Actions, with build provenance for every file:
`gh attestation verify cloche-linux-amd64.tar.gz --repo ahmd-sh/cloche`.
Release APKs are signed by the certificate with SHA-256
`cd0e700aee2266eff1d3cb8315e237d9a827e8b525903cb4d81fc3772aec563a`.

What it doesn't stop:

- A hijacked agent can push a bad build of any app it can sign, which for
  debug builds is any app signed with `~/.android/debug.keystore`. Your tap
  is the check, and the build's sheet shows what the phone read from the
  APK.
- A build's source (repo, branch, commit) is whatever the push says. `cloche
  push` reads it from git, and any program that can push can set it.
- The CLI's socket gives full control to any program running as you: push,
  pair, unpair, or send a build to a friend's iPhone with `-to`. The skill
  asks agents to send to a friend only when asked. Nothing enforces it.
- An iPhone is known by its Tailscale node alone. Any app on a paired iPhone
  can list and download that iPhone's builds.

## License

MIT: see [LICENSE](LICENSE). The app and the web app use IBM Plex Mono,
under the SIL Open Font License
([android/app/FONT-LICENSE.txt](android/app/FONT-LICENSE.txt)).
