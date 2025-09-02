# kronoform

A time-lapse camera for your Kubernetes cluster - track every `kubectl apply` and its resulting resource states

## Description

Have you ever wondered "Which YAML did I apply?" or "What was the exact state before that Pod crashed in production?" These invisible changes often become the root cause of troubles in Kubernetes operations.

Kronoform solves this by acting as a time-lapse camera for your Kubernetes cluster. Simply use `kubectl kronoform apply` instead of `kubectl apply` - it automatically records and accumulates successful YAML files along with the actual resource states immediately after application.

This provides clear visibility into "when, who, what was applied, and what was the result" at a glance. Before introducing heavyweight solutions like GitOps, Kronoform serves as a handy and powerful change history and debugging tool that removes developer anxiety.

Kronoform - bringing "rewind-able" peace of mind to your cluster.

## Getting Started

### Prerequisites

- go version v1.24.0+
- kubectl version v1.11.3+
- Access to a Kubernetes cluster with admin privileges (for CRD installation)

### Installation

**1. Install the Custom Resource Definitions (CRDs):**

```sh
make install
```

**2. Build the kubectl plugin:**

```sh
make build-plugin
```

**3. Install the plugin (optional - for system-wide usage):**

```sh
make install-plugin
```

This will copy the plugin to `/usr/local/bin/kubectl-kronoform` so you can use it as `kubectl kronoform` from anywhere.

### Quick Start

**Use kronoform instead of kubectl apply:**

```sh
# Instead of: kubectl apply -f your-manifest.yaml
kubectl kronoform apply -f your-manifest.yaml
```

**Or use the binary directly:**

```sh
./bin/kubectl-kronoform apply -f your-manifest.yaml
```

**View diffs between changes:**

```sh
kubectl kronoform diff <history-id>
```

This shows the differences between the manifest before and after applying changes.

**Test with example resources:**

```sh
# Test with ConfigMap example
kubectl kronoform apply -f config/samples/configmap_example.yaml

# Test with Deployment example
kubectl kronoform apply -f config/samples/deployment_example.yaml

# View the recorded history
kubectl get kronoformhistories
kubectl get kronoformsnapshots

# Get detailed information
kubectl describe kronoformhistory <history-name>
kubectl describe kronoformsnapshot <snapshot-name>
```

### Using kubectl apply directly (via alias)

To make it even easier to use kronoform, you can set up a shell function so that `kubectl apply` automatically uses kronoform while keeping other kubectl commands unchanged:

**Set up a shell function (recommended):**

Add this to your shell profile (e.g., `~/.bashrc`, `~/.zshrc`):

```sh
function kubectl() {
    if [[ $1 == "apply" ]]; then
        shift
        command kubectl kronoform apply "$@"
    else
        command kubectl "$@"
    fi
}
```

Then reload your shell or run `source ~/.zshrc`.

Now, `kubectl apply -f your-manifest.yaml` will automatically use kronoform to record history, while other commands like `kubectl get` work normally.

**Note:** This overrides `kubectl apply` in your shell session. If you need the original `kubectl apply`, you can use `command kubectl apply` directly.

### Features

- **Intelligent Change Detection**: Only records history when actual changes occur (skips "unchanged" operations)
- **User Tracking**: Records who applied each change
- **Snapshot Management**: Creates snapshots before applying and links them to history records
- **Namespace Support**: Works with resources in any namespace
- **Dry-run Support**: Compatible with `--dry-run` flag

### How it works

1. When you run `kubectl kronoform apply`, it first creates a snapshot record
2. Then executes the actual `kubectl apply` command
3. Analyzes the kubectl output to detect if changes occurred
4. Only creates a history record if actual changes were made
5. If no changes occurred, cleans up the snapshot to avoid clutter

### Cleanup

**Remove the CRDs and all recorded history:**

```sh
make uninstall
```

**Uninstall the plugin:**

```sh
sudo rm /usr/local/bin/kubectl-kronoform
```

## Development

### Building from source

```sh
# Build the plugin
make build-plugin

# Run tests
make test

# Generate CRDs
make manifests

# Install CRDs
make install
```

## Architecture

Kronoform consists of:

- **kubectl plugin**: The main CLI tool that wraps `kubectl apply`
- **Custom Resource Definitions (CRDs)**:
  - `KronoformSnapshot`: Records the manifest and metadata before applying
  - `KronoformHistory`: Records successful apply operations with user tracking
  - `Kronoform`: Basic CRD for the project (currently minimal)

## Distribution

### Installing from GitHub Releases

```sh
# Download the latest release for your platform
curl -L https://github.com/yu-kod/kronoform/releases/latest/download/kubectl-kronoform-darwin-amd64 -o kubectl-kronoform

# Make it executable
chmod +x kubectl-kronoform

# Move to PATH (optional)
sudo mv kubectl-kronoform /usr/local/bin/

# Install CRDs
kubectl apply -f https://github.com/yu-kod/kronoform/releases/latest/download/install.yaml
```

## Contributing

We welcome contributions! Please feel free to submit a Pull Request.

### Development Setup

1. Clone the repository
2. Install dependencies: `go mod download`
3. Build: `make build-plugin`
4. Test: `make test`

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
