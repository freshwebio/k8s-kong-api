package apiplugin

import (
	"encoding/json"

	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/meta"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
)

// APIPlugin provides the type for an
// API plugin resource in Kubernetes. In Kubernetes
// due to the naming convention constraints the kind of the Third Party Resource
// is ApiPlugin and not APIPlugin.
type APIPlugin struct {
	unversioned.TypeMeta `json:",inline"`
	Metadata             api.ObjectMeta `json:"metadata"`
	Spec                 Spec           `json:"spec"`
}

// Event provides the event recieved for plugin resource watchers.
type Event struct {
	Type   string    `json:"type"`
	Object APIPlugin `json:"object"`
}

// ServeiceEvent provides the event recieved for service watchers.
type ServiceEvent struct {
	Type   string     `json:"type"`
	Object v1.Service `json:"object"`
}

// GetObjectKind provides the method to expose the kind
// of our APIPlugin object.
func (p *APIPlugin) GetObjectKind() unversioned.ObjectKind {
	return &p.TypeMeta
}

// GetObjectMeta Retrieves the metadata for the APIPlugin.
func (p *APIPlugin) GetObjectMeta() meta.Object {
	return &p.Metadata
}

// APCopy provides an alias of the APIPlugin to be utilised
// in unmarshalling of JSON data.
type APCopy APIPlugin

// UnmarshalJSON provides the way in which JSON should be unmarshalled correctly for this type.
// This is a temporary workaround for https://github.com/kubernetes/client-go/issues/8
func (p *APIPlugin) UnmarshalJSON(data []byte) error {
	tmp := APCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := APIPlugin(tmp)
	*p = tmp2
	return nil
}

// List provides the type encapsulating a list of APIPlugin resources.
type List struct {
	unversioned.TypeMeta `json:",inline"`
	Metadata             unversioned.ListMeta `json:"metadata"`
	Items                []APIPlugin          `json:"items"`
}

// GetObjectKind provides the method to expose the kind
// of our APIPlugin List object.
func (l *List) GetObjectKind() unversioned.ObjectKind {
	return &l.TypeMeta
}

// GetListMeta Retrieves the metadata for the APIPlugin List.
func (l *List) GetListMeta() unversioned.List {
	return &l.Metadata
}

// ListCopy provides the type alias for list to be used in unmarshalling from JSON.
type ListCopy List

// UnmarshalJSON provides the way in which JSON should be unmarshalled correctly for this list type.
// Temporary workaround for https://github.com/kubernetes/client-go/issues/8
func (l *List) UnmarshalJSON(data []byte) error {
	tmp := ListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := List(tmp)
	*l = tmp2
	return nil
}

// Spec provides the type for the specification
// of the plugin resource specification.
type Spec struct {
	// The name of the plugin to be attached to a specified
	// k8s service that also represents a Kong API object.
	Name string `json:"name"`
	// Configuration for the plugin as expected by Kong.
	// Keys in this map should avoid the config. prefix
	// as will be automatically prepended when requests are made to Kong.
	Config map[string]interface{} `json:"config"`
	// Label selector for selecting the services the APIPlugin resource
	// should be attached to. This will then create a new plugin on the API object
	// in Kong.
	Selector map[string]string `json:"selector"`
}

// Data provides the data to be persisted about
// a plugin object in Kong.
type Data struct {
	ID string `json:"id"`
}
