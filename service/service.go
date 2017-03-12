package service

import (
	"log"
	"strconv"
	"strings"

	"github.com/freshwebio/k8s-kong-api/k8sclient"
	"github.com/freshwebio/k8s-kong-api/kong"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/watch"
)

// Service provides the type for the service which deals with
// listening to kubernetes events to update kong APIs, upstreams and targets etc..
type Service struct {
	kongcli     *kong.Client
	k8scli      *k8sclient.Client
	namespace   string
	routesLabel string
	portLabel   string
	watcher     watch.Interface
}

// NewService creates a new service implementation.
func NewService(kongClient *kong.Client, k8sClient *k8sclient.Client,
	namespace string, routesLabel string, portLabel string) *Service {
	return &Service{kongcli: kongClient, k8scli: k8sClient,
		namespace: namespace, routesLabel: routesLabel, portLabel: portLabel}
}

// Start begins our process in listening to k8s pods and updating
// kong upstream services accordingly.
func (s *Service) Start() {
	// First of all list existing services and spawn a new goroutine to deal with
	// adding entries to kong accordingly while we begin watching service events.
	existingList, err := s.k8scli.ListServices(s.namespace, s.routesLabel)
	if err != nil {
		panic(err.Error())
	}
	go s.processBatch(existingList.Items)
	// Now we'll start watching pod events.
	watcher, err := s.k8scli.WatchServices(s.namespace, s.routesLabel)
	if err != nil {
		panic(err.Error())
	}
	// Set our watcher so we can stop the streaming from elsewhere at the service level.
	s.watcher = watcher
	// Now listen for pods.
	resChan := watcher.ResultChan()
	for {
		select {
		case event := <-resChan:
			s.processEvent(event)
		}
	}
}

// Stop deals with stopping the watcher listening for kubernetes
// service events.
func (s *Service) Stop() {
	if s.watcher != (watch.Interface)(nil) {
		s.watcher.Stop()
	}
}

// Deals with processing a batch of services and creates
// kong APIs, upstreams and targets accordingly.
// This is only to be used when creating new kong entries
// for k8s services. This is expected to be used when the app
// is first loaded to load all the existing services labelled accordingly
// as kong upstream services.
func (s *Service) processBatch(services []v1.Service) {
	for _, srv := range services {
		s.process(&srv, watch.Added)
	}
}

// Deals with processing a k8s service event and carrying out the appropiate
// action to ensure the kong API gateway is up to date.
func (s *Service) process(srv *v1.Service, eventType watch.EventType) {
	pathStr, exists := srv.GetLabels()[s.routesLabel]
	if exists {
		name := srv.GetName()
		upstreams := []string{}
		// Now for each of the ports the service is exposing
		// create a new upstream target entry or if the routes label
		// is set with a reference to the port name use that instead.
		if portName, exists := srv.GetLabels()[s.portLabel]; exists {
			found := false
			i := 0
			for !found && i < len(srv.Spec.Ports) {
				if srv.Spec.Ports[i].Name == portName {
					found = true
				} else {
					i++
				}
			}
			if found {
				upstreams = append(upstreams, name+":"+strconv.Itoa(int(srv.Spec.Ports[i].Port)))
			}
		} else {
			for _, port := range srv.Spec.Ports {
				upstreams = append(upstreams, name+":"+strconv.Itoa(int(port.Port)))
			}
		}
		// Now lets get our paths from the label.
		paths := strings.Split(pathStr, ",")
		switch eventType {
		case watch.Added:
			// Now we'll add our upstreams for the API or create our API to
			// add the upstreams to.
			s.addUpstreams(name, paths, upstreams)
		case watch.Modified:
			// Deals with adding new upstreams if any and removing upstreams
			// that no longer exist as well as updating the set of uris for the API
			// entry if changed.
			s.updateUpstreams(name, paths, upstreams)
		case watch.Deleted:
			// Removes the API entry and the upstream with the deleted service's name.
			s.removeUpstreams(name)
		}
	}
}

// Deals with updating our API entry and upstream/targets for the specified
// service according to changes in paths and provided upstream targets.
func (s *Service) updateUpstreams(name string, paths []string, upstreams []string) error {
	return nil
}

// Deals with removing the API entry and upstream entry
// with the provided service name from the Kong API gateway.
func (s *Service) removeUpstreams(name string) error {
	// First remove the API.
	err := s.kongcli.DeleteAPI(name)
	if err != nil {
		// If something went wrong simply return the error.
		return err
	}
	// Now remove the upstream entry with the same service name.
	return s.kongcli.DeleteUpstream(name)
}

// Deals with adding upstreams to an exiting kong API entry
// or create a new API with the upstreams provided for the specified path.
func (s *Service) addUpstreams(serviceName string, paths []string, upstreams []string) error {
	// First check if an upstream exists for the provided service.
	_, err := s.kongcli.GetUpstream(serviceName)
	if err != nil {
		// If the Upstream doesn't exist already we'll create it.
		if err == kong.ErrNotFound {
			upstream := &kong.Upstream{Name: serviceName}
			_, err = s.kongcli.CreateUpstream(upstream)
			if err != nil {
				return err
			}
		}
	}
	// Now lets create our targets for the upstream we either have
	// just created or loaded.
	for _, upstream := range upstreams {
		target := &kong.Target{
			Target: upstream,
			Weight: 10,
		}
		_, err = s.kongcli.CreateTarget(serviceName, target)
		if err != nil {
			return err
		}
	}
	// Now check if an API for the service.
	_, err = s.kongcli.GetAPI(serviceName)
	if err != nil {
		// If the API doesn't exist let's create one.
		if err == kong.ErrNotFound {
			api := &kong.API{
				Name: serviceName,
				URIs: paths,
				// Don't strip the URI as a single upstream service
				// can take multiple URIs.
				// TODO: Allow for APIs that strip the URI.
				StripURI:    false,
				UpstreamURL: serviceName,
			}
			_, err = s.kongcli.CreateAPI(api)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Deals with processing an event on a routes label
// labelled service.
func (s *Service) processEvent(event watch.Event) {
	srv, ok := event.Object.(*v1.Service)
	if ok {
		s.process(srv, event.Type)
	} else {
		log.Println("The event object was expected to be a " +
			"pointer to a service object but got something else instead.")
	}
}
