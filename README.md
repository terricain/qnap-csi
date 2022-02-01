# QNAP CSI

This is a very alpha QNAP Kubernetes CSI driver which lets you automatically provision iSCSI volumes on a QNAP NAS.

Its only been tested on a TS-1279U-RP (firmware 4.3.6.1711)

# TODO

* Publish docker image to GHCR
* Add CI
* Publish helm chart to github releases

# How to install

The main Helm values you'll need to install this would be:
```yaml
QNAPSettings:
  URL: "http://somenas:8080/"
  portal: "192.168.0.5:3260"
  credentialsSecretName: "qnap"
  storagePoolID: 1
```
The `portal` value seems to need to be an IP. The credentials secret should reside in whatever namespace you install the 
Helm chart into, and have the fields `username` and `password`. You'll most likely only have 1 storage pool, so 1 should
do here, if you have more, you should be more than capable of picking the right number.

To install the Helm chart: (this assumes you've cloned the repo as the chart isnt hosted yet)
```shell
helm install -n kube-system -f custom-values.yaml qnap-csi ./chart/qnap-csi
```
This will install the chart under the name of `qnap-csi` into the `kube-system` namespace using custom values in a YAML file.

By default, it will create a storage account called `qnap` which you'll want to use in any persistent volume claims.

## Testing

### Persistent volume creation

Create a PVC with the `qnap` storage class.
E.g.:
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: qnap-test-claim
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: qnap
  resources:
    requests:
      storage: 20Gi
```

This should create an iSCSI volume on the NAS, for me, it takes around 11 seconds to become bound. If it doesn't create
a volume, check the controller pod, the logs you'll be interested in are from the `controller-server` and `csi-provisioner`
containers.

### Volume mounting

Once a PVC exists and has been created, it needs to be mounted. This relies on `iscsiadm` existing on the host in a decent
location i.e /bin /usr/bin etc...

This part is a bit... sketchy, basically I've repurposed 90% of the main iSCSI CSI driver and its library so I cant provide
too much insight here when things don't work.

Create a pod specifying a volume to mount:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app
spec:
  nodeSelector:
    kubernetes.io/hostname: kube02
  containers:
  - name: app
    image: centos
    command: ["/bin/sh"]
    args: ["-c", "while true; do echo $(date -u) >> /data/out.txt; sleep 5; done"]
    volumeMounts:
    - name: persistent-storage
      mountPath: /data
  volumes:
  - name: persistent-storage
    persistentVolumeClaim:
      claimName: qnap-test-claim
```
Pick a node and specify that node specifically, this will make it easier to figure out which daemonset pod to look at.

There is a `node` container created which runs on every node, the useful log files would be from the `node-server` container.

## Troubleshooting

If this driver is useful to people other than myself I'll look into making it a bit more friendly, creating some more
troubleshooting utilities like something simulating what this driver would do automatically to ensure pre-requisites exist.

If it doesnt work, gather some logs and raise a github issue 


# How it works

At a high level, it listens for persistent volume claims, talks to the QNAP API to create an iSCSI target/initiator and block 
volume. Then it calls iscsiadm on the host and mounts the volume.

## QNAP API

The QNAP I have has a Storage & Snapshots "app" which is where you manage iSCSI volumes. It does not have a proper official
API, so I've pieced together the bare minimum required to create a volume by looking at what the browser submits. It's all
XML based and a pile of garbage, there are random parts of the API which has no error checking and will result in the
HTTP connection being dropped with no response, and you'll get a lovely error message in the NAS dashboard.

## iSCSI

When a request to create a volume goes to the controller, it sets some context values which would be iSCSI IQN, portal ip's etc...
This then eventually gets sent to the node that needs to mount the volume, it passes those values to an `iscsiadm` shim binary
which essentially finds the actual iscsiadm binary on the host (though a mounted volume) and runs it with inside a chroot of the host's
volume.

If you're going to get any errors it'll most likely be weird iSCSI return codes which are ultra cryptic.

