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

// KronoformSnapshotSpec defines the desired state of KronoformSnapshot
type KronoformSnapshotSpec struct {
	// Manifests contains the YAML manifests to apply
	// +required
	Manifests string `json:"manifests"`

	// Description provides a human-readable description of this snapshot
	// +optional
	Description string `json:"description,omitempty"`

	// DryRun performs a dry run without actually applying the manifests
	// +optional
	DryRun bool `json:"dryRun,omitempty"`

	// TargetNamespace specifies the namespace to apply manifests to
	// If empty, uses the namespace from manifest or default
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`
}

// KronoformSnapshotStatus defines the observed state of KronoformSnapshot
type KronoformSnapshotStatus struct {
	// Phase represents the current phase of the snapshot
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message provides detailed information about the current phase
	// +optional
	Message string `json:"message,omitempty"`

	// AppliedAt indicates when the manifests were successfully applied
	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`

	// HistoryRef references the created KronoformHistory resource
	// +optional
	HistoryRef string `json:"historyRef,omitempty"`

	// Conditions represent the latest available observations of the snapshot's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Description",type="string",JSONPath=".spec.description"
// +kubebuilder:printcolumn:name="Applied At",type="date",JSONPath=".status.appliedAt"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KronoformSnapshot is the Schema for applying manifests with history tracking
type KronoformSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KronoformSnapshotSpec   `json:"spec,omitempty"`
	Status KronoformSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KronoformSnapshotList contains a list of KronoformSnapshot
type KronoformSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KronoformSnapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KronoformSnapshot{}, &KronoformSnapshotList{})
}
