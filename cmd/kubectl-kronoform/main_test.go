package main

import (
	"context"
	"os"
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	historyv1alpha1 "github.com/yu-kod/kronoform/api/v1alpha1"
)

func TestMain(m *testing.M) {
	// Setup for all tests
	os.Exit(m.Run())
}

func TestRunDiff(t *testing.T) {
	t.Run("successful diff retrieval", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)

		// Create fake client
		scheme := runtime.NewScheme()
		if err := historyv1alpha1.AddToScheme(scheme); err != nil {
			t.Fatalf("Failed to add scheme: %v", err)
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Create test data
		history := &historyv1alpha1.KronoformHistory{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-history",
			},
			Spec: historyv1alpha1.KronoformHistorySpec{
				Manifests:   "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default\ndata:\n  key: new-value",
				SnapshotRef: "test-snapshot",
			},
		}

		snapshot := &historyv1alpha1.KronoformSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-snapshot",
			},
			Spec: historyv1alpha1.KronoformSnapshotSpec{
				Manifests: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default\ndata:\n  key: old-value",
			},
		}

		// Create objects in fake client
		err := fakeClient.Create(context.TODO(), history)
		g.Expect(err).To(gomega.BeNil())

		err = fakeClient.Create(context.TODO(), snapshot)
		g.Expect(err).To(gomega.BeNil())

		// Test getHistory
		retrievedHistory, err := getHistory(fakeClient, "test-history")
		g.Expect(err).To(gomega.BeNil())
		g.Expect(retrievedHistory.Spec.Manifests).To(gomega.Equal(history.Spec.Manifests))

		// Test getSnapshot
		retrievedSnapshot, err := getSnapshot(fakeClient, "test-snapshot")
		g.Expect(err).To(gomega.BeNil())
		g.Expect(retrievedSnapshot.Spec.Manifests).To(gomega.Equal(snapshot.Spec.Manifests))
	})

	t.Run("non-existent history", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)

		scheme := runtime.NewScheme()
		if err := historyv1alpha1.AddToScheme(scheme); err != nil {
			t.Fatalf("Failed to add scheme: %v", err)
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Test non-existent history
		_, err := getHistory(fakeClient, "non-existent")
		g.Expect(err).To(gomega.HaveOccurred())
	})
}

func TestShowDiff(t *testing.T) {
	t.Run("basic diff display", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)

		before := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: old-value"
		after := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: new-value"

		err := showDiff(before, after)
		g.Expect(err).To(gomega.BeNil())
	})

	t.Run("identical content", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)

		content := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value"

		err := showDiff(content, content)
		g.Expect(err).To(gomega.BeNil())
	})
}

func TestReadManifestFiles(t *testing.T) {
	t.Run("single file", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)

		// Create a temporary file in current directory to avoid absolute path issues
		content := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value"
		tmpFile, err := os.CreateTemp(".", "test-manifest-*.yaml")
		g.Expect(err).To(gomega.BeNil())
		defer func() {
			if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
				t.Logf("Warning: failed to remove temp file: %v", removeErr)
			}
		}()

		_, err = tmpFile.WriteString(content)
		g.Expect(err).To(gomega.BeNil())
		if closeErr := tmpFile.Close(); closeErr != nil {
			t.Logf("Warning: failed to close temp file: %v", closeErr)
		}

		// Test readManifestFiles with relative path
		result, err := readManifestFiles([]string{tmpFile.Name()})
		g.Expect(err).To(gomega.BeNil())
		g.Expect(result).To(gomega.Equal(content))
	})

	t.Run("non-existent file", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)

		_, err := readManifestFiles([]string{"non-existent-file.yaml"})
		g.Expect(err).To(gomega.HaveOccurred())
	})
}
