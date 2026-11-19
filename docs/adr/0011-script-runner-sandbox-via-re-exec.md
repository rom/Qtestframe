# 11. Script runner sandbox via re-exec (`sandbox-exec`), disabled by default

Date: 2026-11-19

## Status

Accepted

## Context

Automated test cases may carry a shell, Python or JavaScript script. Running
them on the server gives instant results without a CI system, but it is
remote code execution by design. Restrictions applied with Landlock,
seccomp and rlimits are per process and irreversible, so they cannot be
applied in the server process just for one child, and Go cannot run code
between `fork` and `exec`.

## Decision

* The runner (`internal/runner`) is **off by default** (`runner.enabled:
  false`) and its endpoint requires `script.execute` (manager, admin).
* With `runner.sandbox: true` (default) the server re-executes its own
  binary as `qtestframe sandbox-exec --workdir DIR --cpu N -- INTERPRETER
  SCRIPT`. That process applies rlimits (CPU, file size 64 MiB, 256 open
  files, no core dumps), no_new_privs, capability dropping, Landlock
  (read-only system directories, read-write only the job directory) and the
  seccomp deny-list, then `execve`s the interpreter.
* Each run gets a fresh directory below `runner.work_dir`; the script is
  written there and passed as an argument, never executed directly. The
  environment is reduced to `PATH`, `HOME`, `TMPDIR`, `LANG` and `QTF_*`
  run metadata; stdin is empty.
* The child runs in its own process group; on timeout the whole group is
  killed with SIGKILL. Output is capped at `runner.max_output_bytes`.
* MAC policies put interpreters into a separate domain/profile
  (`qtestframe_runner_t`, `qtestframe//runner`) without network access.

## Consequences

* The sandbox needs no extra binary or privileges and works wherever the
  server's own hardening works; it also inherits the server's Landlock
  domain and seccomp filter.
* The in-process layer does not block network access for scripts:
  `sandbox-exec` supports `--no-network`, but the runner does not pass it.
  Network isolation comes from SELinux/AppArmor (or a firewall).
* Landlock leaves `/etc` readable to scripts, including
  `/etc/qtestframe`; file permissions and MAC must protect configuration
  and keys. The database directory is not readable.
* Runs are synchronous HTTP requests and limited by `server.write_timeout`.
* With `runner.sandbox: false` the interpreter is started directly with only
  the timeout, output cap and process group; this is meant for tests and
  development.
