package main

import (
	"fmt"
	"time"

	_ "github.com/lib/pq"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
    apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	platformworkflowv1alpha1 "github.com/cloud-ark/kubeplus/platform-operator/pkg/apis/workflowcontroller/v1alpha1"
	clientset "github.com/cloud-ark/kubeplus/platform-operator/pkg/client/clientset/versioned"
	platformstackscheme "github.com/cloud-ark/kubeplus/platform-operator/pkg/client/clientset/versioned/scheme"
	informers "github.com/cloud-ark/kubeplus/platform-operator/pkg/client/informers/externalversions"
	listers "github.com/cloud-ark/kubeplus/platform-operator/pkg/client/listers/workflowcontroller/v1alpha1"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"

	"k8s.io/client-go/rest"
)

const controllerAgentName = "platformstack-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when PlatformStack is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Foo fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by PlatformStack"
	// MessageResourceSynced is the message used for an Event fired when a Foo
	// is synced successfully
	MessageResourceSynced = "PlatformStack synced successfully"
)

// Controller is the controller implementation for Foo resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	platformStackclientset clientset.Interface

	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced
	platformStacksLister        listers.ResourceCompositionLister
	platformStacksSynced        cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewPlatformController(
	kubeclientset kubernetes.Interface,
	platformStackclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	platformstackInformerFactory informers.SharedInformerFactory) *Controller {

	// obtain references to shared index informers for the Deployment and PlatformStack
	// types.
	deploymentInformer := kubeInformerFactory.Apps().V1().Deployments()
	platformStackInformer := platformstackInformerFactory.Workflows().V1alpha1().ResourceCompositions()

	// Create event broadcaster
	// Add platformstack-controller types to the default Kubernetes Scheme so Events can be
	// logged for platformstack-controller types.
	platformstackscheme.AddToScheme(scheme.Scheme)
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:     kubeclientset,
		platformStackclientset:   platformStackclientset,
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		platformStacksLister:        platformStackInformer.Lister(),
		platformStacksSynced:        platformStackInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "PlatformStacks"),
		recorder:          recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when Foo resources change
	platformStackInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueFoo,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*platformworkflowv1alpha1.ResourceComposition)
			oldDepl := old.(*platformworkflowv1alpha1.ResourceComposition)
			//fmt.Println("New Version:%s", newDepl.ResourceVersion)
			//fmt.Println("Old Version:%s", oldDepl.ResourceVersion)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			} else {
				controller.enqueueFoo(new)
			}
		},
		/*
		DeleteFunc: func(obj interface{}) {
		        _, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
			   controller.deleteFoo(obj)
			}
		},*/
	})
	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting PlatformStack controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.deploymentsSynced, c.platformStacksSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch two workers to process Foo resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// enqueueFoo takes a Foo resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Foo.
func (c *Controller) enqueueFoo(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Foo resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Foo resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		glog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	glog.V(4).Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Foo, we should not do anything more
		// with it.
		if ownerRef.Kind != "Foo" {
			return
		}

		foo, err := c.platformStacksLister.ResourceCompositions(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			glog.V(4).Infof("ignoring orphaned object '%s' of foo '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueFoo(foo)
		return
	}
}


func (c *Controller) deleteFoo(obj interface{}) {

	fmt.Println("Inside delete Foo")

	var err error
	if _, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
	   panic(err)
	}
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Foo resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Foo resource with this namespace/name
	foo, err := c.platformStacksLister.ResourceCompositions(namespace).Get(name)
	if err != nil {
		// The Foo resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("platformStack '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	/*labelSelector := foo.Spec.LabelSelector

	fmt.Printf("LabelSelector:%v\n", labelSelector)

	stackElements := foo.Spec.StackElements

	fmt.Printf("Label:%s\n", foo.Spec.LabelSelector)
	fmt.Printf("StackElements:%s\n", foo.Spec.StackElements)

	fmt.Printf("LabelSelector:%s\n", labelSelector)
	for _, stackElement := range stackElements {
		kind := stackElement.Kind
		instance := stackElement.Name
		namespace := "default"
		if stackElement.Namespace != "" {
			namespace = stackElement.Namespace
		}

		fmt.Printf("Kind:%s, Instance:%s, Namespace:%s\n", kind, instance, namespace)
		dependsOn := stackElement.DependsOn
		if dependsOn != nil {
			for _, dependentInstance := range dependsOn {
				fmt.Printf("    DependsOn:%s\n", dependentInstance)
			}
		}
	}
	for key, value := range labelSelector {
		fmt.Printf("Key:%s, Value:%s\n", key, value)
	}*/

	//customAPIs := foo.Spec.NewResource.Resource.Kind
	//fmt.Printf("New APIs:%s\n", customAPIs)
	//for _, customAPI := range customAPIs {
	fmt.Printf("ABC\n")
	fmt.Printf("%v\n", foo.Spec)
	newRes := foo.Spec.NewResource
	fmt.Printf("DEF\n")
	fmt.Printf("%v\n", newRes)
	res := newRes.Resource
	fmt.Printf("GHI\n")
	fmt.Printf("%v\n", res)
	kind := foo.Spec.NewResource.Resource.Kind
	group := foo.Spec.NewResource.Resource.Group
	version := foo.Spec.NewResource.Resource.Version
	plural := foo.Spec.NewResource.Resource.Plural
	chartURL := foo.Spec.NewResource.ChartURL
	chartName := foo.Spec.NewResource.ChartName
	fmt.Printf("Kind:%s, Version:%s Group:%s, Plural:%s\n", kind, version, group, plural)
	fmt.Printf("ChartURL:%s, ChartName:%s\n", chartURL, chartName)
		// Check if CRD is present or not. Create it only if it is not present.
	createCRD(kind, version, group, plural)
	//}
	c.recorder.Event(foo, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func createCRD(kind, version, group, plural string) error {
	fmt.Printf("Inside createCRD\n")
	cfg, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	crdClient, _ := apiextensionsclientset.NewForConfig(cfg)

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: plural + "." + group,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group: group,
			Version: version,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: plural,
				Kind: kind,
			},
		},
	}

	_, err1 := crdClient.CustomResourceDefinitions().Create(crd)
	if err1 != nil {
		panic(err1.Error())
	}

	crdList, err := crdClient.CustomResourceDefinitions().List(metav1.ListOptions{})
	if err != nil {
		fmt.Errorf("Error:%s\n", err)
		return err
	}
	for _, crd := range crdList.Items {
		crdName := crd.ObjectMeta.Name
		crdObj, err := crdClient.CustomResourceDefinitions().Get(crdName, metav1.GetOptions{})
		if err != nil {
			fmt.Errorf("Error:%s\n", err)
			return err
		}
		group := crdObj.Spec.Group
		version := crdObj.Spec.Version
		endpoint := "apis/" + group + "/" + version
		kind := crdObj.Spec.Names.Kind
		plural := crdObj.Spec.Names.Plural
		fmt.Printf("Kind:%s, Group:%s, Version:%s, Endpoint:%s, Plural:%s\n",kind, group, version, endpoint, plural)
	}
	return nil
}

func int32Ptr(i int32) *int32 { return &i }
