# 06 — Security and open questions

Partly answered in Phase 2 (see **Answers** below). Answer the rest with the real device (packet capture, request logs) or from GQ documentation, then record the answer here and update affected decisions.

## Open questions about the device

| ID | Question | Why it matters | How to answer |
|----|----------|----------------|---------------|
| Q-01 | Does the GMC-500+ support HTTPS to the server? Does it validate certificates? | Decides whether TLS on the Ingress helps the device at all, and whether HTTP→HTTPS redirects must be disabled | Try `https://` / port 443; watch Ingress logs |
| Q-02 | Does it support any authentication (basic auth, token, header)? | Decides whether the Secret is needed | Check device menus/firmware docs; inspect request headers |
| Q-03 | Can the Website field contain a port (`host:8080`)? | Decides whether port 80 is mandatory | Try it |
| Q-04 | Can the URL field contain an arbitrary path with subfolders and a query string? How long? | A long random path could act as a shared secret | Try `ingest/abc123...` |
| Q-05 | What response does it expect? Does it retry or show an error on non-200? | Defines the reply body and status codes; decides whether rejecting looks like a device error | Reply with different codes/bodies, watch the device screen |
| Q-06 | Exact parameter set and casing sent by this firmware version (extra fields like `DOSE`, temperature, etc.)? | Parser and metric set | Log raw query strings |
| Q-07 | Reporting interval — fixed at 1 min or configurable? | Scrape interval and "silent device" alert threshold | Observe timestamps |
| Q-08 | Does it send a `Host` header that matches the Website field? Does it use DNS or only IPs? | Ingress host-based routing needs a correct `Host` header and LAN DNS | Inspect request headers |
| Q-09 | Static or DHCP IP for the counter? | Source-IP allowlist needs a stable IP (DHCP reservation) | Router config |
| Q-10 | Does the firmware send the request through any proxy or from a different source than its own IP? | Source-IP allowlist validity | Packet capture |
| Q-11 | Does `uSV` stay in µSv/h above 1 mSv/h, or does the firmware switch units (e.g. `uSV=3.00` meaning 3 mSv/h) like the screen may? | A silent unit switch would make the dose-rate metric 1000× too low; a unit suffix would be rejected as `bad_request` | Expose the counter to > 1 mSv/h (unbagged high-grade ore at contact) with `RADWATCH_DEBUG_REQUESTS=true` and read the raw `uSV=` value |

## Answers (Phase 2, GMC-500+ on the LAN, 2026-10-09)

Captured with `RADWATCH_DEBUG_REQUESTS=true`. Example request as received (addresses and IDs replaced with placeholders):

```
GET /log2.asp?AID=0230111&GID=0034021&CPM=8&ACPM=14.89&uSV=0.05 HTTP/1.1
Host: 192.168.1.10
Accept: */*
Connection: close
```

| ID | Answer |
|----|--------|
| Q-01 | Not tested. The device only offers a host and a path and talks plain HTTP on port 80, so HTTPS is moot for it; TLS on the Ingress only helps other clients |
| Q-02 | No authentication: no `Authorization` or other credential header is sent |
| Q-03 | **No port.** The Website field takes a hostname or IP only; `host:8080` did not work. The device always calls port 80 |
| Q-04 | Not tested yet |
| Q-05 | No visible error on the device with the default `200 OK.ERR0` reply over a one-hour run. Behaviour on non-200 replies not tested (AD-13 keeps honest 4xx; only rejected uploads get them) |
| Q-06 | Exactly `AID`, `GID`, `CPM`, `ACPM`, `uSV`, in that casing, no extra fields. `uSV` has two decimals (steps of 0.01 µSv/h). `ACPM` stayed at 14.89 across the first readings while `CPM` moved: keep watching |
| Q-07 | Every 2 minutes, as configured on the device. Over one hour: 30 uploads, no gaps, intervals between 1:56 and 2:08 |
| Q-08 | **The device does not resolve hostnames**: with a name as Website it sends no DNS query at all, so Website must be an IP (found during the cluster deployment). `Host` is exactly the Website value. Only other headers: `Accept: */*`, `Connection: close`. No `User-Agent`. HTTP/1.1, one connection per upload |
| Q-09 | DHCP, on a separate IoT VLAN; give it a reservation before using the source-IP allowlist |
| Q-10 | No proxy or NAT: requests arrive from the counter's own address across VLANs, so a source-IP allowlist works |
| Q-11 | Open, deferred: to be tested later (> 1 mSv/h needs the ore unbagged). Up to 272 µSv/h the device sent plain µSv/h (`uSV=255.14`, `uSV=270.88`, with ~42–45k CPM). Expected to stay µSv/h because the parameter has no unit and gmcmap.com reads it as µSv/h. Fallback if it switches: warn when CPM and dose rate disagree by orders of magnitude (this unit runs at ~150–170 CPM per µSv/h) |

**One-hour run.** 30 consecutive uploads, all accepted (`200 OK.ERR0`). A test source (natural uranium ore) next to the counter for ~14 minutes gave a steady ~45,000 CPM / ~270 µSv/h (about 167 CPM per µSv/h, vs ~150 at background); CPM fell back to background in the first upload after removing it, so `CPM` is a per-minute value with no long smoothing. `ACPM` only moved during the exposure (+1.0 in total, ~0.15 per upload) and was otherwise constant at two decimals, so it is a very long-term average (likely since power-on or longer), not a recent one: graph `CPM`, not `ACPM`, for current levels.

**WiFi.** The counter would not join the main home network (2.4/5/6 GHz, single name) but joined the IoT network. Its WiFi stack is 2.4 GHz-only and old; WPA3/transition mode, required PMF, 802.11r, band steering and WiFi 6/6E beacon extras commonly break such devices. Use a 2.4 GHz WPA2-only network with PMF optional or off, no fast roaming and no band steering.

**Consequences.** The device must be given an IP, so it cannot use a host-based Ingress rule: it reaches the exporter through a dedicated LoadBalancer Service (`ingestService`, PD-06). The stale timeout (5 min) survives one missed upload; the silence alert threshold should be 5–10 min (PD-07). If an optional Ingress is also used for other clients, it must serve plain HTTP on port 80 for the ingest path with no HTTPS redirect.

## Threat model (short)

- The ingest endpoint is reachable from the LAN (and possibly more, depending on how the Ingress is exposed).
- The device most likely has **no authentication**, and AID/GID are sent in clear text. Anyone who can reach the endpoint can inject fake readings.
- Impact: misleading graphs, false alerts (or suppressed silence alerts), label-cardinality abuse if arbitrary GIDs create new series.
- No secrets or user data are at stake; the risk is integrity of the data and resource abuse.

## Restricting what the ingest endpoint accepts

Layered options, roughly from cheapest to strongest. Choosing the combination is AD-04.

1. **Don't expose it to the Internet.** The ingest Service gets a private LoadBalancer IP, reachable only from the LAN (PD-06); an optional Ingress is LAN-only too.
2. **Strict routing.** Only the exact ingest path, GET only; everything else 404 (in the app on the ingest Service; at the Ingress for the optional name-based path).
3. **Source-IP allowlist.** Give the counter a DHCP reservation and allow only that IP.
   - In the app (`RADWATCH_ALLOWED_CIDRS`). Through the ingest Service with `externalTrafficPolicy: Local`, the app sees the counter's own address. Through an Ingress, the app sees the controller's IP and must read `X-Forwarded-For` **only** from trusted proxy CIDRs (`RADWATCH_TRUSTED_PROXIES`), and the controller's Service must preserve client IPs.
   - At the load balancer (`ingestService.loadBalancerSourceRanges`) or the Ingress (controller annotation).
4. **ID allowlist.** Accept only known GID (and AID). Weak on its own (IDs are visible on the wire) but prevents unbounded label creation.
5. **Path-as-secret.** If Q-04 allows it, use a long random URL path (e.g. `/ingest/9f3c…`) stored in the Secret. Clear-text over HTTP, so only as strong as LAN confidentiality.
6. **Input validation.** Numeric parse with sane bounds (e.g. CPM 0–1,000,000, µSv/h 0–10,000), reject otherwise; cap query-string length.
7. **Rate limiting.** Per source IP, e.g. a few requests per minute; at the Ingress or in-app.
8. **NetworkPolicy.** Port `http` only from the counters' CIDRs (traffic via the ingest Service arrives with their addresses) and/or the Ingress controller; port `metrics` only from the scraper.

Decided in ADR-009 (AD-04): 3 (in the app, with trusted-proxy handling) + 4 + 6 are implemented; 1 and 2 come with the chart (Phase 4); 8 is IMPL-33 (Phase 6); 5 waits for Q-04; 7 is IMPL-35.

## TLS notes

- The chart supports TLS on the Ingress using an **existing** TLS Secret in the release namespace (created by External Secrets, cert-manager, or by hand). The chart only references it by name.
- If the device is HTTP-only (Q-01), TLS still protects other clients (e.g. you testing with curl), but the device's own traffic stays in clear text. HTTP→HTTPS redirect must be off for the ingest path.
- `/metrics` stays in-cluster on its own port; TLS/basic-auth on it (via `exporter-toolkit`) is optional and probably unnecessary (AD-10).
