FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.16-openshift-4.9 AS builder

WORKDIR /go/src/github.com/openshift/cluster-resource-override-admission
COPY . .
RUN make build

FROM registry.ci.openshift.org/ocp/4.9:base

LABEL io.k8s.display-name="OpenShift ClusterResourceOverride Admission Webhook" \
      io.k8s.description="Manages level of overcommit of Pod Resource(s)" \
      io.openshift.tags="openshift,clusterresourceoverride"
COPY --from=builder /go/src/github.com/openshift/cluster-resource-override-admission/bin/cluster-resource-override-admission /usr/bin/
