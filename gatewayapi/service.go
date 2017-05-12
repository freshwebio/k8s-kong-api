package gatewayapi

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/freshwebio/k8s-kong-api/k8sclient"
	"github.com/freshwebio/k8s-kong-api/k8stypes"
	"github.com/freshwebio/k8s-kong-api/kong"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/labels"
	"k8s.io/client-go/pkg/selection"
	"k8s.io/client-go/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var (
	// ErrGatewayNotFound should be used when a gateway can't be found in the Kubernetes cluster.
	ErrGatewayNotFound = errors.New("Could not find the specifed GatewayApi resource in Kubernetes")
	// ErrServiceNotFound should be used when a service resource cannot be found in the Kubernetes cluster.
	ErrServiceNotFound = errors.New("Could not find the specified v1.Service resources in Kubernetes")
)

// Service deals with monitoring and responding
// to events on gateway api resources in k8s
// and updating the Kong representations accordingly.
type Service struct {
	k8sRestClient        *rest.RESTClient
	k8sClient            *k8sclient.Client
	apiLabel             string
	serviceSelectorLabel string
	namespace            string
	kongClient           *kong.Client
}

// NewService creates a new instance of the GatewayApi service.
func NewService(k8sRestClient *rest.RESTClient, k8sClient *k8sclient.Client, kong *kong.Client, namespace string,
	apiLabel string, serviceSelectorLabel string) *Service {
	return &Service{k8sRestClient: k8sRestClient, k8sClient: k8sClient, kongClient: kong, namespace: namespace,
		apiLabel: apiLabel, serviceSelectorLabel: serviceSelectorLabel}
}

// Start deals with beginning the monitoring process which deals with monitoring
// events from k8s gatewayapi resources as well as services to propogate changes to kong.
// This method should be called asynchronously in it's own goroutine.
func (s *Service) Start(doneChan <-chan struct{}, wg *sync.WaitGroup) {
	log.Println("Starting the gatewayapi watcher service")
	// Let's monitor our service and plugin events.
	selector := labels.NewSelector()
	req, err := labels.NewRequirement(s.apiLabel, selection.Exists, []string{})
	if err != nil {
		log.Fatal(err)
	}
	selector.Add(*req)
	serviceEvents, serviceUpdateEvents := s.monitorServiceEvents(s.namespace, selector, doneChan)
	gatewayApiEvents, gatewayApiUpdateEvents := s.monitorGatewayApiEvents(s.namespace, selector, doneChan)
	for {
		select {
		case event := <-gatewayApiEvents:
			err := s.processGatewayApiEvent(event)
			if err != nil {
				log.Printf("Error while processing gateway api event: %v", err)
			}
		case event := <-gatewayApiUpdateEvents:
			err := s.processGatewayApiUpdateEvent(event)
			if err != nil {
				log.Printf("Error while processing gateway api update event: %v", err)
			}
		case event := <-serviceUpdateEvents:
			err := s.processServiceUpdateEvent(event)
			if err != nil {
				log.Printf("Error while processing service update event: %v", err)
			}
		case event := <-serviceEvents:
			err := s.processServiceEvent(event)
			if err != nil {
				log.Printf("Error while processing service event: %v", err)
			}
		case <-doneChan:
			wg.Done()
			log.Println("Stopped gateway api event watcher.")
		}
	}
}

// Handles processing the service events we are interested in for the sake
// of our gateway api resources.
func (s *Service) processServiceEvent(e k8stypes.ServiceEvent) error {
	if e.Type == "ADDED" {
		err := s.createKongGatewayApiForService(e.Object)
		if err != nil {
			return err
		}
	}
	return nil
}

// Handles processing the service update events we are interested in for the sake
// of our gateway api resources.
func (s *Service) processServiceUpdateEvent(e k8stypes.ServiceUpdateEvent) error {
	err := s.updateKongGatewayApiForService(e.Old, e.New)
	if err != nil {
		return err
	}
	return nil
}

// Creates a new kong API object if a gateway exists for the provided service.
func (s *Service) createKongGatewayApiForService(v1s v1.Service) error {
	// First of all we want to make sure that the provided service has the gateway API reference label
	// set and extract the name of the gateway api object from that.
	if gatewayApiName, exists := v1s.Labels[s.apiLabel]; exists {
		gatewayApi, err := s.getGatewayApi(gatewayApiName)
		if err != nil {
			return err
		}

		// Now let's attempt to create our upstream URL for the service, if no ports
		// are provided then we won't create the API object as something is wrong with the service.
		// Also when a service is exposing multiple ports the first one will always be used.
		// TODO: Implement functionality that allows selection of port to be used for a Kong
		// upstream when a service is exposing multiple ports.
		upstreamURL := v1s.Spec.ClusterIP
		if len(v1s.Spec.Ports) > 0 {
			upstreamURL += ":" + strconv.Itoa(int(v1s.Spec.Ports[0].Port))
		} else {
			return fmt.Errorf("The service %v should expose at least one port", v1s.GetName())
		}

		// Only proceed if an API object with the provided name doesn't already exist, in what would be assumed
		// to be a rare case a GatewayApi resource
		// might still be around after a previous deletion of the same or similar service.
		_, err = s.kongClient.GetAPI(v1s.GetName())
		if err != nil && err == kong.ErrNotFound {
			// Now let's create our new API object for the retrieved GatewayApi resource.
			api := &kong.API{
				Name:                   v1s.GetName(),
				Hosts:                  gatewayApi.Spec.Hosts,
				URIs:                   gatewayApi.Spec.Uris,
				UpstreamURL:            upstreamURL,
				StripURI:               gatewayApi.Spec.StripURI,
				Methods:                gatewayApi.Spec.Methods,
				PreserveHost:           gatewayApi.Spec.PreserveHost,
				Retries:                gatewayApi.Spec.Retries,
				UpstreamConnectTimeout: gatewayApi.Spec.UpstreamConnectTimeout,
				UpstreamSendTimeout:    gatewayApi.Spec.UpstreamSendTimeout,
				UpstreamReadTimeout:    gatewayApi.Spec.UpstreamReadTimeout,
				HTTPSOnly:              gatewayApi.Spec.HTTPSOnly,
				HTTPIfTerminated:       gatewayApi.Spec.HTTPIfTerminated,
			}
			_, err = s.kongClient.CreateAPI(api)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Updates the upstream URL of a Kong API object if the service upstream has changed.
// We assume if the API object exist ins in kong then a GatewayApi resource exists in k8s.
// The above may not always be the case but it saves an extra call to the k8s apiserver.
// TODO: Make it work for selecting either a named port or the port number from a range on a single service.
func (s *Service) updateKongGatewayApiForService(old v1.Service, new v1.Service) error {
	// Only proceed if there is a change in the upstream URL.
	oldUpstreamURL := old.Spec.ClusterIP
	newUpstreamURL := new.Spec.ClusterIP
	if len(old.Spec.Ports) > 0 && len(new.Spec.Ports) > 0 {
		oldUpstreamURL += ":" + strconv.Itoa(int(old.Spec.Ports[0].Port))
		newUpstreamURL += ":" + strconv.Itoa(int(new.Spec.Ports[0].Port))
	} else {
		return fmt.Errorf("The service %v should expose at least one port", new.GetName())
	}
	if oldUpstreamURL != newUpstreamURL {
		// Now make sure an API object exists for the provided service.
		api, err := s.kongClient.GetAPI(new.GetName())
		if err != nil {
			return err
		}
		// Let's update the retrieved API object.
		api.UpstreamURL = newUpstreamURL
		_, err = s.kongClient.UpdateAPI(api)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) processGatewayApiEvent(e Event) error {
	switch e.Type {
	case "ADDED":
		err := s.createKongGatewayApi(e.Object)
		if err != nil {
			return err
		}
	case "DELETED":
		err := s.deleteKongGatewayApi(e.Object)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) processGatewayApiUpdateEvent(e UpdateEvent) error {
	err := s.updateKongGatewayApi(e.Old, e.New)
	if err != nil {
		return err
	}
	return nil
}

// Creates a new API object in kong if one for the provided service selector
// doesn't already exist and the service referenced does.
func (s *Service) createKongGatewayApi(a GatewayApi) error {
	if serviceName, exists := a.Spec.Selector[s.serviceSelectorLabel]; exists {
		_, err := s.kongClient.GetAPI(serviceName)
		if err != nil {
			if err == kong.ErrNotFound {
				service, err := s.getServiceByServiceLabelSelector(serviceName)
				if err != nil {
					return err
				}
				// Let's get the upstream URL from the service.
				upstreamURL := service.Spec.ClusterIP
				if len(service.Spec.Ports) > 0 {
					upstreamURL += ":" + strconv.Itoa(int(service.Spec.Ports[0].Port))
				} else {
					return fmt.Errorf("The service %v should expose at least one port", service.GetName())
				}
				api := &kong.API{
					Name:                   service.GetName(),
					Hosts:                  a.Spec.Hosts,
					URIs:                   a.Spec.Uris,
					UpstreamURL:            upstreamURL,
					StripURI:               a.Spec.StripURI,
					Methods:                a.Spec.Methods,
					PreserveHost:           a.Spec.PreserveHost,
					Retries:                a.Spec.Retries,
					UpstreamConnectTimeout: a.Spec.UpstreamConnectTimeout,
					UpstreamSendTimeout:    a.Spec.UpstreamSendTimeout,
					UpstreamReadTimeout:    a.Spec.UpstreamReadTimeout,
					HTTPSOnly:              a.Spec.HTTPSOnly,
					HTTPIfTerminated:       a.Spec.HTTPIfTerminated,
				}
				_, err = s.kongClient.CreateAPI(api)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}
	return nil
}

// Updates the kong API object if the same service is referenced
// otherwise destroys the API object for the old service and creates
// a new API object for the newly referenced service.
func (s *Service) updateKongGatewayApi(old GatewayApi, new GatewayApi) error {
	oldService, oldExists := old.Spec.Selector[s.serviceSelectorLabel]
	newService, newExists := new.Spec.Selector[s.serviceSelectorLabel]
	if !oldExists || !newExists {
		return fmt.Errorf("The gateway api resource %v must have a service selector set", new.Metadata.GetName())
	}
	// Load the new service from k8s. We don't need to load the old service
	// As we only need to delete an API object if one exists for it.
	srvObj, err := s.getServiceByServiceLabelSelector(newService)
	if err != nil {
		return err
	}
	upstreamURL := srvObj.Spec.ClusterIP
	if len(srvObj.Spec.Ports) > 0 {
		upstreamURL += ":" + strconv.Itoa(int(srvObj.Spec.Ports[0].Port))
	} else {
		return fmt.Errorf("The service %v should expose at least one port", srvObj.GetName())
	}
	// Create our new API object either to be saved anew or updated.
	api := &kong.API{
		Name:                   srvObj.GetName(),
		Hosts:                  new.Spec.Hosts,
		URIs:                   new.Spec.Uris,
		UpstreamURL:            upstreamURL,
		StripURI:               new.Spec.StripURI,
		Methods:                new.Spec.Methods,
		PreserveHost:           new.Spec.PreserveHost,
		Retries:                new.Spec.Retries,
		UpstreamConnectTimeout: new.Spec.UpstreamConnectTimeout,
		UpstreamSendTimeout:    new.Spec.UpstreamSendTimeout,
		UpstreamReadTimeout:    new.Spec.UpstreamReadTimeout,
		HTTPSOnly:              new.Spec.HTTPSOnly,
		HTTPIfTerminated:       new.Spec.HTTPIfTerminated,
	}
	if oldService == newService {
		// Simply update the Kong API object.
		_, err = s.kongClient.UpdateAPI(api)
		if err != nil {
			return err
		}
	} else {
		// Delete the API object for the old service and add a new one for our new service.
		_, err := s.kongClient.GetAPI(oldService)
		if err != nil {
			// Only quit when the error is not error not found.
			if err != kong.ErrNotFound {
				return err
			}
		} else {
			// Delete the API object from the old service reference.
			err = s.kongClient.DeleteAPI(oldService)
			if err != nil {
				return err
			}
		}
		// Now we'll create the new API object.
		_, err = s.kongClient.CreateAPI(api)
		if err != nil {
			return err
		}
	}
	return nil
}

// Deletes the API object in kong the provided GatewayApi represents.
func (s *Service) deleteKongGatewayApi(a GatewayApi) error {
	if apiName, exists := a.Spec.Selector[s.serviceSelectorLabel]; exists {
		// Only delete the API object if it already exists.
		_, err := s.kongClient.GetAPI(apiName)
		if err != nil {
			if err == kong.ErrNotFound {
				// Don't do anything as the API object doesn't exist.
				// Also this should not indicate an error so return nil.
				return nil
			} else {
				return err
			}
		} else {
			err = s.kongClient.DeleteAPI(apiName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Writes service events from k8s to a new channel to be consumed.
func (s *Service) monitorServiceEvents(
	namespace string,
	selector labels.Selector,
	done <-chan struct{}) (<-chan k8stypes.ServiceEvent, <-chan k8stypes.ServiceUpdateEvent) {
	events := make(chan k8stypes.ServiceEvent)
	updateEvents := make(chan k8stypes.ServiceUpdateEvent)
	eventCallback := func(evType watch.EventType, obj interface{}) {
		service, ok := obj.(*v1.Service)
		if !ok {
			log.Printf("could not convert %v (%T) into Service", obj, obj)
			return
		}
		events <- k8stypes.ServiceEvent{
			Type:   string(evType),
			Object: *service,
		}
	}
	updateEventCallback := func(evType watch.EventType, old, new interface{}) {
		oldSrv, ook := old.(*v1.Service)
		newSrv, nok := new.(*v1.Service)
		if !(ook && nok) {
			log.Printf("could not convert %v (%T) and %v (%T) into Services", old, old, new, new)
			return
		}
		updateEvents <- k8stypes.ServiceUpdateEvent{
			Old: *oldSrv,
			New: *newSrv,
		}
	}
	source := k8sclient.NewListWatchFromClient(s.k8sClient.Clientset.CoreV1().RESTClient(), "services", namespace, selector)
	store, ctrl := cache.NewInformer(source, &v1.Service{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			eventCallback(watch.Added, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			updateEventCallback(watch.Modified, old, new)
		},
		DeleteFunc: func(obj interface{}) {
			eventCallback(watch.Deleted, obj)
		},
	})

	go func() {
		for _, initObj := range store.List() {
			eventCallback(watch.Added, initObj)
		}

		go ctrl.Run(done)
	}()

	return events, updateEvents
}

// Handles watching events occuring for our custom plugin resource.
// All GatewayApi resources in the given namespace and selector combination are watched in this case.
func (s *Service) monitorGatewayApiEvents(
	namespace string,
	selector labels.Selector,
	done <-chan struct{}) (<-chan Event, <-chan UpdateEvent) {
	events := make(chan Event)
	updateEvents := make(chan UpdateEvent)
	eventCallback := func(evType watch.EventType, obj interface{}) {
		gatewayApi, ok := obj.(*GatewayApi)
		if !ok {
			log.Printf("could not convert %v (%T) into ApiPlugin", obj, obj)
			return
		}
		events <- Event{
			Type:   string(evType),
			Object: *gatewayApi,
		}
	}
	updateEventCallback := func(evType watch.EventType, old, new interface{}) {

	}
	source := k8sclient.NewListWatchFromClient(s.k8sRestClient, "gatewayapis", namespace, selector)
	store, ctrl := cache.NewInformer(source, &GatewayApi{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			eventCallback(watch.Added, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			updateEventCallback(watch.Modified, old, new)
		},
		DeleteFunc: func(obj interface{}) {
			eventCallback(watch.Deleted, obj)
		},
	})

	go func() {
		for _, initObj := range store.List() {
			eventCallback(watch.Added, initObj)
		}

		go ctrl.Run(done)
	}()

	return events, updateEvents
}

// Attempts to retrieve a GatewayApi resource with the provided name.
// The assumption that should be made is if there is in error then the resource
// isn't reachable or doesn't exist so carry on doing other stuff instead of functionality
// dependant on getting the gateway API object.
func (s *Service) getGatewayApi(name string) (*GatewayApi, error) {
	obj, err := s.k8sRestClient.Get().
		Namespace(s.namespace).
		Resource("gatewayapis").
		Name(name).
		Do().
		Get()
	if err != nil {
		return nil, err
	}
	gatewayApiList, ok := obj.(*GatewayApiList)
	if !ok {
		err := fmt.Errorf("could not convert %v (%T) into GatewayApiList", obj, obj)
		log.Println(err)
		return nil, err
	}
	if len(gatewayApiList.Items) > 0 {
		gatewayApi := gatewayApiList.Items[0]
		return &gatewayApi, nil
	}
	return nil, ErrGatewayNotFound
}

// Attempts to retrieve a service by it's service label selector.
// This will only query services with the api label set. e.g. kong.gateway.api
func (s *Service) getServiceByServiceLabelSelector(value string) (*v1.Service, error) {
	selector := labels.NewSelector()
	req, err := labels.NewRequirement(s.serviceSelectorLabel, selection.Equals, []string{value})
	if err != nil {
		return nil, err
	}
	selector.Add(*req)
	// We also need to add a requirement to limit the range to services that are enabled for Gateway APIs.
	req2, err := labels.NewRequirement(s.apiLabel, selection.Exists, []string{})
	if err != nil {
		return nil, err
	}
	selector.Add(*req2)
	obj, err := s.k8sClient.Clientset.CoreV1().RESTClient().Get().
		Namespace(s.namespace).
		Resource("services").
		LabelsSelectorParam(selector).
		Do().
		Get()
	if err != nil {
		return nil, err
	}
	serviceList, ok := obj.(*v1.ServiceList)
	if !ok {
		err := fmt.Errorf("could not convert %v (%T) into ServiceList", obj, obj)
		log.Println(err)
		return nil, err
	}
	if len(serviceList.Items) > 0 {
		service := serviceList.Items[0]
		return &service, nil
	}
	return nil, ErrServiceNotFound
}
