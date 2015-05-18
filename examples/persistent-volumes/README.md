# How To Use Persistent Volumes

The purpose of this guide is to help you become familiar with Kubernetes Persistent Volumes.  By the end of the guide, we'll have
nginx serving content from your persistent volume.

This guide assumes knowledge of Kubernetes fundamentals and that you have a cluster up and running.

## Provisioning

A PersistentVolume in Kubernetes represents a real piece of underlying storage capacity in the infrastructure.  Cluster administrators
must first create storage (create their GCE disks, export their NFS shares, etc.) in order for Kubernetes to mount it.

PVs are intended for "network volumes" like GCE Persistent Disks, NFS shares, and AWS ElasticBlockStore volumes.  ```HostPath``` was included
for ease of development and testing.  You'll create a local ```HostPath``` for this example.

> IMPORTANT! For ```HostPath``` to work, you will need to run a single node cluster.  Kubernetes does not
support local storage on the host at this time.  There is no guarantee your pod ends up on the correct node where the ```HostPath``` resides.

  
```

// this will be nginx's webroot
mkdir /tmp/data01
echo 'I love Kubernetes storage!' > /tmp/data01/index.html

```

PVs are created by posting them to the API server.

```

cluster/kubectl.sh create -f examples/persistent-volumes/volumes/local-01.yaml
cluster/kubectl.sh get pv

NAME        LABELS       CAPACITY            ACCESSMODES         STATUS              CLAIM
pv0001      map[]        10737418240         RWO                 Available                            

```

## Requesting storage

Users of Kubernetes request persistent storage for their pods.  They don't know how the underlying cluster is provisioned.
They just know they can rely on their claim to storage and can manage its lifecycle independently from the many pods that may use it.  

Claims must be created in the same namespace as the pods that use them.

```

cluster/kubectl.sh create -f examples/persistent-volumes/claims/claim-01.yaml
cluster/kubectl.sh get pvc

NAME                LABELS              STATUS              VOLUME
myclaim-1           map[]                                   
           
           
# A background process will attempt to match this claim to a volume.
# The eventual state of your claim will look something like this:

cluster/kubectl.sh get pvc

NAME        LABELS    STATUS    VOLUME                                                          
myclaim-1   map[]     Bound     f5c3a89a-e50a-11e4-972f-80e6500a981e    


cluster/kubectl.sh get pv

NAME                LABELS              CAPACITY            ACCESSMODES         STATUS    CLAIM
pv0001              map[]               10737418240         RWO                 Bound     myclaim-1 / 6bef4c40-e50b-11e4-972f-80e6500a981e          

```

## Using your claim as a volume

Claims are used as volumes in pods.  Kubernetes uses the claim to look up its bound PV.  The PV is then exposed to the pod.

```

cluster/kubectl.sh create -f examples/persistent-volumes/simpletest/pod.yaml

cluster/kubectl.sh get pods

POD       IP           CONTAINER(S)   IMAGE(S)   HOST                  LABELS    STATUS    CREATED
mypod     172.17.0.2   myfrontend     nginx      127.0.0.1/127.0.0.1   <none>    Running   12 minutes


cluster/kubectl.sh create -f examples/persistent-volumes/simpletest/service.json
cluster/kubectl.sh get services

NAME              LABELS                                    SELECTOR            IP           PORT(S)
frontendservice   <none>                                    name=frontendhttp   10.0.0.241   3000/TCP
kubernetes        component=apiserver,provider=kubernetes   <none>              10.0.0.2     443/TCP
kubernetes-ro     component=apiserver,provider=kubernetes   <none>              10.0.0.1     80/TCP


```

## Next steps

You should be able to query your service endpoint and see what content nginx is serving.  A "forbidden" error might mean you 
need to disable SELinux (setenforce 0).

```

curl 10.0.0.241:3000
I love Kubernetes storage!

```

Hopefully this simple guide is enough to get you started with PersistentVolumes.  If you have any questions, join
```#google-containers``` on IRC and ask!

Enjoy!
