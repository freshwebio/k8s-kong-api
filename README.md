# k8s-kong-api
Application that listens to Kubernetes events to dynamically create kong APIs, upstreams and targets.

## Building application (Standalone)
To build this application standalone simply run `go build` from the root directory after ensuring
all the dependencies are current by ensuring you have the godep tool installed and running `godep restore`.

## Building application (Docker)
To build for docker you must firstly ensure all the dependencies are installed using `godep restore`.
Then run `CGO_ENABLED=0 GOOS=linux go build -a -installsuffix .` to get a binary fully packaged with
all dependencies to run on the empty scratch base.
Now you can build the docker image and run it in docker as an out of cluster k8s client or in k8s
for in cluster usage.

## Running the application
You can provide configuration in order to run the application successfully in three different ways,
below is a table of the config used and the default values:

| Type   | Flag                      | Environment                | File                     | Default value      |
| ------ | :------------------------ |:-------------------------- |:------------------------ | :----------------- |
| string | -kubeconfig ./config      | KUBECONFIG="./config"      | kubeconfig ./config      | ""                 |
| string | -namespace myclstr        | NAMESPACE="myclstr"        | namespace myclstr        | "default"          |
| string | -konghost kong-api        | KONGHOST="kong-api"        | konghost kong-api        | "kong"             |
| string | -kongport 8001            | KONGPORT="8001"            | kongport 8001            | "8001"             |
| string | -routesLabel myapi.routes | ROUTESLABEL="myapi.routes" | routesLabel myapi.routes | "kong.api.routes"  |
| string | -portLabel myapi.port     | PORTLABEL="myapi.port"     | portLabel myapi.port     | "kong.api.port"    |

To provide a configuration file run ./k8s-kong-api -config myconf.conf,
To run with flags simply provide the flags and for environment variables, make sure the env vars are set
and then simply run the binary.
The best way to run the application in cluster would be to provide environment variables to the k8s pod container
which encapsulates the application.
