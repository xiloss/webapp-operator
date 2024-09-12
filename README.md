# WEBAPP OPERATOR DEMO

A simple step-by-step guide to start working with kubernetes operators.

For this demonstration, the prerequisite packages and versions are
- golang 1.22
- kubebuilder 4.2.0
- a kubernetes cluster, the current demo is using a 3 nodes local kind, based on version 1.27.3 of kubernetes

## Kubebuilder Setup

First let's setup kubebuilder:

```bashwebapp-operator
# Go to your home directory
cd ~
# Current kubebuilder latest version
KUBEBUILDER_VERSION="4.2.0"
# Kubebuilder github release endpoint
KUBEBUILDER_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}"
# Download the version of kubebuilder 4.2.0 for this demo
curl -L ${KUBEBUILDER_URL}/kubebuilder_linux_amd64 -o kubebuilder-${KUBEBUILDER_VERSION}
# Make the archive executable
chmod ugo+x kubebuilder
# Move the extracted binary to a permanent location
sudo mv kubebuilder /usr/local/bin/kubebuilder-${KUBEBUILDER_VERSION}
# ensure you copy the executable versioned, in order to persist various versions
sudo ln -sfn /usr/local/bin/kubebuilder-${KUBEBUILDER_VERSION} /usr/local/bin/kubebuilder
```

## Kubebuilder Initialization

Everything is now ready to initialize the kubebuilder project.
The first thing to do is to assign the domain name to our API extension.
This could be whatever we want to use as the main domain name for our components.
Please note that it is not necessary an existing domain.
If you pre-scaffolded the project, please be sure to remove an eventually existing main.go file.

NOTE: existing go.mod files will be overwritten.

```bash
# Initialize Kubebuilder project
kubebuilder init --domain kubebuilder.demo
```

## Creating the first API

### Kubebuilder API Creation

We have initialized kubebuilder with a domain name for our API extensions, so we are going to create the API group.
This will be initially at version v1alpha1, as required by the kubernetes standards, and it will be in the group "apps".

```bash
# Create Kubebuilder api for kind webapp
# using yes we will not be asked for confirmation, creating the resources in unattended mode
yes | kubebuilder create api --group apps --version v1alpha1 --kind WebApp
```

## Starting to code

### The goal

In this initial approach to writing a kubernetes operator, the goal will be to create an api extension capable of
creating a WebApp new kind of application, consisting of an initial definition based on replicas, image and port.
Once created, it will take care of provisioning a deployment and an ingress, with the exposed port for the application
to run. Our new kind will be namespaced.

### Defining the WebApp resource

Now we have all the resources and structure created by kubebuilder, the project is looking already scaffolded and the
files are ready to be customized for our needs.
To achieve the first change and update the generated archives, we will open the generated 
[webapp_types.go](api/v1alpha1/webapp_types.go) file and define the WebApp structure, adding the following 
fields to the Spec:
- Replicas: Number of replicas for the deployment.
- Image: Docker image for the web application.
- Port: Container port for the application.

### Implementing the WebApp Controller logic

Now that a basic set of resources has been mapped, we need to proceed with the controller logic.
Let's open the [webapp_controller.go](internal/controller/webapp_controller.go) file and implement the reconciliation 
logic to manage the Deployment and Service for the WebApp.

#### The WebApp Reconcile function

The current code changes has been commented to describe the workflow.
First the WebApp instance, if created, needs to be fetched by its namespace.
It's evident how if the instance object is not defined, the reconciler returns an error.
If it's not present, then it re-queues the request.

##### Adding finalizers

The main scope to have finalizers on resources is to control the process when a resource is deleted. In our case we will
just focus on logging, but it can be used for cleanup and final validation purposes.
After fetching the webapp instance in the reconciler, a full check on object deletion is added, in order to capture the 
related details using the finalizer.
Summarizing the main reasons to care about finalizer:

1. **Finalizer Constant:** Define webAppFinalizer constant to use for the finalizer.
2. **Check Deletion Timestamp:** In the Reconcile method, check if the WebApp instance is being deleted by checking its deletion timestamp.
3. **Handle Finalizer:**
   - If the resource is being deleted and the finalizer is present, log the deletion, perform any necessary cleanup, and then remove the finalizer.
   - If the resource is not being deleted and doesn't have the finalizer, add it and update the resource.
4. **Log Deletion:** Log the deletion of the WebApp resource inside the finalizer handling block.

##### The Deployment

After the main loop, we start defining the Deployment.

Now that we defined the deployment, taking the kubernetes api apps deployment resource, we set the ownership of this 
resource and controller to our WebApp.

At this point the check for eventual existing deployments with the same name, and if the requirements are satisfied, the
creation of the resource, with a log, otherwise the exception is returned.

Since the replicas specified in the WebApp definition must be ensured on the target deployment, a next check is set to 
have the required replicas set.

##### The Service

Time to go for the service definition now, as for the deployment, the resource is mapped to the WebApp name and the 
related namespace. Furthermore, for this we also add the network port.
In this simplified initial version of the controller logic, the protocol is limited to be TCP, since a web application 
should be run to expose an HTTP port.

Next the reference for the controller is marking the ownership of WebApp for the service mapped.

Then with a logic similar to the deployment, the eventual existence of the service is checked and finally the resource 
is created and logged. The exception case of an already existing service is also managed.

##### The SetupWithManager update

The last steps to implement the controller logic is to include the specification of the ownership to the controller 
manager. To achieve that, it's just a matter of including which resources needs to be checked during the loop for the 
WebApp resource.

## Updating the manifests

Since the kubebuilder provided makefile is already equipped with most of the tools and targets to create and maintain 
the operator's resources, to build or regenerate the CRD manifests file it's just a matter of running a simple command:

```bash
# to create or update the crd manifest files
make manifests
```

Having a context set on the command line, pointing to the desired kubernetes cluster, to apply the manifests in both 
cases of creation for the first time or performing an update of the files, the command is straightforward:

```bash
# to install or apply to existing crd manifest files on the context cluster
make install
```

The `make install` command can also be run manually, using the `kustomize` binary available in the current `bin` folder.
To install the custom resource definitions for this operator, run the next command from the root directory of the 
project:

```bash
bin/kustomize build config/crd | kubectl apply -f -
```

## Running locally

Before packaging the operator in a container, we can test it locally, running it from the current folder. This can be 
really helpful for debugging the behavior at runtime, without having to experiment with remote debugging strategies.

```bash
# to run the operator from the local source folder
make run
```

Once started, the operator will output the log information on the local terminal stdout.

**NOTE:** *if the make install has not been run, or generally, if the CRD is not installed on the target cluster, the 
reconciliation loop will just output a missing kind for some time. By default, there is a health probe checking for the 
operator. After failing to complete, the manager process will be shut down.*

## The first sample CRD

The samples are initiallmay created by the kubebuilder scaffolding strategy. I would live the original sample as it is, 
right now it is available in the file [apps_v1alpha1_webapp.yaml](config/samples/apps_v1alpha1_webapp.yaml)
We can create a new file, with the following content:

```yaml
apiVersion: apps.kubebuilder.demo/v1alpha1
kind: WebApp
metadata:
  name: webapp-sample
spec:
  replicas: 2
  image: nginx:latest
  port: 80
```

With this self-descriptive definition, when we create the webapp-sample resource, our operator will provision a 
deployment, with the same name and the image specified in the `spec.image` field.

Then it will ensure the deployment is exposed as a service, using TCP/80 port, as specified in the `spec.port` field.

The replicas will be two for our sample case.

Once the new manifest is saved, that should be added to the kustomization.yaml file in the same folder.

## This is just the beginning

This example demonstrates how to create a simple yet slightly complex Kubernetes operator using Kubebuilder. 
The WebApp resource is used to manage a web application, including a Deployment and Service. 
You can expand this operator by adding more fields to the WebApp resource, implementing more complex reconciliation 
logic, or adding other features like webhooks and metrics.

## Build and Deploy the operator

### Building docker container locally

In the makefile there are several options to build the container image, including the automatic build and push to a 
remote docker registry.
For the current kind example, it will be sufficient to build the container locally, using

```bash
make docker
```

The default will create a local image called `controller:latest`
This image needs to be uploaded to the kind cluster if the target repository will not be created on docker.io registry.
If the target cluster has to retrieve the built image from a remote registry, then a manual re-tagging will be required.
To upload the container image to the local kind cluster, run the following command:

```bash
kind load docker-image docker.io/library/controller:latest --name=<kind-cluster-name>
```

### Customize the Operator Deployment Details

The command to run a deployment on a target cluster is

```bash
make deploy
```

Before executing it, be sure to edit the defaults, opening [kustomization.yaml](config/default/kustomization.yaml)
Changing the name and target namespace to where to deploy the operator could be something useful to override defaults.
For the current case, a preferred choice for the target namespace will be `webapp-operator`, and for the namePrefix 
`webapp-operator-`

## Adding instrumentation for metrics to the webapp-operator

To instrument the controller with metrics for Prometheus, we can use the controller-runtime's built-in support for 
metrics and Prometheus. We will add custom metrics, tracking the creation of WebApp, Deployment, and Service resources.

Next steps will complete the required instrumentation:

### Adding Prometheus dependencies

First it's necessary to add the required dependencies to our current project.
The following command will complete the task:

```bash
go get github.com/prometheus/client_golang/prometheus
go get sigs.k8s.io/controller-runtime/pkg/metrics
go get github.com/prometheus/client_golang/prometheus/promhttp@v1.20.3
```

Now we will be able to use them in the [webapp_controller.go](internal/controller/webapp_controller.go)

Since we are going to introduce metrics for count the number of webapp resources created, the number of deployments and 
the number of service created, this is generally achieved defining global variables.
To achieve that, the global variables will be included in the [main.go](cmd/main.go) file.
The three metrics will be also included in the init() function.

Once the previous steps are completed, we can start include the updates to the reconciler logic.


