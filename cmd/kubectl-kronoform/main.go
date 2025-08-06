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
	"strings"
	"time"

	"github.com/spf13/cobra"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(cmd, args)
		},
	}

	// Add flags similar to kubectl apply
	applyCmd.Flags().StringSliceP("filename", "f", []string{}, "Filename, directory, or URL to files to use to create the resource")
	applyCmd.Flags().Bool("dry-run", false, "If true, only print the object that would be sent, without sending it")
	applyCmd.Flags().StringP("namespace", "n", "", "If present, the namespace scope for this CLI request")

	rootCmd.AddCommand(applyCmd)

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

	kubectlCmd := exec.Command("kubectl", kubectlArgs...)

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
	hasChanges := analyzeKubectlOutput(stdout.String())

	// Create history record after successful apply only if there were changes
	if !dryRun && k8sClient != nil && snapshotName != "" && hasChanges {
		err = createHistory(k8sClient, manifestContent, snapshotName, namespace)
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
		file, err := os.Open(filename)
		if err != nil {
			return "", fmt.Errorf("failed to open file %s: %w", filename, err)
		}
		defer file.Close()

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
func createHistory(k8sClient client.Client, manifestContent string, snapshotName string, namespace string) error {
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
			Manifests:   manifestContent,
			SnapshotRef: snapshotName,
			Description: fmt.Sprintf("Applied by %s", appliedBy),
			AppliedBy:   appliedBy,
		},
		Status: historyv1alpha1.KronoformHistoryStatus{
			AppliedAt: &now,
			Summary:   "Successfully applied manifests",
		},
	}

	if err := k8sClient.Create(ctx, history); err != nil {
		return err
	}

	// Update snapshot status to reference the history
	snapshot := &historyv1alpha1.KronoformSnapshot{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      snapshotName,
		Namespace: getTargetNamespace(namespace),
	}, snapshot); err != nil {
		return err
	}

	snapshot.Status.Phase = "Completed"
	snapshot.Status.AppliedAt = &now
	snapshot.Status.HistoryRef = historyName
	snapshot.Status.Message = "Successfully applied and recorded"

	return k8sClient.Status().Update(ctx, snapshot)
}

// getTargetNamespace returns the appropriate namespace to use
func getTargetNamespace(namespace string) string {
	if namespace != "" {
		return namespace
	}
	return "default"
}

// analyzeKubectlOutput analyzes kubectl apply output to determine if changes were made
func analyzeKubectlOutput(output string) bool {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Check for change indicators
		if strings.Contains(line, "created") ||
			strings.Contains(line, "configured") ||
			strings.Contains(line, "deleted") {
			return true
		}

		// Skip "unchanged" lines - these indicate no changes
		if strings.Contains(line, "unchanged") {
			continue
		}
	}

	return false
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
