# Cloche - Let AI ship you builds

<p align="center"><img src="docs/banner.png" alt="Cloche"></p>

Cloche delivers app builds to your phone over Tailscale. An agent on your
computer pushes an APK, the phone downloads and checks it, and a tap
installs it. iPhones get builds through a web app.

```
cloche push ──unix socket──▶ cloche serve ◀──tailnet (HTTP, long poll)── Cloche (Android)
  (agent)                     (computer)                                 notifies; a tap installs
```

## Install on macOS and Linux

```sh
curl -fsSL https://ahmd-sh.github.io/cloche/install.sh | sh
```

## Use

```sh
cloche push app/build/outputs/apk/debug/app-debug.apk -m "Sessions: rename inline"
cloche push app-debug.apk -wait   # until it's installed (0), refused/failed/declined (3), or 30m (4)
cloche push app-debug.apk -to sam # to a friend's phones instead of yours
cloche status                     # the phones, and what became of recent builds
```

`cloche setup` will add a skill that tells Claude Code how to use cloche.

## Set up

Cloche needs [Tailscale](https://tailscale.com/download) on the computer and
the phone, signed in to the same account.

The install script downloads cloche from the latest release, checks it
against the release's checksums, puts it in `~/.local/bin`, and runs `cloche
setup`. With Go: `go install github.com/ahmd-sh/cloche/cmd/cloche@latest &&
cloche setup`.

`cloche setup` runs `cloche serve` with launchd on macOS or a systemd user
service on Linux, and adds the agent skill. `cloche uninstall` removes both.

Then pair the phone:

```sh
cloche pair -android    # scan the code with the phone's camera, or type it into Cloche
```

Without `-android` or `-ios`, `cloche pair` asks which.

The code will open a page that helps install Tailscale and Cloche on the
phone, then pairs them.

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

## Update

```sh
cloche update
```

`cloche serve` checks GitHub for a new release once a day, and `cloche
status` says when one is out. `cloche update` downloads it, checks it
against the release's checksums, replaces cloche and restarts the service.
The phone will then get the new Cloche like any build: a notification, and
a tap installs it. iPhones will get the new web app the next time they open
it.
To turn the check off: `CLOCHE_NO_UPDATE_CHECK=1 cloche setup`.

## iPhones

iPhones use a web app. iOS installs an app from an `itms-services://` link
when the app is signed for that iPhone. `cloche serve` runs the web app over
HTTPS on the computer's tailnet name, with a certificate from Tailscale.

```sh
cloche pair -ios                 # an iPhone of yours: scan the code, or open the link in Safari
cloche push App.ipa -m "note"    # or the .app from xcodebuild
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
report whether the install finished. `cloche status` will show progress
up to downloaded.

Requirements:

- **HTTPS certificates on for the tailnet**, on the admin console's [DNS
  page](https://login.tailscale.com/admin/dns). Let's Encrypt issues the
  certificate, which puts the computer's tailnet name
  (`name.tailnet.ts.net`) in public certificate logs. `cloche status` will
  say when the web app is down and why.
- **A build signed for the iPhone**: development or ad hoc, with a
  provisioning profile listing its UDID. Anyone else's iPhone needs a paid
  Apple Developer account. `cloche push` refuses builds iOS would refuse and
  says why. An unsigned build will go as a download, for SideStore or AltStore
  to sign with the iPhone owner's Apple ID.
The web app's port is 41780. For the iOS Simulator, `cloche serve -dev`
serves it at `https://localhost:41780` with a local CA. Trust the CA with
`xcrun simctl keychain booted add-root-cert <home>/dev-ca.pem`.

## Friends and more computers

A phone can pair with more than one computer, and a computer with more
than one phone of either kind.

```sh
cloche pair -android -friend sam   # a friend's Android phone, on their own Tailscale account
cloche pair -ios -friend sam       # a friend's iPhone
cloche push app.apk -to sam        # to Sam's phones; -to sam,alex for both
cloche friends                     # friends' phones, and the last build each got
cloche unpair sam                  # forget all of Sam's phones
```

A friend's phones are named after them, like `sam-android` and
`sam-iphone`. `-to` and `cloche unpair` take those names for one phone.

`cloche push` sends to your own phones unless `-to` names friends or
phones. A build goes to the phones that can install it: APKs to Android
phones, iOS builds to iPhones. With `-wait`, the first install ends the
wait.

A friend needs Tailscale, with this computer shared to them (admin
console, the computer's menu, Share). Sharing gives them this computer
alone, with every port on it open unless the access controls limit them.
Limit share recipients to cloche's ports, and keep them out of the
allow-all rule:

```json
"grants": [
  {"src": ["autogroup:member"], "dst": ["*"], "ip": ["*"]},
  {"src": ["autogroup:shared"], "dst": ["*"], "ip": ["tcp:41778", "tcp:41780"]}
]
```

`cloche setup -name "Sam's builds"` sets the name phones will show for this
computer. On Android, Settings lists each paired computer: a name given
there replaces the one the computer offers, and Review first makes every
build from that computer open for review before it installs. By default
an update to an app Cloche installed before takes one tap from the
notification, from any computer. Pairing with a computer that has the
same name as one already paired will ask whether to replace it or keep
both. An iPhone pairs with each computer's web app separately, and each
keeps its own name for its computer.

## How it works

`cloche serve` listens only on the computer's tailnet address. The phone
keeps a long poll open from a foreground service, and a push reaches it as
soon as it's made. The server holds each poll for up to 270 seconds, which
bounds how long a dead connection goes unnoticed. Tailnet addresses stay the
same as the phone moves between networks. [docs/protocol.md](docs/protocol.md)
has the wire format.

## Security

- **Who can reach the computer.** Every request is checked with `tailscale
  whois`. Only a node of the computer's own Tailscale user can pair with the
  one-time code from `cloche pair`. A friend's phone pairs only with a code
  from `cloche pair -friend`. A computer that is a tagged node has no
  owner, and pairs only friends' phones. After pairing, only a paired
  phone's node, with its secret, can poll or fetch, and each phone gets
  only the builds meant for it. Both servers answer only to the computer's
  own address and names, which blocks DNS rebinding.
- **Who the phone trusts.** Pairing pins each computer's signing key, and
  the QR code carries its fingerprint. Every delivery's manifest is signed
  with the key of the computer that sent it, and every file is checked
  against the manifest's hash. The phone pairs only with tailnet addresses
  and connects only through Tailscale's VPN. A link can't point it at the
  internet through an exit node, or at another app on the phone. Pairing
  with a computer that has the same name as a paired one will ask before
  replacing it. Every build will show which computer sent it.
- **What installs.** The phone reads each APK itself and goes by what it reads. It refuses a build signed with a
  different key from the installed app, or from the key it pinned when it
  first installed that app, which it keeps after an uninstall. It refuses a
  build older than the installed one. An app it hasn't seen before gets no
  one-tap Install. Only Cloche's own notifications can open a build's
  Install. Updates to Cloche itself will also go through Android's
  install prompt.
- **iPhones.** The web app answers only on the computer's tailnet name.
  Tailscale identifies the node behind each request. A paired iPhone sees
  only its own builds. Requests that change anything must come from the web
  app's own origin, and no other page can frame it. A friend's iPhone pairs
  only with a `-friend` code and gets only builds that name it or its
  friend. iPhones don't receive file paths from the computer.
- **Where the controls are.** Pins, pairing and the tap live on the phone.
  An agent on the computer can push, but it can't trust a new signer or
  install anything. That holds when Cloche is signed with its own key: set
  `CLOCHE_KEYSTORE` and its passwords in the environment when building it.
  With the debug key, any program running as you can sign a replacement
  Cloche, and Android's install prompt is the last check.

Releases are built by GitHub Actions, with build provenance for every file:
`gh attestation verify cloche-linux-amd64.tar.gz --repo ahmd-sh/cloche`.
Release APKs are signed by the certificate with SHA-256
`cd0e700aee2266eff1d3cb8315e237d9a827e8b525903cb4d81fc3772aec563a`. The
install scripts and `cloche update` check each download against the
release's checksums, fetched from GitHub over HTTPS. Android will install
a new Cloche over the old one only when the same key signed both.

What it doesn't stop:

- A hijacked agent can push a bad build of any app it can sign, which for
  debug builds is any app signed with `~/.android/debug.keystore`. Your tap
  is the check, and the build's sheet will show what the phone read from
  the APK.
- A paired computer, a friend's included, can send any app, and an update
  signed with the same key as the installed app. Pair only with computers
  you trust, and turn on Review first for one you trust less.
- A build's source (repo, branch, commit) is whatever the push says. `cloche
  push` reads it from git, and any program that can push can set it.
- The CLI's socket gives full control to any program running as you: push,
  pair, unpair, or send a build to a friend's phone with `-to`. The skill
  asks agents to send to a friend only when asked.
- An iPhone is known by its Tailscale node alone. Any app on a paired iPhone
  can list and download that iPhone's builds.

## License

MIT: see [LICENSE](LICENSE). The app and the web app use IBM Plex Mono,
under the SIL Open Font License
([android/app/FONT-LICENSE.txt](android/app/FONT-LICENSE.txt)).
