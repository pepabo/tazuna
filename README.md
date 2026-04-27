# tazuna

> Take the reins of your multi-cluster Kubernetes.

`tazuna` is a CLI tool that owns the bootstrap lifecycle of a Kubernetes cluster — from a fresh control plane to a production-ready state. It consolidates manifest application, state reconciliation, and secret integration behind a single declarative configuration file.

You describe the manifests to apply (and how to render them: `kustomize`, `helmfile`, or a custom plugin) in a `tazuna.yaml`, then run `tazuna apply` to roll them out in order.

## Features

- **Declarative cluster bootstrap** — express the initial cluster composition in a single `tazuna.yaml`
- **Multiple manifest backends** — kustomize / helmfile / pluggable types
- **State management** — `state list`, `state diff`, `state sync` make drift visible
- **Tag-based filtering** — apply a subset of a large config with `--tags`
- **Secret integration** — generate and manage `GenesisSecret` resources backed by 1Password

## Installation

### Pre-built binaries

Download the archive for your OS/Arch from [Releases](https://github.com/pepabo/tazuna/releases) and place the `tazuna` binary on your `PATH`.

### From source

```bash
go install github.com/pepabo/tazuna@latest
```

Or clone the repository:

```bash
make build      # produces ./tazuna
make install    # installs to /usr/local/bin (uses sudo)
```

## Quickstart

```yaml
# tazuna.yaml
apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
    - name: "ingress-nginx"
      type: kustomize
      path: ./kustomize/ingress-nginx
      tags:
        - infra
```

```bash
tazuna check                   # validate tazuna.yaml
tazuna build                   # render manifests to stdout
tazuna apply                   # apply to the current cluster
tazuna apply --tags infra      # apply only matching tags
tazuna state list              # list managed resources
tazuna state diff              # show drift against the cluster
tazuna destroy                 # remove tazuna-managed resources
```

See `tazuna --help` for the full subcommand reference.

## Development

```bash
make format             # gofmt
make test               # unit tests
make test-integration   # integration tests (build tag: integration)
make test-e2e           # end-to-end tests (requires a KinD cluster)
make test-all           # all layers
make lint               # golangci-lint

make devenv-create      # spin up a KinD cluster for E2E
make devenv-destroy     # tear it down
```

## License

[MIT License](./LICENSE)
