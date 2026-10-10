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
Traefik (LoadBalancer IP 10.10.10.10 from a Cilium BGP pool; k3s-bundled)
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
- Firewall rule: IoT VLAN → Traefik LoadBalancer IP, TCP 80. With no source-IP allowlist in the app (section 3), keep this rule narrow: only the counter's address, only port 80.
- DNS: the counter resolves through pfSense. A public name resolving to a private IP is blocked by DNS rebinding protection: add a private-domain exception for the zone, or a host override for the name.
- DHCP reservation for the counter, so the source-IP allowlist stays valid.
- Counter WiFi: a 2.4 GHz WPA2-only network (no WPA3/transition mode, PMF optional or off, no band steering or fast roaming).

## 3. Traefik

The reference cluster uses the Traefik bundled with k3s, as the default IngressClass `traefik`.

- **No HTTPS redirect on `web`.** Check the entrypoint args: `kubectl -n kube-system get deploy traefik -o jsonpath='{.spec.template.spec.containers[0].args}'`. A global `web` → `websecure` redirect breaks the counter; if you have one, exclude this host or route it through an entrypoint without it.
- **Client IP and `externalTrafficPolicy`.** Check it with `kubectl -n kube-system get svc traefik -o jsonpath='{.spec.externalTrafficPolicy}'`.
  - `Local`: the counter's address reaches Traefik and is passed on in `X-Forwarded-For`. Put Traefik's pod CIDR in `config.trustedProxies` and the counter's IP in `config.allowedCIDRs`.
  - `Cluster` (the k3s default, and the reference cluster): the source address can be rewritten to a node IP when traffic crosses nodes, so a source-IP allowlist would reject the counter. The reference deployment **skips `allowedCIDRs`** and relies on the GID allowlist, the pfSense firewall rule and the name existing only in private DNS. Switching Traefik to `Local` (k3s `HelmChartConfig`) affects every service behind it.

## 4. Flux resources

The reference repo is a Flux monorepo: one directory per app under `clusters/<cluster>/`, picked up automatically by the cluster's root Kustomization, and the chart sources in `flux-system/helm-repositories.yaml`. Written for Flux 2.3 (`HelmRelease` v2; `OCIRepository` is only `v1beta2` there, so the chart comes through an OCI `HelmRepository`).

Chart source, added to `flux-system/helm-repositories.yaml`:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: radwatch
  namespace: flux-system
spec:
  type: oci
  interval: 1h
  url: oci://ghcr.io/dickeyf/charts
```

App directory `clusters/<cluster>/radwatch/`:

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: radwatch
resources:
- namespace.yaml
- helm-release.yaml
```

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: radwatch
```

```yaml
# helm-release.yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: radwatch
spec:
  interval: 15m
  chart:
    spec:
      chart: radwatch-exporter
      version: "0.1.x"
      sourceRef:
        kind: HelmRepository
        name: radwatch
        namespace: flux-system
      interval: 1h
  values:
    config:
      # allowedCIDRs: ["192.168.1.50"]   # only with externalTrafficPolicy: Local (section 3)
      trustedProxies: ["10.0.0.0/8"]     # pod CIDR: logs show the address Traefik saw
      deviceNames: {"0034021": home}
      gmcmap:
        allowedGIDs: ["0034021"]
    ingress:
      enabled: true
      className: traefik
      host: radwatch.example.com
      annotations:
        # Private name: point the record at Traefik's private LoadBalancer IP, DNS-only.
        external-dns.alpha.kubernetes.io/target: 10.10.10.10
        external-dns.alpha.kubernetes.io/cloudflare-proxied: "false"
        traefik.ingress.kubernetes.io/router.entrypoints: web
    serviceMonitor:
      enabled: true
      labels:
        # Must match the Prometheus CR's serviceMonitorSelector; check with:
        # kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceMonitorSelector}'
        release: kube-prometheus-stack
```

The `external-dns.alpha.kubernetes.io/target` annotation matters when Traefik has several addresses (e.g. a private IPv4 and a public IPv6): without it, external-dns publishes all of them.

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
