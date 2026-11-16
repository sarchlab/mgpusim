# Agent instructions

Follow [CLAUDE.md](CLAUDE.md) for build, test, lint and architecture guidance, and
the [Akita v5 architecture rules](https://github.com/sarchlab/akita/blob/main/doc/architecture.md).

## Checkpoints

A checkpoint must restore exactly when it is loaded by the same executable on
the same platform. Across executables, for example after a code or configuration
change, loading is best effort: if it loads, it is used; otherwise loading
reports an error. Breaking checkpoint compatibility is never a concern when
changing code, and checkpoints carry no version or configuration-hash checks.
