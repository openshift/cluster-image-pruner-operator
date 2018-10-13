FROM openshift/origin-release:golang-1.10 as builder
WORKDIR /go/src/github.com/openshift/cluster-image-pruner-operator
COPY . .
RUN make build

FROM centos:7
RUN useradd cluster-image-pruner-operator
USER cluster-image-pruner-operator
COPY --from=builder /go/src/github.com/openshift/cluster-image-pruner-operator/tmp/_output/bin/cluster-image-pruner-operator /usr/bin
# these manifests are necessary for the installer
COPY deploy/image-references deploy/00-crd.yaml deploy/01-namespace.yaml deploy/02-rbac.yaml deploy/03-sa.yaml deploy/04-operator.yaml /manifests/
LABEL io.openshift.release.operator true
