package apiplugin

import (
	"encoding/json"

	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/meta"
	"k8s.io/client-go/pkg/api/unversioned"
)

// ApiPlugin provides the type for an
// API plugin resource in Kubernetes.
type ApiPlugin struct {
	unversioned.TypeMeta `json:",inline"`
	Metadata             api.ObjectMeta `json:"metadata"`
	Spec                 Spec           `json:"spec"`
}

// Event provides the event recieved for plugin resource watchers.
type Event struct {
	Type   string    `json:"type"`
	Object ApiPlugin `json:"object"`
}

// GetObjectKind provides the method to expose the kind
// of our ApiPlugin object.
func (p *ApiPlugin) GetObjectKind() unversioned.ObjectKind {
	return &p.TypeMeta
}

// GetObjectMeta Retrieves the metadata for the ApiPlugin.
func (p *ApiPlugin) GetObjectMeta() meta.Object {
	return &p.Metadata
}

// APCopy provides an alias of the ApiPlugin to be utilised
// in unmarshalling of JSON data.
type APCopy ApiPlugin

// UnmarshalJSON provides the way in which JSON should be unmarshalled correctly for this type.
// This is a temporary workaround for https://github.com/kubernetes/client-go/issues/8
func (p *ApiPlugin) UnmarshalJSON(data []byte) error {
	tmp := APCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := ApiPlugin(tmp)
	*p = tmp2
	return nil
}

// ApiPluginList provides the type encapsulating a list of ApiPlugin resources.
type ApiPluginList struct {
	unversioned.TypeMeta `json:",inline"`
	Metadata             unversioned.ListMeta `json:"metadata"`
	Items                []ApiPlugin          `json:"items"`
}

// GetObjectKind provides the method to expose the kind
// of our ApiPlugin List object.
func (l *ApiPluginList) GetObjectKind() unversioned.ObjectKind {
	return &l.TypeMeta
}

// GetListMeta Retrieves the metadata for the ApiPlugin List.
func (l *ApiPluginList) GetListMeta() unversioned.List {
	return &l.Metadata
}

// ListCopy provides the type alias for list to be used in unmarshalling from JSON.
type ListCopy ApiPluginList

// UnmarshalJSON provides the way in which JSON should be unmarshalled correctly for this list type.
// Temporary workaround for https://github.com/kubernetes/client-go/issues/8
func (l *ApiPluginList) UnmarshalJSON(data []byte) error {
	tmp := ListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := ApiPluginList(tmp)
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
	// Label selector for selecting the services the ApiPlugin resource
	// should be attached to. This will then create a new plugin on the API object
	// in Kong.
	Selector map[string]string `json:"selector"`
}
