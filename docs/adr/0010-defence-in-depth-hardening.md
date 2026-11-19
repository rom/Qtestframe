# 10. Defence-in-depth hardening: systemd and MAC outside, no_new_privs/caps/Landlock/seccomp inside

Date: 2026-11-19

## Status

Accepted

## Context

The server parses untrusted input (HTTP, XML from IdPs, query
expressions, export data) and can optionally run user-supplied scripts.
Deployments differ: some hosts have systemd and SELinux or AppArmor, some
run the binary in a container or under another init system. A single
layer of confinement is either missing in some environments or must be
perfect.

## Decision

Layers outside the process, shipped in `deploy/`:

* systemd unit with `ProtectSystem=strict`, private `/tmp` and devices,
  `NoNewPrivileges`, `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`,
  `RestrictNamespaces`, `MemoryDenyWriteExecute`, `SystemCallFilter`
  allow-list, resource limits;
* an SELinux policy module (`qtestframe_t`, a separate
  `qtestframe_runner_t`, booleans for optional network access) and an
  AppArmor profile with a `runner` child profile;
* nftables rules, sysctl settings, a container seccomp allow-list and
  read-only distroless container.

Layers inside the process (`internal/hardening`, applied by `serve` after
all start-up files are open, configurable under `hardening:`):

* `PR_SET_NO_NEW_PRIVS` on all threads;
* dropping all capabilities except `CAP_NET_BIND_SERVICE` (bounding set when
  `CAP_SETPCAP` is held, effective/permitted/inheritable on all threads,
  ambient cleared);
* **Landlock** (ABI v5, best effort): read-only access to the paths the
  server needs after start-up (`hardening.DefaultReadOnly`, TLS and IdP
  files, credentials), read-write only to the SQLite directory, the temp
  directory, the runner work dir and configured paths, execute paths only
  when the runner is enabled;
* a **seccomp deny-list** (`hardening.DeniedSyscalls`: mount family,
  module loading, kexec, bpf, ptrace, process_vm_*, unshare/setns, keyring,
  reboot, swap, clock setting, personality, userfaultfd, perf_event_open,
  ...) returning `EPERM`, installed with TSYNC.

Unsupported features are reported as skipped in the start-up log line
`hardening applied=[…] skipped=[…]`; they are not fatal. On non-Linux
systems all of this is a no-op.

## Consequences

* Protection exists even without systemd or MAC, and stacks with them
  when present.
* A deny-list is weaker than an allow-list but robust against Go runtime
  and library changes; the strict allow-lists live in the systemd unit and
  the container seccomp profile, where operators can adjust them.
* Landlock means every new file the server needs after start-up must be
  added to the rules; forgotten paths fail with `EACCES`. Development
  configurations turn Landlock off (`scripts/dev.sh`).
* Release builds must use `CGO_ENABLED=0` so that prctl/capset reach every
  thread.
* The hardening is verified in tests that run in child processes
  ([testing.md](../testing.md#5-hardening-tests-in-child-processes)).
