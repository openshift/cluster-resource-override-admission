FROM registry.ci.openshift.org/openshift/release:rhel-9-release-golang-1.24-openshift-4.21 AS builder
WORKDIR /go/src/github.com/openshift/cluster-resource-override-admission
COPY . .
RUN make build
FROM registry.ci.openshift.org/ocp/4.21:base-rhel9
COPY --from=builder /go/src/github.com/openshift/cluster-resource-override-admission/bin/cluster-resource-override-admission /usr/bin/
