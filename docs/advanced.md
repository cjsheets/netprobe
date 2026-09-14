# Advanced usage

## Chart scale

The dashboard uses a linear latency scale by default. Enable **Log scale** when a large spike makes the normal latency band difficult to read. Packet-loss markers remain separate from successful latency measurements.

## Incident classification

Classifications are deliberately worded as **likely**. They summarize the available evidence; they are not proof.

- **local network**: a `local` gateway and downstream targets fail together.
- **ISP or upstream**: a local target remains reachable while `public` targets fail.
- **VPN or work network**: public connectivity stays healthy while `vpn` or `work` targets fail.
- **remote service**: public connectivity stays healthy while the relevant `service` probe fails.
- **ambiguous**: coverage is insufficient or the evidence conflicts.

Failures and large latency increases within `incident_window` are grouped. The database retains affected probes, loss, route and source details, nearby markers, and a plain-language rationale.

## VPN and route diagnosis

Every successful connection records the resolved destination, selected source address, derived interface, and a compact route. A source or route change becomes a `network_change` event shown beside incidents and in reports.

Configure targets on both sides of a VPN: your router and a public endpoint outside the tunnel, plus work DNS, hosts, or services inside it. Without both sides, VPN and ISP failures are usually ambiguous.

## ICMP permissions

ICMP failure never stops TCP, DNS, or HTTP probes.

- **Windows:** the build calls the native IP Helper API (`IcmpSendEcho`) and normally needs no elevation.
- **macOS:** unprivileged datagram ICMP sockets are used. If endpoint security blocks them, allow the signed `netprobe` executable in that product.
- **Linux:** enable unprivileged ping sockets for the collector's group with `net.ipv4.ping_group_range`. If policy forbids that, grant only `cap_net_raw` to the installed executable.

The exact socket error and the narrow remediation are recorded as `permission`; other probes continue.

## Storage and privacy

Raw observations default to 14 days, minute rollups to one year, and incidents and markers indefinitely. Sleep, wake, clock shifts, collector downtime, and route changes are stored separately and do not count as packet loss.

At one-second intervals, allow roughly 15–35 MB per target per 14 days. Normal CPU use is low. HTTP and TLS probes should use longer intervals; one-second sampling is intended for small ICMP, TCP, and DNS probes.

Exports contain configured target metadata and probe results. They do not inventory the host, cookies, response bodies, DNS answers, or TLS secrets. URLs and hostnames may still be sensitive, so review reports before sharing.

## Troubleshooting

- **All ICMP rows show permission:** follow the platform guidance above and verify that TCP or DNS probes continue.
- **Everything looks like an outage after sleep:** check for a `sleep_or_wake` event. That interval is excluded from incident input.
- **VPN diagnosis is ambiguous:** add public and internal work targets, then verify their stored source and interface values.
- **Database busy:** use only one `run` process as the collector. Reports and watchers may read at the same time.
- **HTTP fails while browsing works:** probes intentionally bypass proxy discovery to measure the configured route.
