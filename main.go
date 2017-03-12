package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/freshwebio/k8s-kong-api/k8sclient"
	"github.com/freshwebio/k8s-kong-api/kong"
	"github.com/freshwebio/k8s-kong-api/service"
)

var (
	kubeconfig    = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	kubeNamespace = flag.String("namespace", "default", "The namespace to use to watch k8s events in.")
	kongHost      = flag.String("konghost", "kong", "The host of the kong admin api")
	kongPort      = flag.String("kongport", "8001", "The port the kong admin api lives on")
	routesLabel   = flag.String("routeslabel", "kong.api.routes", "The name of the label to identify kong services")
	portLabel     = flag.String("portlabel", "kong.api.port", "The name of the label that provides the port to be used for a service")
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
	kongClient := kong.NewClient(*kongHost, *kongPort)
	// Now let's instantiate and start our service which deals
	// with listening to kubernetes events and propogating events
	// to kong accordingly.
	srv := service.NewService(kongClient, cli, *kubeNamespace, *routesLabel, *portLabel)
	srv.Start()

	// Handle SIGINT and SIGTERM.
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	// Stop the service gracefully.
	srv.Stop()
}
