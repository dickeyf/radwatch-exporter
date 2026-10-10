# Deploy guide: radwatch-exporter on Kubernetes

A reference deployment for a home lab, written while doing it (IMPL-29, IMPL-37). All addresses, hostnames and IDs are placeholders: replace them with yours.

Reference stack: Flux 2.3, k3s with its bundled Traefik, Cilium with BGP LoadBalancer IPs, Prometheus Operator CRDs (kube-prometheus-stack), pfSense as router and firewall. The counter is a GQ GMC-500+ on a separate IoT VLAN.

> Status: in progress. Sections marked _TBD_ are filled in as the reference deployment is done.

## The one constraint that shapes everything

**The GMC-500+ does not resolve hostnames.** With a name in its Website field it never even sends a DNS query; it only works with an IP. It also sends the Website value as the HTTP `Host` header, always on plain HTTP port 80.

So the counter cannot go through a host-based Ingress (an Ingress rule cannot name an IP as its host). Instead the chart's **`ingestService`** gives the exporter its own LoadBalancer IP, exposing only the ingest port:

```
GMC-500+ (IoT VLAN, 192.168.1.50)
  │  HTTP GET http://10.10.10.20/log2.asp?...   (Website = the IP, port 80)
  ▼
pfSense: routes IoT VLAN → LoadBalancer subnet; firewall allows 192.168.1.50 → 10.10.10.20:80
  ▼
radwatch-exporter-ingest Service (LoadBalancer 10.10.10.20, Cilium BGP, externalTrafficPolicy: Local)
  ▼
pod :8080 (ingest)                    Prometheus ← ServiceMonitor → ClusterIP Service :9090 (metrics, in-cluster only)
```

`externalTrafficPolicy: Local` keeps the counter's real address, so the exporter's source-IP allowlist (`config.allowedCIDRs`) works. The metrics port is never on the LoadBalancer.

An Ingress (with external-dns) is still useful for other clients that do resolve names, such as testing with curl; it is optional and not needed by the counter.

## 1. Prerequisites

- Flux (source and Helm controllers).
- A LoadBalancer implementation with a private address pool reachable from the counter's VLAN. Reference: Cilium LB-IPAM pools plus BGP advertisement of LoadBalancer IPs to pfSense.
- Prometheus Operator CRDs, for the ServiceMonitor.
- Optional: an Ingress controller and external-dns, only for name-based access by other clients.

## 2. Network

- **A free, fixed IP** in the private LoadBalancer pool for the ingest Service (here `10.10.10.20`). It goes into the counter's Website field, so it must never change: pin it in the values.
- **Routing:** the IoT VLAN must reach the LoadBalancer subnet (with BGP, pfSense learns the route).
- **Firewall:** allow the counter's address to the ingest IP on TCP 80, nothing else.
- **DHCP reservation** for the counter, so `allowedCIDRs` stays valid.
- **Counter WiFi:** a 2.4 GHz WPA2-only network (no WPA3/transition mode, PMF optional or off, no band steering or fast roaming).

## 3. Flux resources

The reference repo is a Flux monorepo: one directory per app under `clusters/<cluster>/`, picked up automatically by the cluster's root Kustomization, and the chart sources in `flux-system/helm-repositories.yaml`. Flux 2.3 has `HelmRelease` v2 but `OCIRepository` only as `v1beta2`, so the chart comes through an OCI `HelmRepository`.

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
      version: "0.2.x"
      sourceRef:
        kind: HelmRepository
        name: radwatch
        namespace: flux-system
      interval: 1h
  values:
    config:
      allowedCIDRs: ["192.168.1.50"]       # the counter (works thanks to externalTrafficPolicy: Local)
      deviceNames: {"0034021": home}
      gmcmap:
        allowedGIDs: ["0034021"]
    ingestService:
      enabled: true
      loadBalancerIP: 10.10.10.20          # what the counter's Website field holds
      annotations:
        lbipam.cilium.io/ips: 10.10.10.20  # Cilium LB-IPAM's way to pin the address
      labels:
        ipPool: private                    # whatever selects your private Cilium pool
    serviceMonitor:
      enabled: true
      labels:
        # Must match the Prometheus CR's serviceMonitorSelector; check with:
        # kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceMonitorSelector}'
        release: kube-prometheus-stack
```

Optional, for name-based access by other clients through the Ingress controller (not used by the counter):

```yaml
    config:
      trustedProxies: ["10.0.0.0/8"]       # ingress controller pod CIDR
    ingress:
      enabled: true
      className: traefik
      host: radwatch.example.com
      annotations:
        external-dns.alpha.kubernetes.io/target: 10.10.10.10   # ingress controller's private IP
        external-dns.alpha.kubernetes.io/cloudflare-proxied: "false"
        traefik.ingress.kubernetes.io/router.entrypoints: web
```

With an Ingress and `allowedCIDRs` both set, clients coming through the Ingress must also be in `allowedCIDRs`. With Traefik's default `externalTrafficPolicy: Cluster` their address may arrive as a node IP; see the README's trusted-proxy notes.

## 4. Point the counter at it

On the counter, Menu → Server: Website `10.10.10.20` (the ingest IP, no port), URL `log2.asp`.

## 5. Verify

_TBD_

- Flux: `flux get helmreleases -n radwatch`.
- The ingest Service has the pinned IP: `kubectl -n radwatch get svc` (`EXTERNAL-IP` of `radwatch-radwatch-exporter-ingest`).
- Pod logs: a `reading accepted` line every upload interval (2 minutes on the reference counter), with `client_ip` = the counter's address.
- Metrics: `radwatch_cpm{device="home",...}`; the target is up in Prometheus.

## 6. Troubleshooting

- **Counter configured with a hostname, nothing arrives:** expected; it never resolves names. Use the IP.
- **Counter configured with an IP, Ingress answers 404:** the Ingress routes by host, and the counter sends the IP as `Host`. Use `ingestService`.
- **`reading rejected` with `source not allowed`:** check `client_ip` in the log. If it is a node address, the Service is not using `externalTrafficPolicy: Local`.
