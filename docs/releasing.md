# Releasing

## Once, on GitHub

1. In Settings, Environments, create an environment named `release`. Limit
   its deployment branches and tags to tags matching `v*`, and add the
   signing key as secrets, from the folder that holds it:

   ```sh
   base64 < cloche-release.p12 | gh secret set CLOCHE_KEYSTORE_BASE64 --env release
   gh secret set CLOCHE_KEYSTORE_PASSWORD --env release < password.txt
   ```

2. In Settings, General, Releases, turn on release immutability. A published
   release's files and tag then can't change.

## Each release

```sh
echo 0.3.1 > VERSION
git commit -am "cloche 0.3.1"
git tag v0.3.1
git push origin main v0.3.1
```

The Release workflow checks the tag against `VERSION`, runs the tests,
builds the binaries and the APK, checks that the release key alone signed
the APK, and publishes the release with `checksums.txt`, `latest.json` and
build provenance for every file.

`VERSION` also sets the app's version. Its versionCode is
major × 1,000,000 + minor × 1,000 + patch.

## The release key

Release APKs are signed by the certificate with SHA-256
`cd0e700aee2266eff1d3cb8315e237d9a827e8b525903cb4d81fc3772aec563a`. The
workflow refuses an APK signed by anything else.

The keystore and its password live outside the repo, with an offline backup.
Android installs an update to Cloche only when the same key signed it. If
the key is lost, everyone has to uninstall Cloche to get the next version.
