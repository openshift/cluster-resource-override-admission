FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.19-openshift-4.13 AS builder
WORKDIR /go/src/github.com/openshift/cluster-resource-override-admission
COPY . .
RUN make build
FROM registry.ci.openshift.org/ocp/4.13:base
COPY --from=builder /go/src/github.com/openshift/cluster-resource-override-admission/bin/cluster-resource-override-admission /usr/bin/
