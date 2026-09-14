# Releasing Netprobe

Pushing a version tag builds a universal Mac app, signs the app and disk image with Developer ID, sends the disk image to Apple for notarization, and publishes it on GitHub.

## One-time setup

Create a **Developer ID Application** certificate in the Apple Developer portal. Install it on your Mac, then export the certificate and private key from Keychain Access as a password-protected `.p12` file.

Create a team API key in App Store Connect and download its `.p8` private key. Apple only lets you download this file once.

Add these GitHub Actions secrets to the repository:

- `MACOS_CERTIFICATE`: the `.p12` file encoded as base64
- `MACOS_CERTIFICATE_PASSWORD`: the password used when exporting the `.p12`
- `APPLE_API_KEY_ID`: the API key ID
- `APPLE_API_ISSUER_ID`: the API issuer ID
- `APPLE_API_PRIVATE_KEY`: the full contents of the `.p8` file

The GitHub CLI can add them without putting secret values in the repository:

```text
base64 -i DeveloperID.p12 | gh secret set MACOS_CERTIFICATE
gh secret set MACOS_CERTIFICATE_PASSWORD
gh secret set APPLE_API_KEY_ID
gh secret set APPLE_API_ISSUER_ID
gh secret set APPLE_API_PRIVATE_KEY < AuthKey_KEYID.p8
```

## Publish a version

Use a three-part version tag:

```text
git tag -a v0.1.0 -m "Netprobe 0.1.0"
git push origin v0.1.0
```

The tag becomes the app version. GitHub publishes `Netprobe-0.1.0-macOS-universal.dmg` and its SHA-256 checksum after Apple accepts the notarization. The disk image contains `Netprobe.app` and an Applications shortcut for drag-to-install setup.
