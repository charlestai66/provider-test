/*
Copyright 2022 The Crossplane Authors.

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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// TestTypeParameters are the configurable fields of a TestType.
type TestTypeParameters struct {
	ConfigurableField string `json:"configurableField"`
	PolicyId          string `json:"policyId"`
	SslPath           string `json:"sslPath"`
}

// TestTypeObservation are the observable fields of a TestType.
type TestTypeObservation struct {
	ObservableField string `json:"observableField,omitempty"`
}

// A TestTypeSpec defines the desired state of a TestType.
type TestTypeSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       TestTypeParameters `json:"forProvider"`
}

// A TestTypeStatus represents the observed state of a TestType.
type TestTypeStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          TestTypeObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A TestType is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,test}
type TestType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestTypeSpec   `json:"spec"`
	Status TestTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TestTypeList contains a list of TestType
type TestTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestType `json:"items"`
}

// TestType type metadata.
var (
	TestTypeKind             = reflect.TypeOf(TestType{}).Name()
	TestTypeGroupKind        = schema.GroupKind{Group: Group, Kind: TestTypeKind}.String()
	TestTypeKindAPIVersion   = TestTypeKind + "." + SchemeGroupVersion.String()
	TestTypeGroupVersionKind = SchemeGroupVersion.WithKind(TestTypeKind)
)

func init() {
	SchemeBuilder.Register(&TestType{}, &TestTypeList{})
}
