# Kronoform Project Guidelines

## Project Overview

Kronoform is a "time-lapse camera" for Kubernetes clusters - a kubectl plugin that records every `kubectl apply` operation and its resulting resource states.

**Tech Stack:**
- Go 1.24+
- Kubernetes Operator (Kubebuilder v4.7.1)
- controller-runtime
- Cobra CLI
- Ginkgo/Gomega testing

**Architecture:**
- kubectl plugin (`cmd/kubectl-kronoform/`) - Main CLI tool wrapping kubectl apply
- Kubernetes Operator (`cmd/main.go`) - Manager with controllers and webhooks
- CRDs: `KronoformSnapshot`, `KronoformHistory`, `Kronoform`

## Critical Rules

### 1. Immutability (CRITICAL)

ALWAYS use DeepCopy before modifying Kubernetes objects:

```go
// WRONG: Direct mutation
snapshot.Status.Phase = "Completed"
k8sClient.Status().Update(ctx, snapshot)

// CORRECT: DeepCopy first
snapshotCopy := snapshot.DeepCopy()
snapshotCopy.Status.Phase = "Completed"
k8sClient.Status().Update(ctx, snapshotCopy)
```

### 2. Error Handling

NEVER ignore errors from function calls:

```go
// WRONG: Ignored error
filenames, _ := cmd.Flags().GetStringSlice("filename")

// CORRECT: Handle error
filenames, err := cmd.Flags().GetStringSlice("filename")
if err != nil {
    return fmt.Errorf("failed to get filename flag: %w", err)
}
```

### 3. Context Propagation

ALWAYS pass context from caller, never use context.TODO():

```go
// WRONG
err := k8sClient.Get(context.TODO(), key, obj)

// CORRECT
err := k8sClient.Get(ctx, key, obj)
```

### 4. Resource Cleanup

Close files immediately after use, not with defer in loops:

```go
// WRONG: Deferred close in loop
for _, f := range files {
    file, _ := os.Open(f)
    defer file.Close()  // Leaks until function returns
}

// CORRECT: Close immediately or use helper
for _, f := range files {
    content, err := readFile(f)  // Helper closes file
}
```

### 5. Namespace Handling

ALWAYS specify namespace when accessing namespaced resources:

```go
// WRONG: Missing namespace
k8sClient.Get(ctx, client.ObjectKey{Name: name}, obj)

// CORRECT: Include namespace
k8sClient.Get(ctx, client.ObjectKey{
    Name:      name,
    Namespace: namespace,
}, obj)
```

## Known Issues (Must Fix)

| Priority | Issue | Location |
|----------|-------|----------|
| P0 | Before/After snapshots never populated | `cmd/kubectl-kronoform/main.go` |
| P0 | Controller reconcile loop is empty | `internal/controller/kronoform_controller.go` |
| P0 | No garbage collection for old records | Architecture gap |
| P1 | RBAC missing for History/Snapshot CRDs | `config/rbac/role.yaml` |
| P1 | Webhook validation is empty | `internal/webhook/v1alpha1/` |
| P1 | No file size limit on manifest reads | `cmd/kubectl-kronoform/main.go:203` |

## File Structure

```
kronoform/
├── api/v1alpha1/           # CRD type definitions
│   ├── kronoform_types.go
│   ├── kronoformhistory_types.go
│   └── kronoformsnapshot_types.go
├── cmd/
│   ├── main.go             # Operator entry point
│   └── kubectl-kronoform/  # kubectl plugin
│       ├── main.go
│       └── main_test.go
├── internal/
│   ├── controller/         # Reconciliation logic
│   └── webhook/v1alpha1/   # Admission webhooks
├── config/
│   ├── crd/bases/          # Generated CRD YAMLs
│   ├── rbac/               # RBAC configuration
│   ├── manager/            # Operator deployment
│   └── samples/            # Example manifests
└── test/
    ├── e2e/                # End-to-end tests
    └── utils/              # Test utilities
```

## Key Patterns

### CRD Phase Constants

Use constants instead of magic strings:

```go
const (
    PhasePending   = "Pending"
    PhaseCompleted = "Completed"
    PhaseNoChanges = "NoChanges"
    PhaseFailed    = "Failed"
)
```

### Kubernetes Client Creation

```go
func createK8sClient() (client.Client, error) {
    scheme := runtime.NewScheme()
    _ = historyv1alpha1.AddToScheme(scheme)

    config, err := ctrl.GetConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
    }

    return client.New(config, client.Options{Scheme: scheme})
}
```

### Input Validation

```go
func validatePath(filename string) (string, error) {
    cleanPath := filepath.Clean(filename)

    if strings.Contains(cleanPath, "..") {
        return "", fmt.Errorf("path traversal not allowed")
    }

    if filepath.IsAbs(cleanPath) {
        return "", fmt.Errorf("absolute paths not allowed")
    }

    // Resolve symlinks and verify within working directory
    cwd, _ := os.Getwd()
    absPath, _ := filepath.Abs(cleanPath)
    if !strings.HasPrefix(absPath, cwd) {
        return "", fmt.Errorf("path escapes working directory")
    }

    return cleanPath, nil
}
```

### Size-Limited File Reading

```go
const maxManifestSize = 10 * 1024 * 1024 // 10MB

func readManifestFile(path string) ([]byte, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    limitedReader := io.LimitReader(file, maxManifestSize+1)
    content, err := io.ReadAll(limitedReader)
    if err != nil {
        return nil, err
    }

    if int64(len(content)) > maxManifestSize {
        return nil, fmt.Errorf("file exceeds %d bytes", maxManifestSize)
    }

    return content, nil
}
```

## Testing Requirements

### Minimum Coverage: 80%

| Test Type | Location | Purpose |
|-----------|----------|---------|
| Unit | `*_test.go` alongside code | Functions and utilities |
| Controller | `internal/controller/suite_test.go` | Reconciliation logic |
| Webhook | `internal/webhook/v1alpha1/webhook_suite_test.go` | Admission logic |
| E2E | `test/e2e/` | Full operator deployment |

### Test Commands

```bash
make test              # Unit + integration tests
make test-e2e          # E2E tests with Kind cluster
make setup-test-e2e    # Create Kind cluster
make cleanup-test-e2e  # Delete Kind cluster
```

### Writing Tests

Use Ginkgo/Gomega BDD style:

```go
var _ = Describe("KronoformController", func() {
    Context("When reconciling a resource", func() {
        It("should create history record", func() {
            // Arrange
            snapshot := createTestSnapshot()

            // Act
            result, err := reconciler.Reconcile(ctx, req)

            // Assert
            Expect(err).NotTo(HaveOccurred())
            Expect(result.Requeue).To(BeFalse())
        })
    })
})
```

## Security Checklist

Before committing:

- [ ] No hardcoded secrets or API keys
- [ ] All file paths validated (no traversal)
- [ ] File size limits enforced
- [ ] User input sanitized before exec.Command
- [ ] RBAC follows least-privilege principle
- [ ] Webhooks validate CRD content
- [ ] Error messages don't leak sensitive info
- [ ] Development mode logging disabled for production

## Available Commands

```bash
# Build
make build              # Build operator
make build-plugin       # Build kubectl plugin
make docker-build       # Build container image

# Generate
make manifests          # Generate CRDs and RBAC
make generate           # Generate DeepCopy methods

# Deploy
make install            # Install CRDs to cluster
make deploy             # Deploy operator to cluster
make uninstall          # Remove CRDs

# Quality
make test               # Run tests
make lint               # Run golangci-lint
make fmt                # Format code
make vet                # Run go vet
```

## Git Workflow

### Commit Format

```
<type>: <description>

<optional body>
```

**Types:** `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`

### Examples

```
feat: add garbage collection for old snapshots
fix: handle namespace correctly in history lookup
refactor: extract file validation to separate function
test: add unit tests for path validation
```

## Development Workflow

1. **Plan**: Use `/plan` for complex features
2. **Test First**: Write failing test
3. **Implement**: Write minimal code to pass
4. **Refactor**: Clean up while tests pass
5. **Review**: Run `/code-review` before commit
6. **Security**: Run `/security-review` for sensitive changes

## API Group

- **Group:** `history.yu-kod.github.io`
- **Version:** `v1alpha1`
- **Resources:** `kronoforms`, `kronoformhistories`, `kronoformsnapshots`

## Environment Variables

```bash
# Required for development
KUBECONFIG=~/.kube/config

# Optional
ENABLE_WEBHOOKS=true
METRICS_BIND_ADDRESS=:8080
HEALTH_PROBE_BIND_ADDRESS=:8081
LEADER_ELECT=false
```

## Roadmap Reference

See [ROADMAP.md](ROADMAP.md) for planned features:
- v0.2: Enhanced diff, delete/patch support
- v0.3: Restore functionality, notifications
- v0.4: Terminal UI for history browsing
- v0.5: Helm support, drift detection
- v1.0: API stabilization, external storage
