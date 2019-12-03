## Admission Webhooks over localhost
* Use `DaemonSet`: The DaemonSet controller can make Pods even when the scheduler has not been started, which can help cluster bootstrap.
* Use `hostnetwork` for `PodSpec`
* The pods must be scheduled on to each master node so that core API server can access the webhook.
* The Admission Webhook generates`localhost` serving certs. 
* The Admission Webhook binds to `127.0.0.1` to disable external connection.
* Use `https://localhost` to define the `URL` of the Admission Webhook.


Note that we are not using the following:
* The `DaemonSet` pods are not fronted by any `Service`
* No API aggregation, the Admission webhook can not be reached via the `kubernetes.default.svc` service. So we don't get the advantages to registering the webhook server as an aggregated API.   

### Setup
* Grant the `ServiceAccount` of the `DaemonSet` access to the `hostnetwork` `SCC`
```bash
oc adm policy add-scc-to-user hostnetwork system:serviceaccount:cluster-resource-override:clusterresourceoverride
``` 
* Grant `create` verb on the designated API resource of the API group the admission webhook exposes to `system:anonymous`.  
* Deploy the `DaemonSet`
* Create a `MutatingWebhokConfiguration`

#### MutatingWebhookConfiguration 
```yaml
apiVersion: admissionregistration.k8s.io/v1beta1
kind: MutatingWebhookConfiguration
metadata:
  name: clusterresourceoverrides.admission.autoscaling.openshift.io
  labels:
    clusterresourceoverride: "true"
webhooks:
  - name: clusterresourceoverrides.admission.autoscaling.openshift.io
    clientConfig:
      # serving on localhost.
      url: https://localhost:9443/apis/admission.autoscaling.openshift.io/v1/clusterresourceoverrides
      caBundle: SERVICE_SERVING_CERT_CA
    rules:
      - operations:
          - CREATE
          - UPDATE
        apiGroups:
          - ""
        apiVersions:
          - "v1"
        resources:
          - "pods"
    failurePolicy: Fail
```

#### DaemonSet Spec
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: clusterresourceoverride
  labels:
    clusterresourceoverride: "true"
spec:
  selector:
    matchLabels:
      clusterresourceoverride: "true"
  template:
    metadata:
      name: clusterresourceoverride
      labels:
        clusterresourceoverride: "true"
    spec:
      nodeSelector:
        # we want the pods to be running on every master node.
        node-role.kubernetes.io/master: ''
      
      # enable hostNetwork to do localhost serving  
      hostNetwork: true

      serviceAccountName: clusterresourceoverride
      containers:
        - name: clusterresourceoverride
          image: docker.io/tohinkashem/demo.clusterresourceoverride:dev
          imagePullPolicy: Always
          args:
            # the server binds to 127.0.0.1 to disable external connection.
            # pod readiness and liveness check does not work.  
            - "--bind-address=127.0.0.1"
            - "--secure-port=9443"            
            - "--audit-log-path=-"
            - "--tls-cert-file=/var/serving-cert/tls.crt"
            - "--tls-private-key-file=/var/serving-cert/tls.key"
            - "--v=8"
          env:
            - name: CONFIGURATION_PATH
              value: /etc/clusterresourceoverride/config/override.yaml
          ports:
            - containerPort: 9443
              hostPort: 9443
              protocol: TCP
          volumeMounts:
            - mountPath: /var/serving-cert
              name: serving-cert
          readinessProbe:
            httpGet:
              path: /healthz
              port: 9443
              scheme: HTTPS
      volumes:
        - name: serving-cert
          secret:
            defaultMode: 420
            secretName: server-serving-cert
      tolerations:
        - key: node-role.kubernetes.io/master
          operator: Exists
          effect: NoSchedule
        - key: node.kubernetes.io/unreachable
          operator: Exists
          effect: NoExecute
          tolerationSeconds: 120
        - key: node.kubernetes.io/not-ready
          operator: Exists
          effect: NoExecute
          tolerationSeconds: 120
```

#### SubjectAccessReview
The Admission Webhook server posts a `SubjectAccessReview` request to the core API server.
```json
{
  "kind":"SubjectAccessReview",
  "apiVersion":"authorization.k8s.io/v1beta1",
  "metadata":{
    "creationTimestamp":null
  },
  "spec":{
    "resourceAttributes":{
      "verb":"create",
      "group":"admission.autoscaling.openshift.io",
      "version":"v1",
      "resource":"clusterresourceoverrides"
    },
    "user":"system:anonymous",
    "group":[
      "system:unauthenticated"
    ]
  }
}
```
The core API server responds with `"status":{"allowed":false}`. 
```
Forbidden: "/apis/admission.autoscaling.openshift.io/v1/clusterresourceoverrides?timeout=30s", Reason: ""
```

To solve this issue grant `create` verb on the designated API resource to `system:anonymous`.  
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: clusterresourceoverride-anonymous-access
rules:
  - apiGroups:
      - "admission.autoscaling.openshift.io"
    resources:
      - "clusterresourceoverrides"
    verbs:
      - create
      - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: clusterresourceoverride-anonymous-access
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: clusterresourceoverride-anonymous-access
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: system:anonymous
```
