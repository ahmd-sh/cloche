---
name: cloche
description: Deliver an Android build (APK) to the user's phone, or an iOS build (.ipa or .app) to a paired iPhone, with cloche, which reaches it over Tailscale wherever it is. Use when told to use cloche, or asked to put, send or install a build on the phone (or a friend's iPhone) while no device is attached.
---

# Delivering a build with cloche

`cloche` runs on this Mac and relays APKs to the user's paired phone over
their tailnet. The phone downloads each build, checks it, and notifies; the
user taps to install. Nothing installs without that tap.

1. Build the APK the way the project does (for example
   `./gradlew :app:assembleDebug`).
2. Push it, with one line on what changed. The user reads it in the
   notification, so write it for them, not as a commit message:

   ```sh
   cloche push app/build/outputs/apk/debug/app-debug.apk -m "Sessions: rename inline"
   ```

3. Tell the user what push printed: the phone has it, the phone is offline
   (it gets the build when it's back), or no phone is paired.

Then carry on. Don't wait for the install unless the next step needs it.

## More

- `cloche push … -wait` blocks until the build is installed (exit 0), or
  refused, failed, declined or superseded (exit 3), or 30 minutes pass
  (exit 4). Use it only when what you do next depends on the install.
- `cloche status` shows the phone (connected, or when it was last seen) and
  recent deliveries with what became of each.
- If push says cloche isn't running:
  `launchctl kickstart -k gui/$(id -u)/sh.ahmd.cloche`.
- The phone refuses a build signed with a different key from the installed
  app (or from the one Cloche first installed), or with a lower versionCode
  than the installed one. If a push comes back refused, tell the user why
  rather than pushing again.
- Don't run `cloche pair`, `cloche unpair` or `cloche serve` unless the user
  asks: pairing is theirs to do.
- When a device is attached (`adb devices` lists one) and the user hasn't
  asked for cloche, `adb install -r` is quicker.

## iPhones

An iPhone has no Cloche app: `cloche serve` runs a web app for it, and a tap
there hands the build to iOS's own installer. iOS installs only builds
signed for that iPhone, by a provisioning profile that lists its UDID
(`cloche status` shows each paired iPhone's).

1. Build for a device, signed:

   ```sh
   xcodebuild -scheme App -configuration Debug -destination generic/platform=iOS \
     -derivedDataPath build/dd -allowProvisioningUpdates build
   ```

   That's a development build (the iPhone needs Developer Mode on). For an
   ad hoc one, archive and export with `method` `release-testing` in the
   export options.
2. Push the `.app` (cloche packs it) or the `.ipa`. `-to` names the iPhone;
   without it, the push goes to the user's one iPhone. A friend's iPhone
   gets a build only when `-to` names it. Send to a friend only when the
   user asks for that friend by name: it lands on someone else's phone.

   ```sh
   cloche push build/dd/Build/Products/Debug-iphoneos/App.app -m "Inbox: swipe to archive" -to sam
   ```

3. Tell the user what push printed, notes included.

- Push refuses a build iOS wouldn't install: a Simulator build, one signed
  for the App Store, an expired profile, or a profile without the iPhone's
  UDID. Tell the user why; registering a device and signing are theirs to
  do.
- An unsigned build goes too, as a download the iPhone's owner installs with
  SideStore or AltStore, signed with their own Apple ID.
- iOS doesn't report installs: `-wait` ends when the iPhone has downloaded
  the build (exit 0).
- `cloche pair -ios` and `cloche pair -friend <name>` pair iPhones: like
  `cloche pair`, only when the user asks.
