package gatewayapi

import (
	"encoding/json"

	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/meta"
	"k8s.io/client-go/pkg/api/unversioned"
)

// GatewayApi provides the type for an
// API plugin resource in Kubernetes.
type GatewayApi struct {
	unversioned.TypeMeta `json:",inline"`
	Metadata             api.ObjectMeta `json:"metadata"`
	Spec                 Spec           `json:"spec"`
}

// Event provides the event recieved for gateway api resource watchers.
type Event struct {
	Type   string     `json:"type"`
	Object GatewayApi `json:"object"`
}

// UpdateEvent provides the event recieved for gateway api resource watchers
// for update events specifically.
type UpdateEvent struct {
	Old GatewayApi `json:"old"`
	New GatewayApi `json:"new"`
}

// GetObjectKind provides the method to expose the kind
// of our GatewayApi object.
func (p *GatewayApi) GetObjectKind() unversioned.ObjectKind {
	return &p.TypeMeta
}

// GetObjectMeta Retrieves the metadata for the GatewayApi.
func (p *GatewayApi) GetObjectMeta() meta.Object {
	return &p.Metadata
}

// GACopy provides an alias of the GatewayApi to be utilised
// in unmarshalling of JSON data.
type GACopy GatewayApi

// UnmarshalJSON provides the way in which JSON should be unmarshalled correctly for this type.
// This is a temporary workaround for https://github.com/kubernetes/client-go/issues/8
func (p *GatewayApi) UnmarshalJSON(data []byte) error {
	tmp := GACopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := GatewayApi(tmp)
	*p = tmp2
	return nil
}

// GatewayApiList provides the type encapsulating a list of GatewayApi resources.
type GatewayApiList struct {
	unversioned.TypeMeta `json:",inline"`
	Metadata             unversioned.ListMeta `json:"metadata"`
	Items                []GatewayApi         `json:"items"`
}

// GetObjectKind provides the method to expose the kind
// of our GatewayApi List object.
func (l *GatewayApiList) GetObjectKind() unversioned.ObjectKind {
	return &l.TypeMeta
}

// GetListMeta Retrieves the metadata for the GatewayApi List.
func (l *GatewayApiList) GetListMeta() unversioned.List {
	return &l.Metadata
}

// ListCopy provides the type alias for list to be used in unmarshalling from JSON.
type ListCopy GatewayApiList

// UnmarshalJSON provides the way in which JSON should be unmarshalled correctly for this list type.
// Temporary workaround for https://github.com/kubernetes/client-go/issues/8
func (l *GatewayApiList) UnmarshalJSON(data []byte) error {
	tmp := ListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := GatewayApiList(tmp)
	*l = tmp2
	return nil
}

// Spec provides the type for the specification
// of the plugin resource specification.
// The name and upstream url of the API to be created in kong are
// taken from the service in the selector.
type Spec struct {
	Hosts                  []string `json:"hosts,omitempty"`
	Uris                   []string `json:"uris,omitempty"`
	StripURI               *bool    `json:"strip_uri,omitempty"`
	Methods                []string `json:"methods,omitempty"`
	PreserveHost           *bool    `json:"preserve_host,omitempty"`
	Retries                int64    `json:"retries,omitempty"`
	UpstreamConnectTimeout int64    `json:"upstream_connect_timeout,omitempty"`
	UpstreamSendTimeout    int64    `json:"upstream_send_timeout,omitempty"`
	UpstreamReadTimeout    int64    `json:"upstream_read_timeout,omitempty"`
	HTTPSOnly              *bool    `json:"https_only,omitempty"`
	HTTPIfTerminated       *bool    `json:"http_if_terminated,omitempty"`
	// Label selector for selecting the services the GatewayApi resource
	// represents. This will then create a new API object
	// in Kong for the configuration and service upstream host.
	Selector map[string]string `json:"selector"`
}
