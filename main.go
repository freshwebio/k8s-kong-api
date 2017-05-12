package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/runtime/serializer"
	"k8s.io/client-go/pkg/watch/versioned"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/namsral/flag"

	"github.com/freshwebio/k8s-kong-api/apiplugin"
	"github.com/freshwebio/k8s-kong-api/gatewayapi"
	"github.com/freshwebio/k8s-kong-api/k8sclient"
	"github.com/freshwebio/k8s-kong-api/kong"
)

var (
	kubeconfig           = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	kubeNamespace        = flag.String("namespace", "default", "The namespace to use to watch k8s events in.")
	kongScheme           = flag.String("kongscheme", "http://", "The scheme of the kong admin api, http or https")
	kongHost             = flag.String("konghost", "kong", "The host of the kong admin api")
	kongPort             = flag.String("kongport", "8001", "The port the kong admin api lives on")
	apiLabel             = flag.String("apilabel", "kong.gateway.api", "The name of the label used to identify a kong API that references a GatewayApi resource")
	serviceSelectorLabel = flag.String("sslabel", "service", "The name the label to be used for selecting services in custom k8s resources")
)

func main() {
	flag.Parse()
	var err error
	var cli *k8sclient.Client
	if *kubeconfig == "" {
		// Let's create an in cluster client.
		cli, err = k8sclient.NewInClusterClient()
		if err != nil {
			panic(err.Error())
		}
	} else {
		// If kube config flag is specified lets load our client
		// from config.
		cli, err = k8sclient.NewClient(*kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	}
	// Now let's initialise our kong client.
	kongClient := kong.NewClient(*kongHost, *kongPort, *kongScheme)

	// Now setup our api plugin scheme.
	groupVersion := unversioned.GroupVersion{
		Group:   "k8s.freshweb.io",
		Version: "v1",
	}
	schemeBuilder := runtime.NewSchemeBuilder(
		func(scheme *runtime.Scheme) error {
			scheme.AddKnownTypes(
				groupVersion,
				&apiplugin.ApiPlugin{},
				&apiplugin.ApiPluginList{},
				&gatewayapi.GatewayApi{},
				&gatewayapi.GatewayApiList{},
				&api.ListOptions{},
				&api.DeleteOptions{},
			)
			versioned.AddToGroupVersion(scheme, groupVersion)
			return nil
		})
	if err = schemeBuilder.AddToScheme(api.Scheme); err != nil {
		log.Fatalf("error setting up apiplugin and gatewayapi scheme: %v", err)
	}
	var k8sRestConfig *rest.Config
	if *kubeconfig == "" {
		k8sRestConfig, err = rest.InClusterConfig()
	} else {
		k8sRestConfig, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	}
	if err != nil {
		log.Fatalf("Error trying to configure k8s REST client: %v", err)
	}
	tprConfig := *k8sRestConfig
	tprConfig.GroupVersion = &groupVersion
	tprConfig.APIPath = "/apis"
	tprConfig.ContentType = runtime.ContentTypeJSON
	tprConfig.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}

	k8sRestClient, err := rest.RESTClientFor(&tprConfig)
	if err != nil {
		log.Fatalf("error creating our general k8s client for the apiplugin service: %v", err)
	}

	// Instantiate the GatewayApi manager.
	gatewayApiService := gatewayapi.NewService(k8sRestClient, cli, kongClient, *kubeNamespace, *apiLabel, *serviceSelectorLabel)

	// Now instantiate our ApiPlugin manager.
	apipluginService := apiplugin.NewService(k8sRestClient, cli, kongClient, *kubeNamespace, *apiLabel, *serviceSelectorLabel)

	// Asynchronously start watching and refreshing apiplugins and kong API objects
	wg := sync.WaitGroup{}
	doneChan := make(chan struct{})
	wg.Add(1)
	go gatewayApiService.Start(doneChan, &wg)

	wg.Add(1)
	go apipluginService.Start(doneChan, &wg)

	// Listen for shutdown signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	log.Println("Shutdown signal received, exiting...")
	close(doneChan)
	wg.Wait()
	return
}
