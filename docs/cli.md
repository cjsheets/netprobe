# CLI and configuration

Build the command-line application:

```text
go build -trimpath -ldflags "-s -w" -o netprobe ./cmd/netprobe
```

Copy `netprobe.example.yaml` to the normal configuration directory:

- macOS: `~/Library/Application Support/netprobe/config.yaml`
- Windows: `%AppData%\netprobe\config.yaml`
- Linux: `~/.config/netprobe/config.yaml`

Any location can be used with `--config PATH` or `NETPROBE_CONFIG`.

```text
netprobe config check
netprobe run
# In another terminal:
netprobe watch
netprobe mark "video froze during customer call"
netprobe incidents --since 2h
netprobe report --since 2h --format html --output incident.html
```

Stop the collector with Ctrl-C or a normal termination signal.

## Commands

```text
netprobe [--config PATH] run
netprobe [--config PATH] watch [--once] [--width 80] [--ascii] [--no-color]
netprobe [--config PATH] status
netprobe [--config PATH] mark [message]
netprobe [--config PATH] incidents [--since 1h | --from RFC3339 --to RFC3339] [--scope SCOPE]
netprobe [--config PATH] report [--incident ID | time flags] --format html|json [--output FILE]
netprobe [--config PATH] export [time flags] --format csv|json [--output FILE]
netprobe [--config PATH] config check
```

Time flags accept Go durations such as `30s`, `15m`, `2h`, and `336h`. RFC3339 timestamps always include a timezone. `watch --once` works well in scripts and non-interactive terminals. `--ascii` replaces Unicode plots; loss always uses `X`, so meaning never depends on color.

## Default configuration

```yaml
database: netprobe.db
concurrency: 32
incident_window: 5s

targets:
  - name: home-router
    host: 192.168.1.1
    type: icmp
    interval: 1s
    timeout: 800ms
    tags: [local]

  - name: cloudflare
    host: 1.1.1.1
    type: icmp
    interval: 1s
    timeout: 800ms
    tags: [public]

  - name: google
    host: 8.8.8.8
    type: icmp
    interval: 1s
    timeout: 800ms
    tags: [public]

  - name: quad9
    host: 9.9.9.9
    type: icmp
    interval: 1s
    timeout: 800ms
    tags: [public]
```

Replace `192.168.1.1` if your gateway uses another address. The public addresses are anycast and usually route to nearby sites.

## Configuration reference

Global fields are `database`, `concurrency` (1–1024), `raw_retention`, `rollup_retention`, and `incident_window`. Each target has:

| Field | Meaning |
|---|---|
| `name`, `description` | Unique display name and optional explanation |
| `type` | `icmp`, `tcp`, `dns`, or `http` |
| `host` | ICMP/TCP host; required for those types |
| `port` | TCP port, 1–65535 |
| `resolver`, `query` | DNS server (port defaults to 53) and lookup name |
| `url` | Complete `http://` or `https://` URL |
| `interval`, `timeout` | Positive durations; timeout may not exceed interval |
| `source`, `interface` | Optional source IP and interface label |
| `enabled` | Defaults to true |
| `tags` | Diagnostic roles: `local`, `public`, `vpn`, `work`, `service` |

Unknown fields, duplicate names, missing type-specific settings, invalid URLs and addresses, and unsafe duration relationships produce field-specific validation errors.

## Export schemas

JSON uses top-level schema `netprobe.export.v1`, UTC RFC3339 timestamps, `targets`, `observations`, minute `rollups`, `incidents`, `markers`, and `gaps`. CSV contains raw observations with this fixed v1 header:

```text
schema,timestamp_utc,target,probe_type,success,latency_ms,error_category,destination_address,source_address,interface,route
```

HTML reports embed their data, CSS, and JavaScript. They make no network requests and open directly from disk.
