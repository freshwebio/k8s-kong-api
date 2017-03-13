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
	vhostPrefix string
}

// NewService creates a new service implementation.
func NewService(kongClient *kong.Client, k8sClient *k8sclient.Client,
	namespace string, routesLabel string, portLabel string, vhostPrefix string) *Service {
	return &Service{kongcli: kongClient, k8scli: k8sClient,
		namespace: namespace, routesLabel: routesLabel, portLabel: portLabel,
		vhostPrefix: vhostPrefix}
}

// Start begins our process in listening to k8s services and updating
// kong upstream services accordingly.
func (s *Service) Start() {
	log.Println("Starting the watcher service")
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
	// Now listen for services.
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
		log.Println("Stopping the watcher service")
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
	log.Println("Processing initial batch of services with the routes label")
	for _, srv := range services {
		s.process(&srv, watch.Added)
	}
}

// Deals with processing a k8s service event and carrying out the appropiate
// action to ensure the kong API gateway is up to date.
func (s *Service) process(srv *v1.Service, eventType watch.EventType) {
	pathStr, exists := srv.GetLabels()[s.routesLabel]
	if exists {
		log.Println("Processing service with the paths label: " + pathStr)
		name := srv.GetName()
		// Use cluster IP as all dns resolution or service names or internal k8s FQDNs
		// simply doesn't work correctly. If a cluster IP isn't set for the service then this will not work.
		clusterIP := srv.Spec.ClusterIP
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
				upstreams = append(upstreams, clusterIP+":"+strconv.Itoa(int(srv.Spec.Ports[i].Port)))
			}
		} else {
			for _, port := range srv.Spec.Ports {
				upstreams = append(upstreams, clusterIP+":"+strconv.Itoa(int(port.Port)))
			}
		}
		// Now lets get our paths from the label.
		// Unfortunately because of Kubernetes strict regular expression
		// for metadata labels we have to unconventionally use _ as the path separator.
		paths := strings.Split(pathStr, "_")
		// We also need to prepend / to the paths to make them as root uris.
		// Kubernetes labels don't accept / as a valid character so we have to preprocess
		// them instead.
		for i, path := range paths {
			paths[i] = "/" + path
		}
		switch eventType {
		case watch.Added:
			log.Println("Reacting to service added event")
			// Now we'll add our upstreams for the API or create our API to
			// add the upstreams to.
			err := s.addUpstreams(name, paths, upstreams)
			if err != nil {
				log.Println("Adding upstreams error: " + err.Error())
			}
		case watch.Modified:
			// Deals with adding new upstreams if any and removing upstreams
			// that no longer exist as well as updating the set of uris for the API
			// entry if changed.
			log.Println("Reacting to service update event")
			err := s.updateUpstreams(name, paths, upstreams)
			if err != nil {
				log.Println("Updating upstreams error: " + err.Error())
			}
		case watch.Deleted:
			// Removes the API entry and the upstream with the deleted service's name.
			log.Println("Reacting to service deletion event")
			err := s.removeUpstreams(name)
			if err != nil {
				log.Println("Removing upstreams error: " + err.Error())
			}
		}
	}
}

// Deals with updating our API entry and upstream/targets for the specified
// service according to changes in paths and provided upstream targets.
func (s *Service) updateUpstreams(name string, paths []string, upstreams []string) error {
	// First of all we'll load up the API.
	api, err := s.kongcli.GetAPI(name)
	if err != nil {
		return err
	}
	// Replace the list of paths if the list of paths differs.
	var differs bool
	if len(paths) != len(api.URIs) {
		differs = true
	} else {
		i := 0
		for !differs && i < len(paths) {
			if api.URIs[i] != paths[i] {
				differs = true
			} else {
				i++
			}
		}
	}
	if differs {
		api.URIs = paths
		_, err = s.kongcli.UpdateAPI(api)
		if err != nil {
			return err
		}
	}
	// Now we'll get the list of upstream targets to disable, enable
	// and add new targets accordingly.
	targets, err := s.kongcli.ListTargets(s.vhostPrefix + name)
	if err != nil {
		return err
	}
	newTargets := s.createNewTargets(upstreams, targets)
	for _, target := range newTargets {
		_, err := s.kongcli.CreateTarget(s.vhostPrefix+name, target)
		if err != nil {
			return err
		}
	}
	return nil
}

// Checks the difference between a list of kong targets and of the targets
// retrieved from a k8s service.
func (s *Service) createNewTargets(upstreams []string, targets *kong.TargetList) []*kong.Target {
	var newTargets []*kong.Target
	for _, target := range targets.Data {
		if targetInUpstreams(target.Target, upstreams) {
			// If the weight is set to 0 (The target is disabled) we'll add a new target
			// entry with a weight of 10.
			if target.Weight == 0 {
				newTarget := &kong.Target{
					Target: target.Target,
					Weight: 10,
				}
				newTargets = append(newTargets, newTarget)
			}
		} else {
			// Where the target is not in the set of upstreams
			// then we'll create a new entry which disables the target.
			if target.Weight > 0 {
				newTarget := &kong.Target{
					Target: target.Target,
					Weight: 0,
				}
				newTargets = append(newTargets, newTarget)
			}
		}
	}
	return newTargets
}

// Determines whether the specified target is in the provided set of upstreams.
func targetInUpstreams(target string, upstreams []string) bool {
	found := false
	i := 0
	for !found && i < len(upstreams) {
		if upstreams[i] == target {
			found = true
		} else {
			i++
		}
	}
	return found
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
	return s.kongcli.DeleteUpstream(s.vhostPrefix + name)
}

// Deals with adding upstreams to an exiting kong API entry
// or create a new API with the upstreams provided for the specified path.
func (s *Service) addUpstreams(serviceName string, paths []string, upstreams []string) error {
	// First check if an upstream exists for the provided service.
	_, err := s.kongcli.GetUpstream(s.vhostPrefix + serviceName)
	if err != nil {
		// If the Upstream doesn't exist already we'll create it.
		if err == kong.ErrNotFound {
			upstream := &kong.Upstream{Name: s.vhostPrefix + serviceName}
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
		_, err = s.kongcli.CreateTarget(s.vhostPrefix+serviceName, target)
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
				StripURI: false,
				// Ensure we prepend the upstream handle with http://
				// as only scheme prepended values are accepted for this field.
				UpstreamURL: "http://" + s.vhostPrefix + serviceName,
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
		log.Println("Beginning processing of k8s service watch event")
		s.process(srv, event.Type)
	}
}
