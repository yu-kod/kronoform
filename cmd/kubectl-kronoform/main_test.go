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

func TestRunDiff(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Create fake client
	scheme := runtime.NewScheme()
	historyv1alpha1.AddToScheme(scheme)

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

	// Test showDiff (this will print to stdout, but we can test it doesn't error)
	err = showDiff(snapshot.Spec.Manifests, history.Spec.Manifests)
	g.Expect(err).To(gomega.BeNil())
}

func TestShowDiff(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	before := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: old-value"
	after := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: new-value"

	err := showDiff(before, after)
	g.Expect(err).To(gomega.BeNil())
}

func TestReadManifestFiles(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Create a temporary file
	content := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value"
	tmpFile, err := os.CreateTemp("", "test-manifest-*.yaml")
	g.Expect(err).To(gomega.BeNil())
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	g.Expect(err).To(gomega.BeNil())
	tmpFile.Close()

	// Test readManifestFiles
	result, err := readManifestFiles([]string{tmpFile.Name()})
	g.Expect(err).To(gomega.BeNil())
	g.Expect(result).To(gomega.Equal(content))
}
