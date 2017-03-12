package k8sclient

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client provides the type to interact
// with Kubernetes.
type Client struct {
	clientset *kubernetes.Clientset
}

// NewInClusterClient deals with creating a new
// instance of a Kubernetes client.
func NewInClusterClient() (*Client, error) {
	// Let's create an in cluster config.
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{clientset: clientset}, nil
}

// NewClient deals with creating
// a new kubernetes client instance from provided configuration.
func NewClient(configFile string) (*Client, error) {
	// Create our configuration from the provided file.
	config, err := clientcmd.BuildConfigFromFlags("", configFile)
	if err != nil {
		return nil, err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{clientset: clientset}, nil
}

// WatchServices deals with watching services for the provided namespace.
// To note: Only services with the defined label are watched in this stream.
func (cli *Client) WatchServices(namespace string, routesLabel string) (watch.Interface, error) {
	// We only care about pods which are created to be upstream
	// API services so filter to only those with the defined
	// label.
	options := v1.ListOptions{
		LabelSelector: routesLabel,
	}
	return cli.clientset.Services(namespace).Watch(options)
}

// ListServices retrieves a list of services with the defined label.
func (cli *Client) ListServices(namespace string, routesLabel string) (*v1.ServiceList, error) {
	options := v1.ListOptions{
		LabelSelector: routesLabel,
	}
	return cli.clientset.Services(namespace).List(options)
}
