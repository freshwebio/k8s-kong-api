package apiplugin

import (
	"fmt"
	"log"
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

// Service deals with monitoring and responding
// to events on api plugin resources in k8s
// and updating the Kong representations accordingly.
type Service struct {
	k8sRestClient              *rest.RESTClient
	k8sClient                  *k8sclient.Client
	apiLabel                   string
	pluginServiceSelectorLabel string
	namespace                  string
	kongClient                 *kong.Client
}

// NewService creates a new instance of the ApiPlugin service.
func NewService(k8sRestClient *rest.RESTClient, k8sClient *k8sclient.Client, kong *kong.Client, namespace string,
	apiLabel string, pluginServiceSelectorLabel string) *Service {
	return &Service{k8sRestClient: k8sRestClient, k8sClient: k8sClient, kongClient: kong, namespace: namespace,
		apiLabel: apiLabel, pluginServiceSelectorLabel: pluginServiceSelectorLabel}
}

// Start deals with beginning the monitoring process which deals with monitoring
// events from k8s apiplugin resources as well as services to propogate changes to kong.
// This method should be called asynchronously in it's own goroutine.
func (s *Service) Start(doneChan <-chan struct{}, wg *sync.WaitGroup) {
	log.Println("Starting the plugin watcher service")
	// Let's monitor our service and plugin events.
	selector := labels.NewSelector()
	req, err := labels.NewRequirement(s.apiLabel, selection.Exists, []string{})
	if err != nil {
		log.Fatal(err)
	}
	selector = selector.Add(*req)
	serviceEvents := s.monitorServiceEvents(s.namespace, selector, doneChan)
	pluginEvents := s.monitorPluginEvents(s.namespace, labels.Nothing(), doneChan)
	for {
		select {
		case event := <-pluginEvents:
			err := s.processPluginEvent(event)
			if err != nil {
				log.Printf("Error while processing plugin event: %v", err)
			}
		case event := <-serviceEvents:
			err := s.processServiceEvent(event)
			if err != nil {
				log.Printf("Error while processing service event: %v", err)
			}
		case <-doneChan:
			wg.Done()
			log.Println("Stopped api plugin event watcher.")
		}
	}
}

// Handles processing the service events we are interested in for the sake
// of our plugins.
func (s *Service) processServiceEvent(e k8stypes.ServiceEvent) error {
	switch e.Type {
	case "ADDED", "MODIFIED":
		err := s.attachServicePlugins(e.Object)
		if err != nil {
			return err
		}
	}
	return nil
}

// Attaches plugins to a service if they aren't already attached.
func (s *Service) attachServicePlugins(v1s v1.Service) error {
	// First let's get a list of existing plugins with the provided service selector.
	selector := labels.NewSelector()
	req, err := labels.NewRequirement(s.pluginServiceSelectorLabel, selection.Equals, []string{v1s.GetName()})
	if err != nil {
		return err
	}
	selector = selector.Add(*req)
	source := k8sclient.NewListWatchFromClient(s.k8sRestClient, "apiplugins", s.namespace, selector)
	store, _ := cache.NewInformer(source, &ApiPlugin{}, 0, cache.ResourceEventHandlerFuncs{})
	for _, obj := range store.List() {
		plugin, ok := obj.(*ApiPlugin)
		if !ok {
			return fmt.Errorf("could not convert %v (%T) into ApiPlugin", obj, obj)
		}
		// The APIs are saved with the same name as the service.
		kongPlugin := &kong.Plugin{
			Name:   plugin.Spec.Name,
			Config: plugin.Spec.Config,
		}
		hasPlugin, err := s.kongClient.APIHasPlugin(v1s.GetName(), kongPlugin.Name)
		if err != nil {
			return err
		}
		if !hasPlugin {
			err := s.kongClient.AddPlugin(v1s.GetName(), kongPlugin)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) processPluginEvent(e Event) error {
	switch e.Type {
	case "ADDED":
		err := s.attachPluginToService(e.Object)
		if err != nil {
			return err
		}
	case "MODIFIED":
		err := s.updatePlugin(e.Object)
		if err != nil {
			return err
		}
	case "DELETED":
		err := s.detachPluginFromService(e.Object)
		if err != nil {
			return err
		}
	}
	return nil
}

// Simply deals with attaching a plugin to a service given the service
// has a valid API object in kong and a plugin of the same type doesn't already
// exist for the service.
func (s *Service) attachPluginToService(p ApiPlugin) error {
	// First of all attempt to retrieve the service provided
	// by the plugin's selector to make sure it exists.
	if serviceName, exists := p.Spec.Selector[s.pluginServiceSelectorLabel]; exists {
		_, err := s.kongClient.GetAPI(serviceName)
		if err != nil {
			return err
		}
		// Now let's attach our plugin.
		kongPlugin := &kong.Plugin{
			Name:   p.Spec.Name,
			Config: p.Spec.Config,
		}
		// For the case where one might define duplicate plugins for a single service
		// let's ensure the service doesn't already have the provided plugin.
		hasPlugin, err := s.kongClient.APIHasPlugin(serviceName, kongPlugin.Name)
		if err != nil {
			return err
		}
		if !hasPlugin {
			err := s.kongClient.AddPlugin(serviceName, kongPlugin)
			if err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("The service selector (%v) was not provided in the plugin",
			s.pluginServiceSelectorLabel)
	}
	return nil
}

// Deals with updating a plugin for the given service selector
// if both the service exists and the plugin to be updated is already attached to the service.
func (s *Service) updatePlugin(p ApiPlugin) error {
	if serviceName, exists := p.Spec.Selector[s.pluginServiceSelectorLabel]; exists {
		_, err := s.kongClient.GetAPI(serviceName)
		if err != nil {
			return err
		}
		// Now let's update our plugin.
		kongPlugin := &kong.Plugin{
			Name:   p.Spec.Name,
			Config: p.Spec.Config,
		}
		// Ensure the plugin exists for the provided service.
		hasPlugin, err := s.kongClient.APIHasPlugin(serviceName, kongPlugin.Name)
		if err != nil {
			return err
		}
		if hasPlugin {
			err := s.kongClient.UpdatePlugin(serviceName, kongPlugin)
			if err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("The service selector (%v) was not provided in the plugin",
			s.pluginServiceSelectorLabel)
	}
	return nil
}

// Deals with removing a plugin from an API service in kong.
func (s *Service) detachPluginFromService(p ApiPlugin) error {
	if serviceName, exists := p.Spec.Selector[s.pluginServiceSelectorLabel]; exists {
		_, err := s.kongClient.GetAPI(serviceName)
		if err != nil {
			return err
		}
		// Ensure the plugin exists for the provided service.
		hasPlugin, err := s.kongClient.APIHasPlugin(serviceName, p.Spec.Name)
		if err != nil {
			return err
		}
		if hasPlugin {
			err := s.kongClient.RemovePlugin(serviceName, p.Spec.Name)
			if err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("The service selector (%v) was not provided in the plugin",
			s.pluginServiceSelectorLabel)
	}
	return nil
}

// Writes service events from k8s to a new channel to be consumed.
func (s *Service) monitorServiceEvents(namespace string, selector labels.Selector, done <-chan struct{}) <-chan k8stypes.ServiceEvent {
	events := make(chan k8stypes.ServiceEvent)
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
	source := k8sclient.NewListWatchFromClient(s.k8sClient.Clientset.CoreV1().RESTClient(), "services", namespace, selector)
	store, ctrl := cache.NewInformer(source, &v1.Service{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			eventCallback(watch.Added, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			eventCallback(watch.Modified, new)
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

	return events
}

// Handles watching events occuring for our custom plugin resource.
// All ApiPlugin resources in the give namespace and selector combination are watched in this case.
func (s *Service) monitorPluginEvents(namespace string, selector labels.Selector, done <-chan struct{}) <-chan Event {
	events := make(chan Event)
	eventCallback := func(evType watch.EventType, obj interface{}) {
		plugin, ok := obj.(*ApiPlugin)
		if !ok {
			log.Printf("could not convert %v (%T) into ApiPlugin", obj, obj)
			return
		}
		events <- Event{
			Type:   string(evType),
			Object: *plugin,
		}
	}
	source := k8sclient.NewListWatchFromClient(s.k8sRestClient, "apiplugins", namespace, selector)
	store, ctrl := cache.NewInformer(source, &ApiPlugin{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			eventCallback(watch.Added, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			eventCallback(watch.Modified, new)
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

	return events
}
