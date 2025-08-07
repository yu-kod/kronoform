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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceSnapshot represents the state of a single Kubernetes resource
type ResourceSnapshot struct {
	// APIVersion of the resource
	// +required
	APIVersion string `json:"apiVersion"`

	// Kind of the resource
	// +required
	Kind string `json:"kind"`

	// Name of the resource
	// +required
	Name string `json:"name"`

	// Namespace of the resource (empty for cluster-scoped resources)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Operation indicates what operation was performed (Created, Updated, Deleted, Unchanged)
	// +optional
	Operation string `json:"operation,omitempty"`

	// Before contains the resource state before applying manifests
	// +optional
	Before string `json:"before,omitempty"`

	// After contains the resource state after applying manifests
	// +optional
	After string `json:"after,omitempty"`
}

// KronoformHistorySpec defines the desired state of KronoformHistory
type KronoformHistorySpec struct {
	// Manifests contains the original YAML manifests that were applied
	// +required
	Manifests string `json:"manifests"`

	// SnapshotRef references the KronoformSnapshot that created this history
	// +required
	SnapshotRef string `json:"snapshotRef"`

	// Description provides a human-readable description
	// +optional
	Description string `json:"description,omitempty"`

	// AppliedBy indicates who/what applied the manifests
	// +optional
	AppliedBy string `json:"appliedBy,omitempty"`

	// ResourceTypes contains the list of resource types affected (e.g., ["ConfigMap", "Deployment"])
	// +optional
	ResourceTypes []string `json:"resourceTypes,omitempty"`

	// ResourceNames contains the list of resource names affected (e.g., ["my-configmap", "my-deployment"])
	// +optional
	ResourceNames []string `json:"resourceNames,omitempty"`

	// ResourceNamespaces contains the list of namespaces affected
	// +optional
	ResourceNamespaces []string `json:"resourceNamespaces,omitempty"`
}

// KronoformHistoryStatus defines the observed state of KronoformHistory
type KronoformHistoryStatus struct {
	// AppliedAt indicates when the manifests were applied
	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`

	// ResourceSnapshots contains the before/after states of affected resources
	// +optional
	ResourceSnapshots []ResourceSnapshot `json:"resourceSnapshots,omitempty"`

	// Summary provides a high-level summary of changes
	// +optional
	Summary string `json:"summary,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Snapshot",type="string",JSONPath=".spec.snapshotRef"
// +kubebuilder:printcolumn:name="Description",type="string",JSONPath=".spec.description"
// +kubebuilder:printcolumn:name="Applied By",type="string",JSONPath=".spec.appliedBy"
// +kubebuilder:printcolumn:name="Resource Types",type="string",JSONPath=".spec.resourceTypes"
// +kubebuilder:printcolumn:name="Applied At",type="date",JSONPath=".status.appliedAt"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KronoformHistory is the Schema for tracking the history of applied manifests
type KronoformHistory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KronoformHistorySpec   `json:"spec,omitempty"`
	Status KronoformHistoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KronoformHistoryList contains a list of KronoformHistory
type KronoformHistoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KronoformHistory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KronoformHistory{}, &KronoformHistoryList{})
}
