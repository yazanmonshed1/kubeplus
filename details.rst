=======================
KubePlus tooling
=======================

KubePlus tooling simplifies building Kubernetes-native workflow automation using Kubernetes Custom APIs/ Resources by extending the Kubernetes resource graph and maintaining all implicit and explicit relationships of Custom Resources created through labels, annotations, spec properties or sub-resources. This Custom Resource relationship graph is then used for improved visibility, monitoring and debuggability of workflows. KubePlus tooling additionally allows you to define workflow level Kubernetes resource dependencies and allows applying security or robustness policies to all the workflow resources together. 

This tool is being developed as a part of our  `Platform as Code practice`_.

.. _Platform as Code practice: https://cloudark.io/platform-as-code


--------
Details
--------

Kubernetes Custom Resources and Custom Controllers, popularly known as `Operators`_, extend Kubernetes to run third-party softwares directly on Kubernetes. Teams adopting Kubernetes assemble required Operators of platform softwares such as databases, security, backup etc. to build the required application platforms. KubePlus tooling simplifies creation of Kubernertes-native platform workflows leveraging Custom Resources available through the various Operators.

.. image:: ./docs/Kubernetes-native-stack-with-KubePlus.jpg
   :height: 200px
   :width: 200px
   :align: center

The main benefit of using KubePlus to DevOps/Platform engineers are:

- easily discover static and runtime information about Custom Resources available in their cluster.
- aggregate Custom and built-in resources to build secure and robust platform workflows.

KubePlus provides discovery commands, binding functions, and an orchestration mechanism to enable DevOps/Platform engineers to define Kubernetes-native platform workflows using Kubernetes Custom and built-in resources.

.. You can think of KubePlus API Add-on as a tool that enables AWS CloudFormation/Terraform like experience when working with Kubernetes Custom Resources.

.. _Operators: https://coreos.com/operators/

.. _as Code: https://cloudark.io/platform-as-code


.. KubePlus API add-on Components
.. -------------------------------
   .. .. image:: ./docs/KubePlus-API-Addon-Components.png
..   :height: 100px
..   :width: 200 px
..   :align: center


KubePlus Components
----------------------
KubePlus tooling is made up of - CRD Annotations, client-side kubectl plugins, and server-side components (binding functions and PlatformWorkflow Operator).


CRD annotations
-----------------

In order to build and maintain Custom Resource relationship graph, KubePlus expects CRD packages to be updated with annotations described below. 

.. code-block:: bash

   resource/usage

The 'usage' annotation is used to define usage information for a Custom Resource.
The value of 'usage' annotation is the name of the ConfigMap that stores the usage information.

.. code-block:: bash

   resource/composition

The 'composition' annotation is used to define Kubernetes's built-in resources that are created as part of instantiating a Custom Resource instance.


.. code-block:: bash

   resource/annotation-relationship
   resource/label-relationship
   resource/specproperty-relationship

The relationship annotations are used to declare annotation / label / spec-property based relationships that instances of this Custom Resource can have with other Resources.  

Above annotations need to be defined on the Custom Resource Definition (CRD) YAMLs of Operators in order to make Custom Resources discoverable and usable by DevOps/Platform engineers.

As an example, annotations on Moodle Custom Resource Definition (CRD) are shown below:

.. code-block:: yaml

  apiVersion: apiextensions.k8s.io/v1beta1
  kind: CustomResourceDefinition
  metadata:
    name: moodles.moodlecontroller.kubeplus
    annotations:
      resource/composition: Deployment, Service, PersistentVolume, PersistentVolumeClaim, Secret, Ingress
      resource/usage: moodle-operator-usage.usage
      resource/specproperty-relationship: "on:INSTANCE.spec.mySQLServiceName, value:Service.spec.metadata.name"
  spec:
    group: moodlecontroller.kubeplus
    version: v1
    names:
      kind: Moodle
      plural: moodles
    scope: Namespaced

The composition annotation declares the set of Kubernetes resources that are created by the Moodle Operator when instantiating a Moodle Custom Resource instance.
The specproperty relationship defines that an instance of Moodle Custom Resource is connected through it's mySQLServiceName spec attribute to an instance of a Service resource through that resource's name (metadata.name). Below is an example of a Kubernetes platform workflow in which a Moodle Custom Resource instance is bound to a MysqlCluster Custom Resource instance through the Service resource that is created by the MysqlCluster Operator. The specproperty relationship helps discover this relationship as seen below:

.. code-block:: bash

  (venv) Devs-MacBook:kubeplus devdatta$ kubectl connections Moodle moodle1 namespace1
  Level:0 kind:Moodle name:moodle1 Owner:/
  Level:1 kind:Service name:cluster1-mysql-master Owner:MysqlCluster/cluster1
  Level:2 kind:Pod name:cluster1-mysql-0 Owner:MysqlCluster/cluster1
  Level:3 kind:Service name:cluster1-mysql-nodes Owner:MysqlCluster/cluster1
  Level:3 kind:Service name:cluster1-mysql Owner:MysqlCluster/cluster1
  Level:2 kind:Pod name:moodle1-5847c6b69c-mtwg8 Owner:Moodle/moodle1
  Level:3 kind:Service name:moodle1 Owner:Moodle/moodle1

Here are examples of defining the ``resource/label-relationship`` and ``resoure/annotation`` relationship.

.. code-block:: bash

  resource/annotation-relationship: on:Pod, key:k8s.v1.cni.cncf.io/networks, value:INSTANCE.metadata.name

This annotation-relationship annotation is defined on NetworkAttachmentDefinition CRD available from the Multus Operator. It defines that the relationship between a Pod and an instance of NetworkAttachmentDefinition Custom Resource instance is through the ``k8s.v1.cni.cncf.io/networks`` annotation. This annotation needs to be defined on a Pod and the value of the annotation is the name of a NetworkAttachmentDefinition Custom resource instance.

.. code-block:: bash

  resource/specproperty-relationship: "on:INSTANCE.spec.volumeMounts, value:Deployment.spec.containers.volumemounts.mountpath"
  resource/label-relationship: "on:Deployment, value:INSTANCE.spec.selector"

Above annotations are defined on the Restic Custom Resource available from the Stash Operator. Restic Custom Resource needs two things as input. First, the mount path of the Volume that needs to be backed up. Second, the Deployment in which the Volume is mounted needs to be given some label and that label needs to be specified in the Restic Custom Resource's selector.


Client-side kubectl plugins
----------------------------

KubePlus offers following kubectl plugins towards discovery and use of Custom Resources and obtaining insights into Kubernetes-native application.

.. code-block:: bash

   $ kubectl man cr
   $ kubectl composition
   $ kubectl connections
   $ kubectl metrics cr
   $ kubectl metrics service
   $ kubectl metrics account
   $ kubectl metrics helmrelease
   $ kubectl grouplogs cr
   $ kubectl grouplogs service
   $ kubectl grouplogs helmrelease

In order to use these plugins you need to add KubePlus folder to your PATH variable.

.. code-block:: bash

   $ export KUBEPLUS_HOME=<Full path where kubeplus is cloned>
   $ export PATH=$PATH:`pwd`/plugins


Cluster-side add-on 
---------------------

Binding Functions
------------------

Custom Resource relationships can be categorized into two categories. Explicit relationships based on labels/annotations/spec-properties are static and can be hard-coded into Helm charts / YAML files before the deployment. Implicit relationships can not be hard coded pre-deployment and need to be resolved run-time. Example of implicit relationship can be – Restic Custom Resource depends on label on Moodle Custom Resources Deployment sub-resource which gets created only after Moodle resource is created. KubePlus offers additional functions that can be used directly in the YAML definitions to define such implicit dependencies. 

.. code-block:: bash

   1. Fn::ImportValue(<Parameter>)

This function should be used for defining Custom Resource Spec property values that need to be resolved using runtime information. The function resolves specified parameter at runtime using information about various resources running in a cluster and imports that value into the Spec where the function is defined.

Here is how the ``Fn::ImportValue()`` function can be used in a Custom Resource YAML definition.

.. image:: ./docs/mysql-cluster1.png
   :scale: 10%
   :align: left

.. image:: ./docs/moodle1.png
   :scale: 10%
   :align: right

In the above example the name of the ``Service`` object which is child of ``cluster1`` Custom Resource instance and whose name contains the string ``master`` is discovered at runtime and that value is injected as the value of ``mySQLServiceName`` attribute in the ``moodle1`` Custom Resource Spec.

.. code-block:: bash

   2. Fn::AddLabel(label, <Resource>)

This function adds the specified label to the specified resource/sub-resource by resolving the resource name using runtime information in a cluster.


.. code-block:: bash

   3. Fn::AddAnnotation(annotation, <Resource>)

This function adds the specified annotation to the specified resource/sub-resource by resolving the resource name using runtime information in a cluster.


The ``AddLabel`` and ``AddAnnotation`` functions should be defined as annotations on those Custom Resources that
need appropriate labels and/or annotations on other resources in a cluster for their operation.
`Here`_ is an example of using the ``AddLabel`` function with the ``Moodle`` Custom Resource.

.. _Here: https://github.com/cloud-ark/kubeplus/blob/master/examples/kubectl-plugins-and-binding-functions/moodle1.yaml#L6

This example shows adding a label on the Service created by the Mysql Operator to which the Moodle Custom Resource is binding.

Formal grammar of ``ImportValue``, ``AddLabel``, ``AddAnnotation`` functions is available in the `functions doc`_.

.. _functions doc: https://github.com/cloud-ark/kubeplus/blob/master/docs/kubeplus-functions.txt


PlatformWorkflow Operator
--------------------------
Creating workflows requires treating the set of resources representing the workflow as a unit. For this purpose, KubePlus provides a Custom Resource of its own - ResourceComposition. This Custom Resource enables DevOps/Platform engineers to 
register new services in a cluster as new custom APIs.

PlatformWorkflow Operator does not actually deploy any resources defined in a workflow. Resource creation is done by DevOps/Platform engineers as usual using 'kubectl'/'helm'.


Getting started
----------------

Read our `blog post`_ to understand how Kubernetes Custom Resources affect the notion of 'as-Code' systems.

.. _blog post: https://medium.com/@cloudark/kubernetes-and-the-future-of-as-code-systems-b1b2de312742


Install KubePlus:

.. code-block:: bash

   $ git clone https://github.com/cloud-ark/kubeplus.git
   $ cd kubeplus
   $ export KUBEPLUS_HOME=<Full path where kubeplus is cloned>
   $ export PATH=$PATH:`pwd`/plugins
   $ cd scripts
   $ ./deploy-kubeplus.sh

Platform-as-Code examples:

1. `Manual discovery and binding`_

.. _Manual discovery and binding: https://github.com/cloud-ark/kubeplus/blob/master/examples/moodle-with-presslabs/steps.txt


2. `Automatic discovery and binding`_

.. _Automatic discovery and binding: https://github.com/cloud-ark/kubeplus/blob/master/examples/kubectl-plugins-and-binding-functions/steps.txt


Comparison
-----------

Check comparison of KubePlus with other `community tools`_.

.. _community tools: https://github.com/cloud-ark/kubeplus/blob/master/Comparison.md



Bug reports
------------

Follow `contributing guidelines`_ to submit bug reports.

.. _contributing guidelines: https://github.com/cloud-ark/kubeplus/blob/master/Contributing.md



KubePlus in Action
-------------------

1. Kubernetes Community Meeting notes_

.. _notes: https://discuss.kubernetes.io/t/kubernetes-weekly-community-meeting-notes/35/60

2. Kubernetes Community Meeting `slide deck`_

.. _slide deck: https://drive.google.com/open?id=1fzRLBpCLYBZoMPQhKMQDM4KE5xUh6-xU

3. Kubernetes Community Meeting demo_

.. _demo: https://www.youtube.com/watch?v=taOrKGkZpEc&feature=youtu.be



Operator FAQ
-------------

New to Operators? Checkout `Operator FAQ`_.

.. _Operator FAQ: https://github.com/cloud-ark/kubeplus/blob/master/Operator-FAQ.md






