package v1alpha1

import (
	"time"

	operatorapi "github.com/openshift/api/operator/v1alpha1"
	metaapi "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PruneServiceList struct {
	metaapi.TypeMeta `json:",inline"`
	metaapi.ListMeta `json:"metadata"`
	Items            []PruneService `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PruneService struct {
	metaapi.TypeMeta   `json:",inline"`
	metaapi.ObjectMeta `json:"metadata"`
	Spec               PruneServiceSpec   `json:"spec"`
	Status             PruneServiceStatus `json:"status,omitempty"`
}

type PruneServiceImages struct {
	KeepTagRevisions   *int           `json:"keepTagRevisions"`
	KeepTagYoungerThan *time.Duration `json:"keepYoungerThan"`
}

type PruneServiceSpec struct {
	Images PruneServiceImages `json:"images"`
}
type PruneServiceStatus struct {
	operatorapi.OperatorStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ImageReferenceList struct {
	metaapi.TypeMeta `json:",inline"`
	metaapi.ListMeta `json:"metadata"`
	Items            []ImageReference `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ImageReference struct {
	metaapi.TypeMeta   `json:",inline"`
	metaapi.ObjectMeta `json:"metadata"`
	Spec               ImageReferenceSpec `json:"spec"`
}

type ImageReferenceSpec struct {
	APIVersion          string   `json:"apiVersion"`
	Group               string   `json:"group"`
	Kind                string   `json:"kind"`
	Name                string   `json:"name"`
	Namespaced          bool     `json:"namespaced"`
	ImageFieldSelectors []string `json:"imageFieldSelectors"`
}
