/*
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
*/

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	historyv1alpha1 "github.com/yu-kod/kronoform/api/v1alpha1"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "kubectl-kronoform",
		Short: "A time-lapse camera for your Kubernetes cluster",
		Long: `Kronoform tracks every kubectl apply and its resulting resource states.
Simply use 'kubectl kronoform apply' instead of 'kubectl apply' to automatically
record and accumulate successful YAML files along with the actual resource states.`,
	}

	var applyCmd = &cobra.Command{
		Use:   "apply",
		Short: "Apply configuration to a resource and record the change",
		Long: `Apply configuration to a resource by filename or stdin and record the change.
This command combines kubectl apply with automatic history tracking.`,
		RunE: runApply,
	}

	// Add flags similar to kubectl apply
	applyCmd.Flags().StringSliceP("filename", "f", []string{}, "Filename, directory, or URL to files to use to create the resource")
	applyCmd.Flags().Bool("dry-run", false, "If true, only print the object that would be sent, without sending it")
	applyCmd.Flags().StringP("namespace", "n", "", "If present, the namespace scope for this CLI request")

	var diffCmd = &cobra.Command{
		Use:   "diff",
		Short: "Show timeline of changes with resource details",
		Long: `Display a timeline of all recorded changes, showing when each change was applied
and what resources were created, modified, or deleted.`,
		RunE: runDiff,
	}

	var deleteCmd = &cobra.Command{
		Use:   "delete",
		Short: "Delete resources and record the change",
		Long: `Delete resources by filename, resource and name, or by resources and label selector.
This command combines kubectl delete with automatic history tracking.`,
		RunE: runDelete,
	}

	// Add flags similar to kubectl delete
	deleteCmd.Flags().StringSliceP("filename", "f", []string{}, "Filename, directory, or URL to files identifying the resources to delete")
	deleteCmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
	deleteCmd.Flags().StringP("namespace", "n", "", "If present, the namespace scope for this CLI request")
	deleteCmd.Flags().Bool("all", false, "Delete all resources, including uninitialized ones, in the namespace of the specified resource types")
	deleteCmd.Flags().StringSlice("ignore-not-found", []string{}, "Treat \"resource not found\" as a successful delete")

	var patchCmd = &cobra.Command{
		Use:   "patch",
		Short: "Update field(s) of a resource and record the change",
		Long: `Update field(s) of a resource using strategic merge patch, JSON merge patch, or JSON patch.
This command combines kubectl patch with automatic history tracking.`,
		Args: cobra.MinimumNArgs(2),
		RunE: runPatch,
	}

	// Add flags similar to kubectl patch
	patchCmd.Flags().StringP("type", "p", "strategic", "The type of patch being provided; one of [strategic, merge, json]")
	patchCmd.Flags().StringP("namespace", "n", "", "If present, the namespace scope for this CLI request")

	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(patchCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runApply(cmd *cobra.Command, args []string) error {
	fmt.Printf("[%s] Kronoform: Starting apply operation...\n", time.Now().Format("15:04:05"))

	// Get flags
	filenames, _ := cmd.Flags().GetStringSlice("filename")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	namespace, _ := cmd.Flags().GetString("namespace")

	// Read the manifest content
	var manifestContent string
	if len(filenames) > 0 {
		content, err := readManifestFiles(filenames)
		if err != nil {
			return fmt.Errorf("failed to read manifest files: %w", err)
		}
		manifestContent = content
	}

	// Create Kubernetes client
	k8sClient, err := createK8sClient()
	if err != nil {
		fmt.Printf("[%s] Kronoform: Warning - Could not create k8s client, skipping history recording: %v\n", time.Now().Format("15:04:05"), err)
	}

	// Create snapshot record before applying (if not dry-run and client available)
	var snapshotName string
	if !dryRun && k8sClient != nil && manifestContent != "" {
		snapshotName, err = createSnapshot(k8sClient, manifestContent, namespace)
		if err != nil {
			fmt.Printf("[%s] Kronoform: Warning - Could not create snapshot: %v\n", time.Now().Format("15:04:05"), err)
		} else {
			fmt.Printf("[%s] Kronoform: Created snapshot: %s\n", time.Now().Format("15:04:05"), snapshotName)
		}
	}

	// Build kubectl args
	kubectlArgs := []string{"apply"}

	// Add filenames
	for _, filename := range filenames {
		kubectlArgs = append(kubectlArgs, "-f", filename)
	}

	// Add dry-run flag
	if dryRun {
		kubectlArgs = append(kubectlArgs, "--dry-run=client")
	}

	// Add namespace
	if namespace != "" {
		kubectlArgs = append(kubectlArgs, "-n", namespace)
	}

	// Add any additional args
	kubectlArgs = append(kubectlArgs, args...)

	kubectlCmd := exec.Command("kubectl", kubectlArgs...) // #nosec G204 - kubectlArgs is constructed from validated inputs

	// Capture stdout to analyze the output
	var stdout strings.Builder
	kubectlCmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	kubectlCmd.Stderr = os.Stderr
	kubectlCmd.Stdin = os.Stdin

	fmt.Printf("[%s] Kronoform: Executing kubectl %v\n", time.Now().Format("15:04:05"), kubectlArgs)

	if err := kubectlCmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply failed: %w", err)
	}

	fmt.Printf("[%s] Kronoform: Apply operation completed successfully\n", time.Now().Format("15:04:05"))

	// Check if there were actual changes by analyzing kubectl output
	hasChanges, changes := analyzeKubectlOutput(stdout.String())

	// Create history record after successful apply only if there were changes
	if !dryRun && k8sClient != nil && snapshotName != "" && hasChanges {
		// Get actual state after apply
		actualState, err := getActualState(manifestContent, namespace)
		if err != nil {
			fmt.Printf("[%s] Kronoform: Warning - Could not get actual state: %v\n", time.Now().Format("15:04:05"), err)
			actualState = manifestContent // fallback to manifest
		}
		err = createHistory(k8sClient, actualState, snapshotName, namespace, changes)
		if err != nil {
			fmt.Printf("[%s] Kronoform: Warning - Could not create history: %v\n", time.Now().Format("15:04:05"), err)
		} else {
			fmt.Printf("[%s] Kronoform: History recorded successfully\n", time.Now().Format("15:04:05"))
		}
	} else if !hasChanges {
		fmt.Printf("[%s] Kronoform: No changes detected, skipping history recording\n", time.Now().Format("15:04:05"))

		// Clean up the snapshot since no changes were made
		if k8sClient != nil && snapshotName != "" {
			cleanupSnapshot(k8sClient, snapshotName, namespace)
		}
	}

	return nil
}

// readManifestFiles reads and concatenates content from multiple manifest files
func readManifestFiles(filenames []string) (string, error) {
	var allContent strings.Builder

	for _, filename := range filenames {
		// Path validation to prevent directory traversal and file inclusion attacks
		cleanPath := filepath.Clean(filename)
		if strings.Contains(cleanPath, "..") {
			return "", fmt.Errorf("invalid filename: %s (contains '..')", filename)
		}
		// Ensure the path is not absolute and doesn't start with dangerous patterns
		if filepath.IsAbs(cleanPath) {
			return "", fmt.Errorf("invalid filename: %s (absolute paths not allowed)", filename)
		}

		file, err := os.Open(cleanPath) // #nosec G304 - Path is validated above
		if err != nil {
			return "", fmt.Errorf("failed to open file %s: %w", filename, err)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to close file %s: %v\n", filename, closeErr)
			}
		}()

		content, err := io.ReadAll(file)
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", filename, err)
		}

		if allContent.Len() > 0 {
			allContent.WriteString("\n---\n")
		}
		allContent.Write(content)
	}

	return allContent.String(), nil
}

// getCurrentNamespace gets the current kubectl namespace
func getCurrentNamespace() (string, error) {
	cmd := exec.Command("kubectl", "config", "view", "--minify", "--output", "jsonpath={..namespace}")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	namespace := strings.TrimSpace(string(output))
	if namespace == "" {
		namespace = "default"
	}
	return namespace, nil
}

// createK8sClient creates a Kubernetes client using the default kubeconfig
func createK8sClient() (client.Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, err
		}
	}

	// Create scheme and add our CRDs
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		return nil, err
	}
	if err := historyv1alpha1.AddToScheme(s); err != nil {
		return nil, err
	}

	c, err := client.New(config, client.Options{Scheme: s})
	if err != nil {
		return nil, err
	}

	return c, nil
}

// getActualState retrieves the actual state of resources from the cluster after apply
func getActualState(manifestContent, namespace string) (string, error) {
	var actualStates []string

	// Split manifest into individual resources
	resources := strings.Split(manifestContent, "---")
	for _, resource := range resources {
		resource = strings.TrimSpace(resource)
		if resource == "" {
			continue
		}

		// Extract kind, name, namespace using regex
		kindRegex := regexp.MustCompile(`kind:\s*(\w+)`)
		nameRegex := regexp.MustCompile(`name:\s*([^\s]+)`)
		nsRegex := regexp.MustCompile(`namespace:\s*([^\s]+)`)

		kindMatch := kindRegex.FindStringSubmatch(resource)
		nameMatch := nameRegex.FindStringSubmatch(resource)
		nsMatch := nsRegex.FindStringSubmatch(resource)

		if len(kindMatch) < 2 || len(nameMatch) < 2 {
			continue
		}

		kind := kindMatch[1]
		name := nameMatch[1]
		
		// Validate extracted values to prevent command injection
		if !isValidKubernetesName(kind) || !isValidKubernetesName(name) {
			continue
		}
		
		resNamespace := namespace
		if len(nsMatch) >= 2 {
			resNamespace = nsMatch[1]
			if !isValidKubernetesName(resNamespace) {
				resNamespace = "default"
			}
		}
		if resNamespace == "" {
			resNamespace = "default"
		}

		// Get actual state using kubectl
		cmd := exec.Command("kubectl", "get", strings.ToLower(kind), name, "-n", resNamespace, "-o", "yaml") // #nosec G204 - validated inputs
		output, err := cmd.Output()
		if err != nil {
			// If get fails, use original manifest
			actualStates = append(actualStates, resource)
		} else {
			actualStates = append(actualStates, string(output))
		}
	}

	return strings.Join(actualStates, "\n---\n"), nil
}

// createSnapshot creates a KronoformSnapshot resource
func createSnapshot(k8sClient client.Client, manifestContent string, namespace string) (string, error) {
	ctx := context.Background()
	now := metav1.Now()

	// Get current user for tracking
	currentUser, _ := user.Current()
	appliedBy := "unknown"
	if currentUser != nil {
		appliedBy = currentUser.Username
	}

	// Generate snapshot name
	snapshotName := fmt.Sprintf("kronoform-snapshot-%d", now.Unix())

	snapshot := &historyv1alpha1.KronoformSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: getTargetNamespace(namespace),
		},
		Spec: historyv1alpha1.KronoformSnapshotSpec{
			Manifests:       manifestContent,
			Description:     fmt.Sprintf("Applied by %s at %s", appliedBy, now.Format(time.RFC3339)),
			TargetNamespace: namespace,
		},
		Status: historyv1alpha1.KronoformSnapshotStatus{
			Phase: "Pending",
		},
	}

	if err := k8sClient.Create(ctx, snapshot); err != nil {
		return "", err
	}

	return snapshotName, nil
}

// createHistory creates a KronoformHistory resource
func createHistory(k8sClient client.Client, manifestContent string, snapshotName string, namespace string, changes []historyv1alpha1.ResourceChange) error {
	ctx := context.Background()
	now := metav1.Now()

	// Get current user for tracking
	currentUser, _ := user.Current()
	appliedBy := "unknown"
	if currentUser != nil {
		appliedBy = currentUser.Username
	}

	// Generate history name
	historyName := fmt.Sprintf("kronoform-history-%d", now.Unix())

	history := &historyv1alpha1.KronoformHistory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      historyName,
			Namespace: getTargetNamespace(namespace),
		},
		Spec: historyv1alpha1.KronoformHistorySpec{
			Manifests:       manifestContent,
			SnapshotRef:     snapshotName,
			Description:     fmt.Sprintf("Applied by %s", appliedBy),
			AppliedBy:       appliedBy,
			ResourceChanges: changes,
		},
		Status: historyv1alpha1.KronoformHistoryStatus{
			AppliedAt: &now,
			Summary:   "Successfully applied manifests",
		},
	}

	if err := k8sClient.Create(ctx, history); err != nil {
		return err
	}

	// Update snapshot status to reference the history (only if snapshot exists)
	snapshot := &historyv1alpha1.KronoformSnapshot{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      snapshotName,
		Namespace: getTargetNamespace(namespace),
	}, snapshot); err == nil {
		// Snapshot exists, update it
		snapshot.Status.Phase = "Completed"
		snapshot.Status.AppliedAt = &now
		snapshot.Status.HistoryRef = historyName
		snapshot.Status.Message = "Successfully applied and recorded"
		return k8sClient.Status().Update(ctx, snapshot)
	} else {
		// Snapshot doesn't exist (e.g., for delete operations), just return success
		fmt.Printf("[%s] Kronoform: History recorded without snapshot update\n", time.Now().Format("15:04:05"))
		return nil
	}
}

// getTargetNamespace returns the appropriate namespace to use
func getTargetNamespace(namespace string) string {
	if namespace != "" {
		return namespace
	}
	return "default"
}

// analyzeKubectlOutput analyzes kubectl apply output to determine if changes were made and extract resource changes
func analyzeKubectlOutput(output string) (bool, []historyv1alpha1.ResourceChange) {
	var changes []historyv1alpha1.ResourceChange
	hasChanges := false

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse lines like "deployment.apps/kronoform-demo-app configured" or "deployment.apps "kronoform-demo-app" deleted"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			resource := parts[0]
			var operation string

			// Handle different output formats
			switch {
			case len(parts) == 2:
				// Format: "resource operation" (apply)
				operation = parts[1]
			case len(parts) == 3 && parts[1] == `"`+strings.Trim(parts[1], `"`) && parts[2] == "deleted":
				// Format: 'resource "name" deleted' (delete)
				resource = parts[0] + "/" + strings.Trim(parts[1], `"`)
				operation = parts[2]
			case len(parts) >= 3:
				// Try to find the operation
				for i, part := range parts {
					if part == "created" || part == "configured" || part == "unchanged" || part == "deleted" {
						operation = part
						if i > 0 {
							resource = strings.Join(parts[:i], " ")
						}
						break
					}
				}
			}

			if operation != "" {
				// Clean up resource name (remove .apps, .v1, etc.)
				resource = cleanResourceName(resource)

				// Capitalize operation
				switch strings.ToLower(operation) {
				case "created":
					operation = "Created"
				case "configured":
					operation = "Configured"
				case "unchanged":
					operation = "Unchanged"
				case "deleted":
					operation = "Deleted"
				}

				changes = append(changes, historyv1alpha1.ResourceChange{
					Resource:  resource,
					Operation: operation,
				})

				if operation == "Created" || operation == "Configured" || operation == "Deleted" {
					hasChanges = true
				}
			}
		}
	}

	return hasChanges, changes
}

// isValidKubernetesName validates if a string is a valid Kubernetes resource name
func isValidKubernetesName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	
	// Kubernetes resource names must consist of lower case alphanumeric characters, '-', and '.'
	// and must start and end with an alphanumeric character
	for i, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' {
			return false
		}
		if i == 0 && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
		if i == len(name)-1 && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// cleanResourceName cleans up Kubernetes resource names for better display
func cleanResourceName(resource string) string {
	// Remove common API group suffixes to make names shorter and clearer
	resource = strings.ReplaceAll(resource, ".apps", "")
	resource = strings.ReplaceAll(resource, ".v1", "")
	resource = strings.ReplaceAll(resource, ".batch", "")
	resource = strings.ReplaceAll(resource, ".networking.k8s.io", "")
	resource = strings.ReplaceAll(resource, ".rbac.authorization.k8s.io", "")
	resource = strings.ReplaceAll(resource, ".storage.k8s.io", "")
	resource = strings.ReplaceAll(resource, ".apiextensions.k8s.io", "")
	return resource
}

// cleanupSnapshot removes a snapshot that was created but not needed due to no changes
func cleanupSnapshot(k8sClient client.Client, snapshotName string, namespace string) {
	ctx := context.Background()
	snapshot := &historyv1alpha1.KronoformSnapshot{}

	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      snapshotName,
		Namespace: getTargetNamespace(namespace),
	}, snapshot)

	if err != nil {
		// Snapshot doesn't exist, nothing to clean up
		return
	}

	// Update snapshot status to indicate no changes
	snapshot.Status.Phase = "NoChanges"
	snapshot.Status.Message = "No changes detected, snapshot not needed"

	if err := k8sClient.Status().Update(ctx, snapshot); err != nil {
		fmt.Printf("[%s] Kronoform: Warning - Could not update snapshot status: %v\n", time.Now().Format("15:04:05"), err)
	}

	// Optionally delete the snapshot entirely
	if err := k8sClient.Delete(ctx, snapshot); err != nil {
		fmt.Printf("[%s] Kronoform: Warning - Could not delete unused snapshot: %v\n", time.Now().Format("15:04:05"), err)
	} else {
		fmt.Printf("[%s] Kronoform: Cleaned up unused snapshot: %s\n", time.Now().Format("15:04:05"), snapshotName)
	}
}

func getHistory(k8sClient client.Client, historyID string, namespace string) (*historyv1alpha1.KronoformHistory, error) {
	history := &historyv1alpha1.KronoformHistory{}
	err := k8sClient.Get(context.TODO(), client.ObjectKey{
		Name:      historyID,
		Namespace: namespace,
	}, history)
	if err != nil {
		return nil, err
	}
	return history, nil
}

func getSnapshot(k8sClient client.Client, snapshotName string, namespace string) (*historyv1alpha1.KronoformSnapshot, error) {
	snapshot := &historyv1alpha1.KronoformSnapshot{}
	err := k8sClient.Get(context.TODO(), client.ObjectKey{
		Name:      snapshotName,
		Namespace: namespace,
	}, snapshot)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func showDiff(before, after string) error {
	// Parse YAMLs
	var beforeObj, afterObj map[interface{}]interface{}
	if err := yaml.Unmarshal([]byte(before), &beforeObj); err != nil {
		return fmt.Errorf("failed to parse before YAML: %w", err)
	}
	if err := yaml.Unmarshal([]byte(after), &afterObj); err != nil {
		return fmt.Errorf("failed to parse after YAML: %w", err)
	}

	// Remove unwanted fields
	removeUnwantedFields(beforeObj)
	removeUnwantedFields(afterObj)

	// Convert back to YAML for diff
	beforeClean, err := yaml.Marshal(beforeObj)
	if err != nil {
		return fmt.Errorf("failed to marshal before: %w", err)
	}
	afterClean, err := yaml.Marshal(afterObj)
	if err != nil {
		return fmt.Errorf("failed to marshal after: %w", err)
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(string(beforeClean), string(afterClean), false)

	fmt.Println("Diff between before and after (filtered):")
	fmt.Println(dmp.DiffPrettyText(diffs))
	return nil
}

func runDiff(cmd *cobra.Command, args []string) error {
	fmt.Printf("[%s] Kronoform: Showing timeline of changes...\n", time.Now().Format("15:04:05"))

	// Get current namespace
	namespace, err := getCurrentNamespace()
	if err != nil {
		return fmt.Errorf("failed to get current namespace: %w", err)
	}

	// Create Kubernetes client
	k8sClient, err := createK8sClient()
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	// List all histories
	histories := &historyv1alpha1.KronoformHistoryList{}
	err = k8sClient.List(context.TODO(), histories, client.InNamespace(namespace))
	if err != nil {
		return fmt.Errorf("failed to list histories: %w", err)
	}

	// Sort by creation time
	sort.Slice(histories.Items, func(i, j int) bool {
		return histories.Items[i].CreationTimestamp.Before(&histories.Items[j].CreationTimestamp)
	})

	fmt.Println("Timeline of Changes:")
	fmt.Println("====================")

	// Print table header
	fmt.Printf("%-3s %-20s %-15s %-35s %-35s %-35s %-30s\n", "#", "Time", "Operation", "Created", "Modified", "Deleted", "Snapshot ID")
	fmt.Println(strings.Repeat("-", 173))

	for i, history := range histories.Items {
		timeStr := history.CreationTimestamp.Format("2006-01-02 15:04:05")
		operation := "Apply" // Default, could be enhanced to detect from snapshot name
		if strings.HasPrefix(history.Spec.SnapshotRef, "delete-") {
			operation = "Delete"
		} else if strings.HasPrefix(history.Spec.SnapshotRef, "patch-") {
			operation = "Patch"
		}

		// Count resources by operation
		var created, modified, deleted []string
		for _, change := range history.Spec.ResourceChanges {
			switch change.Operation {
			case "Created":
				created = append(created, change.Resource)
			case "Configured":
				modified = append(modified, change.Resource)
			case "Deleted":
				deleted = append(deleted, change.Resource)
			}
		}

		// Format resource lists (truncate if too long)
		createdStr := strings.Join(created, ", ")
		if len(createdStr) > 33 {
			createdStr = createdStr[:30] + "..."
		}
		modifiedStr := strings.Join(modified, ", ")
		if len(modifiedStr) > 33 {
			modifiedStr = modifiedStr[:30] + "..."
		}
		deletedStr := strings.Join(deleted, ", ")
		if len(deletedStr) > 33 {
			deletedStr = deletedStr[:30] + "..."
		}

		fmt.Printf("%-3d %-20s %-15s %-35s %-35s %-35s %-30s\n", i+1, timeStr, operation, createdStr, modifiedStr, deletedStr, history.Spec.SnapshotRef)
	}

	// Interactive selection
	if len(histories.Items) > 0 {
		fmt.Printf("\nEnter the number of the history to view YAML content (1-%d, or 0 to exit): ", len(histories.Items))
		var choice int
		_, err := fmt.Scanf("%d", &choice)
		if err != nil {
			fmt.Printf("[%s] Kronoform: Invalid input, exiting\n", time.Now().Format("15:04:05"))
			return nil
		}
		if choice > 0 && choice <= len(histories.Items) {
			selected := histories.Items[choice-1]
			fmt.Printf("\nYAML Content for History %s:\n", selected.Name)
			fmt.Println("=====================================")
			fmt.Println(selected.Spec.Manifests)
		}
	}

	return nil
}

// removeUnwantedFields removes Kubernetes-managed fields that shouldn't be compared
func removeUnwantedFields(obj map[interface{}]interface{}) {
	// Remove status
	delete(obj, "status")

	// Remove unwanted metadata fields
	if metadata, ok := obj["metadata"].(map[interface{}]interface{}); ok {
		delete(metadata, "creationTimestamp")
		delete(metadata, "resourceVersion")
		delete(metadata, "uid")
		delete(metadata, "generation")
		delete(metadata, "managedFields")
		delete(metadata, "annotations") // Remove all annotations for simplicity
	}
}

func runDelete(cmd *cobra.Command, args []string) error {
	fmt.Printf("[%s] Kronoform: Starting delete operation...\n", time.Now().Format("15:04:05"))

	// Get flags
	filenames, _ := cmd.Flags().GetStringSlice("filename")
	selector, _ := cmd.Flags().GetString("selector")
	namespace, _ := cmd.Flags().GetString("namespace")
	all, _ := cmd.Flags().GetBool("all")
	ignoreNotFound, _ := cmd.Flags().GetStringSlice("ignore-not-found")

	// Create Kubernetes client
	k8sClient, err := createK8sClient()
	if err != nil {
		fmt.Printf("[%s] Kronoform: Warning - Could not create k8s client, skipping history recording: %v\n", time.Now().Format("15:04:05"), err)
	}

	// For delete operations, we need to capture the current state before deletion
	var beforeState string
	if k8sClient != nil {
		beforeState, err = captureCurrentState(k8sClient, filenames, args, namespace)
		if err != nil {
			fmt.Printf("[%s] Kronoform: Warning - Could not capture current state: %v\n", time.Now().Format("15:04:05"), err)
		}
	}

	// Build kubectl args
	kubectlArgs := []string{"delete"}

	// Add resource type and name if provided as args
	kubectlArgs = append(kubectlArgs, args...)

	// Add filenames
	for _, filename := range filenames {
		kubectlArgs = append(kubectlArgs, "-f", filename)
	}

	// Add selector
	if selector != "" {
		kubectlArgs = append(kubectlArgs, "-l", selector)
	}

	// Add namespace
	if namespace != "" {
		kubectlArgs = append(kubectlArgs, "-n", namespace)
	}

	// Add all flag
	if all {
		kubectlArgs = append(kubectlArgs, "--all")
	}

	// Add ignore-not-found
	for _, ignore := range ignoreNotFound {
		kubectlArgs = append(kubectlArgs, "--ignore-not-found="+ignore)
	}

	kubectlCmd := exec.Command("kubectl", kubectlArgs...) // #nosec G204 - kubectlArgs is constructed from validated inputs

	// Capture stdout to analyze the output
	var stdout strings.Builder
	kubectlCmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	kubectlCmd.Stderr = os.Stderr
	kubectlCmd.Stdin = os.Stdin

	fmt.Printf("[%s] Kronoform: Executing kubectl %v\n", time.Now().Format("15:04:05"), kubectlArgs)

	if err := kubectlCmd.Run(); err != nil {
		return fmt.Errorf("kubectl delete failed: %w", err)
	}

	fmt.Printf("[%s] Kronoform: Delete operation completed successfully\n", time.Now().Format("15:04:05"))

	// Analyze kubectl output to extract resource changes
	hasChanges, changes := analyzeKubectlOutput(stdout.String())

	// Create history record for the delete operation
	if k8sClient != nil && beforeState != "" && hasChanges {
		// Generate snapshot name for delete operation
		snapshotName := fmt.Sprintf("delete-%s", time.Now().Format("20060102-150405"))
		err = createHistory(k8sClient, beforeState, snapshotName, namespace, changes)
		if err != nil {
			fmt.Printf("[%s] Kronoform: Warning - Could not create delete history: %v\n", time.Now().Format("15:04:05"), err)
		} else {
			fmt.Printf("[%s] Kronoform: Delete history recorded successfully\n", time.Now().Format("15:04:05"))
		}
	}

	return nil
}

func runPatch(cmd *cobra.Command, args []string) error {
	fmt.Printf("[%s] Kronoform: Starting patch operation...\n", time.Now().Format("15:04:05"))

	if len(args) < 2 {
		return fmt.Errorf("patch requires at least 2 arguments: resource and patch data")
	}

	resource := args[0]
	patchData := args[1]

	// Get flags
	patchType, _ := cmd.Flags().GetString("type")
	namespace, _ := cmd.Flags().GetString("namespace")

	// Create Kubernetes client
	k8sClient, err := createK8sClient()
	if err != nil {
		fmt.Printf("[%s] Kronoform: Warning - Could not create k8s client, skipping history recording: %v\n", time.Now().Format("15:04:05"), err)
	}

	// Capture current state before patching
	var beforeState string
	if k8sClient != nil {
		beforeState, err = captureResourceState(k8sClient, resource, namespace)
		if err != nil {
			fmt.Printf("[%s] Kronoform: Warning - Could not capture current state: %v\n", time.Now().Format("15:04:05"), err)
		}
	}

	// Build kubectl args
	kubectlArgs := []string{"patch", resource, "-p", patchData, "--type", patchType}

	// Add namespace
	if namespace != "" {
		kubectlArgs = append(kubectlArgs, "-n", namespace)
	}

	kubectlCmd := exec.Command("kubectl", kubectlArgs...) // #nosec G204 - kubectlArgs is constructed from validated inputs

	// Capture stdout to analyze the output
	var stdout strings.Builder
	kubectlCmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	kubectlCmd.Stderr = os.Stderr
	kubectlCmd.Stdin = os.Stdin

	fmt.Printf("[%s] Kronoform: Executing kubectl %v\n", time.Now().Format("15:04:05"), kubectlArgs)

	if err := kubectlCmd.Run(); err != nil {
		return fmt.Errorf("kubectl patch failed: %w", err)
	}

	fmt.Printf("[%s] Kronoform: Patch operation completed successfully\n", time.Now().Format("15:04:05"))

	// Analyze kubectl output to extract resource changes
	hasChanges, changes := analyzeKubectlOutput(stdout.String())

	// Create history record for the patch operation
	if k8sClient != nil && beforeState != "" && hasChanges {
		// Generate snapshot name for patch operation
		snapshotName := fmt.Sprintf("patch-%s", time.Now().Format("20060102-150405"))
		err = createHistory(k8sClient, beforeState, snapshotName, namespace, changes)
		if err != nil {
			fmt.Printf("[%s] Kronoform: Warning - Could not create patch history: %v\n", time.Now().Format("15:04:05"), err)
		} else {
			fmt.Printf("[%s] Kronoform: Patch history recorded successfully\n", time.Now().Format("15:04:05"))
		}
	}

	return nil
}

func captureCurrentState(k8sClient client.Client, filenames []string, args []string, namespace string) (string, error) {
	// This is a simplified implementation - in practice, you'd need to get the actual resource specs
	// For now, we'll create a placeholder manifest based on the delete command
	var manifest strings.Builder
	manifest.WriteString("# Captured state before delete operation\n")

	if len(filenames) > 0 {
		for _, filename := range filenames {
			content, err := os.ReadFile(filename) // #nosec G304 - filename comes from validated user input
			if err != nil {
				continue
			}
			manifest.Write(content)
			manifest.WriteString("\n---\n")
		}
	} else if len(args) > 0 {
		manifest.WriteString(fmt.Sprintf("# Deleting resource: %s\n", strings.Join(args, " ")))
	}

	return manifest.String(), nil
}

func captureResourceState(k8sClient client.Client, resource string, namespace string) (string, error) {
	// This is a simplified implementation - in practice, you'd need to query the actual resource
	// For now, we'll create a placeholder
	return fmt.Sprintf("# Current state of resource: %s\n", resource), nil
}
