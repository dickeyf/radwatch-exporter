# Deploy guide: radwatch-exporter on Kubernetes

A reference deployment for a home lab, written while doing it (IMPL-29, IMPL-37). All addresses, hostnames and IDs are placeholders: replace them with yours.

Reference stack: Flux, Traefik, external-dns (Cloudflare), Cilium with BGP LoadBalancer IPs, cert-manager, Prometheus Operator CRDs, pfSense as router and firewall. The counter is a GQ GMC-500+ on a separate IoT VLAN.

> Status: skeleton. Sections marked _TBD_ are filled in as the reference deployment is done.

## How it fits together

```
GMC-500+ (IoT VLAN, 192.168.1.50)
  │  HTTP GET http://radwatch.example.com/log2.asp?...   (port 80, plain HTTP)
  │  DNS: radwatch.example.com → 10.10.10.10 (Cloudflare DNS-only record, by external-dns)
  ▼
pfSense: routes IoT VLAN → LoadBalancer subnet, firewall allows TCP 80 to 10.10.10.10
  ▼
Traefik (LoadBalancer IP 10.10.10.10, announced by Cilium BGP, externalTrafficPolicy: Local)
  │  Ingress: host radwatch.example.com, path /log2.asp only, entrypoint "web" (no HTTPS redirect)
  ▼
radwatch-exporter Service :80 → pod :8080        Prometheus ← ServiceMonitor → Service :9090
```

## 1. Prerequisites

_TBD_: versions and anything specific about each component.

- Flux (Helm controller and source controller)
- Traefik as the Ingress controller, with a `web` (port 80) entrypoint
- external-dns watching Ingresses, with a Cloudflare token for the zone
- Cilium with BGP control plane and a LoadBalancer IP pool
- cert-manager (only if TLS is wanted for other clients; the counter itself uses HTTP)
- Prometheus Operator CRDs (for the ServiceMonitor)

## 2. Network

_TBD_

- pfSense: the LoadBalancer subnet (virtual, announced by Cilium BGP), routing from the IoT VLAN.
- Firewall rule: IoT VLAN → Traefik LoadBalancer IP, TCP 80.
- DNS: the counter resolves through pfSense. A public name resolving to a private IP is blocked by DNS rebinding protection: add a private-domain exception for the zone, or a host override for the name.
- DHCP reservation for the counter, so the source-IP allowlist stays valid.
- Counter WiFi: a 2.4 GHz WPA2-only network (no WPA3/transition mode, PMF optional or off, no band steering or fast roaming).

## 3. Traefik

_TBD_

- Serve the host on the `web` entrypoint. A global `web` → `websecure` redirect breaks the counter: exclude this host, or use an entrypoint without it.
- `externalTrafficPolicy: Local` on Traefik's LoadBalancer Service, so the counter's IP reaches the exporter in `X-Forwarded-For`.
- Traefik's pod CIDR goes into the exporter's `config.trustedProxies`.

## 4. Flux resources

_TBD_: adjust API versions and layout to the Flux version and repo structure in use.

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: radwatch-exporter
  namespace: radwatch
spec:
  interval: 1h
  url: oci://ghcr.io/dickeyf/charts/radwatch-exporter
  ref:
    semver: "0.1.x"
  layerSelector:
    mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
    operation: copy
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: radwatch
  namespace: radwatch
spec:
  interval: 30m
  chartRef:
    kind: OCIRepository
    name: radwatch-exporter
  values:
    config:
      allowedCIDRs: ["192.168.1.50"]        # the counter
      trustedProxies: ["10.0.0.0/8"]        # Traefik pod CIDR
      deviceNames: {"0034021": home}
      gmcmap:
        allowedGIDs: ["0034021"]
    ingress:
      enabled: true
      className: traefik
      host: radwatch.example.com
      annotations:
        external-dns.alpha.kubernetes.io/cloudflare-proxied: "false"
        traefik.ingress.kubernetes.io/router.entrypoints: web
    serviceMonitor:
      enabled: true
```

## 5. Point the counter at it

_TBD_

On the counter, Menu → Server: Website `radwatch.example.com`, URL `log2.asp`. The Website value is sent as the `Host` header, so it must match `ingress.host`.

## 6. Verify

_TBD_

- Flux: `flux get helmreleases -n radwatch`
- Pod logs: a `reading accepted` line every upload interval, with `client_ip` = the counter's address (not Traefik's).
- Metrics: `radwatch_cpm{device="home",...}` on the Service's port 9090; the target is up in Prometheus.

## 7. Troubleshooting

_TBD_: what went wrong during the reference deployment, and how it was fixed.
