# Contributing

The shared Go packages separate configuration, scheduling, probes, storage, incident analysis, terminal rendering, and reporting. Platform ICMP implementations live behind build tags. The macOS app uses the Go executable as a bundled helper.

Run the tests before submitting a change:

```text
go test ./...
go vet ./...
```

No automated test reaches the public internet. Check the command-line builds with:

```text
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/netprobe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/netprobe
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/netprobe
```

Native ICMP should also be tested on its target operating system because socket policy is controlled by the host.

Build the macOS application with:

```text
./scripts/build-macos-app.sh
```

Create the distributable disk image after building the app with:

```text
./scripts/build-macos-dmg.sh
```

Development builds are ad-hoc signed. For a Developer ID build, provide the exact identity reported by `security find-identity -v -p codesigning`:

```text
NETPROBE_VERSION=0.1.0 \
NETPROBE_CODESIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" \
./scripts/build-macos-app.sh
```

A Developer ID build still needs Apple notarization before public distribution.
