package k8stypes

import "k8s.io/client-go/pkg/api/v1"

// ServiceEvent provides the event recieved for service watchers.
type ServiceEvent struct {
	Type   string     `json:"type"`
	Object v1.Service `json:"object"`
}

// ServiceUpdateEvent provides the event recieved for service watchers
// for update events.
type ServiceUpdateEvent struct {
	Old v1.Service `json:"old"`
	New v1.Service `json:"new"`
}
