These notes were written by AI. It's been a long day and I haven't bothered
to vet them yet. Feel free to read, ignore, roll your eyes - it's your call.

## The handful of things that make it work

A MicroVM has one inbound channel (its endpoint), no inbound TCP, and cannot
route to other MicroVMs. Everything below exists to work within that.

1. **Two images, one Dockerfile, one entrypoint.** Both images are built from the
   same context; they differ only by the image-level env var `$ROLE` baked at
   build time (`build.sh` → `create-microvm-image --environment-variables ROLE=...`),
   and the single `start` entrypoint
   dispatches on it. So there is no run hook and no per-VM payload at boot.

2. **Guest prep (every node), baked in `base-prep`.** k3s needs cgroup-v2
   controllers, but the root cgroup holds the boot processes, so we do the
   **delegation dance** (move them into `/sys/fs/cgroup/init`, then enable
   controllers in `subtree_control`). `/dev/kmsg` returns `EPERM` even with all
   caps, so we replace it with a **FIFO shim** kubelet can open.

3. **Image needs all OS capabilities.** `build.sh` always creates images with
   `--additional-os-capabilities ALL` (`AdditionalOsCapabilities=ALL`) and an
   8 GiB memory floor; containerd/runc/k3s require the caps.

4. **worker → apiserver:** each worker runs `tundial` (started by `node-join`)
   bridging to the cp's `:6443` over the token-WS. **Gotcha:** k3s agents
   discover the *real* apiserver address (the cp's node IPv6, unroutable between
   MicroVMs) and switch to it. Fix: alias the cp's IPv6 onto each worker's `lo`
   and bind `tundial` there, so the discovered address resolves to the local
   tunnel. The one thing a worker can't know until launch — the cp's MicroVM id
   and IPv6 — is supplied by `up.sh` via one `/exec` call to `node-join`.

5. **laptop → apiserver:** same `tundial`, plus a kubeconfig rewritten to
   `https://127.0.0.1:6443`. The serving cert carries `127.0.0.1`
   (`--tls-san=127.0.0.1`), so TLS validates.

6. **native ingress reaches the MAIN netns only.** `X-aws-proxy-port: N` is
   proxied to port `N` in the MicroVM's primary network namespace — not into pod
   netns. So `nginx.sh` runs nginx as a **hostNetwork** pod (binds `:80` on the
   node) and curls it directly over the native ingress (a port-scoped token + the
   `X-aws-proxy-port: 80` header).

## What the MicroVM kernel allows (hard-won lessons)

Tested on `6.1.x amzn2023 aarch64` with `AdditionalOsCapabilities=ALL`:

- **No L3 overlay is possible.** `/dev/net/tun` (TUN/TAP) is absent and
  vxlan / wireguard / geneve / gre / ipip / sit are all unsupported; only **veth**
  and **AF_PACKET** work. Anything that needs a tun device (wireguard-go,
  Tailscale) or vxlan (flannel VXLAN) is out — this is *why* cross-node pod
  networking is hard, and why the only way to move packets is an L2 bridge over
  the endpoint.
- **WebSocket is the only full-duplex transport through the endpoint.** The
  MicroVM's L7 proxy carries a WebSocket but **not** raw HTTP `CONNECT`/hijack —
  that's why `tundial`/`microtunnel` tunnel over a WebSocket rather than CONNECT.
- **The cgroup-v2 hurdle is the "no internal processes" rule, not a permanent
  limit.** Once the delegation dance (`base-prep`) is done it stays fixed, and
  **resource limits are genuinely enforced** — `memory.max` / `cpu.max` were
  verified set, so both BestEffort and CPU/memory-limited pods run normally.

## Known limitations

- **No cross-node pod↔pod networking.** Removed for simplicity; pods on the same
  node still talk via the local CNI bridge.
- **ClusterIP Services / kube-proxy do not work.** The MicroVM kernel can't
  create the `filter`/`mangle` netfilter tables kube-proxy needs — the repeating
  `ip6tables ... TABLE_ADD failed (Operation not supported)` lines in the agent
  log are this, **not** harmless noise — so service VIPs are never programmed.
  CoreDNS is also disabled. A fix would need an eBPF dataplane (e.g. Cilium
  kube-proxy replacement); this is also why Knative isn't feasible as-is.

## Caveats

- Every laptop→cluster byte (kubectl via `tundial`, nginx via native ingress)
  round-trips through an AWS-managed endpoint. Great for IAM-native auth and zero
  relay ops; not for high throughput.
- MicroVMs have an 8h max lifetime and auto-suspend on idle.
- Auth tokens expire ~30 min, but are only needed to *establish* a tunnel.

## If you push further

Breadcrumbs from earlier prototypes (apiserver-in-a-MicroVM, cross-node pod↔pod)
that were cut for simplicity but cost real debugging time:

- **apiserver → kubelet over the endpoint** (needed for `kubectl logs/exec/
  port-forward`): point the apiserver's `--egress-selector-config` at an
  HTTPConnect proxy that bridges over the token WebSocket. Two traps:
  HTTPConnect-over-TCP **nil-panics the apiserver without a client-cert
  `tlsConfig`** (mTLS is mandatory), and you must **never wrap `etcd`** in the
  selector (k3s's kine is a unix socket → "context deadline exceeded") — declare
  only the `cluster` selection. Make kubelet advertise the **microVM ID as its
  Hostname** (`--node-name=<id>` + apiserver `--kubelet-preferred-address-types=
  Hostname`) so the proxy reads the target ID straight off the CONNECT line — no
  IP→ID map needed.
- **Cross-node pod↔pod without a relay**: bridge an **L2 virtual wire** (veth +
  AF_PACKET) over the token WebSocket. The trap that looks like sorcery:
  AF_PACKET captures locally-generated frames with **unfilled checksums**
  (offload), the peer kernel drops them, but **ARP has no checksum so it
  resolves** — i.e. "ARP works, TCP hangs." Fix: disable veth offload
  (`ethtool -K <dev> tx off rx off tso off gso off gro off`).
- **Shell gotcha:** `pkill -f /tmp/foo` self-matches the exec shell running it
  (its command line contains the pattern); use `pkill -x` or a `[/]tmp/...` regex.
