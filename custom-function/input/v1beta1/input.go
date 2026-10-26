// Package v1beta1 contains the input type for function-resource-labeler.
// +kubebuilder:object:generate=true
// +groupName=resourcelabeler.fn.example.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we ship a CRD (package/input) that describes its schema; `crossplane xpkg build`
// embeds it so tooling can validate a pipeline step's input.

// Input is the input of one pipeline step that uses this function.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Labels are set on every composed resource. They win over a label with the same key that an earlier step set.
	Labels map[string]string `json:"labels"`
}
