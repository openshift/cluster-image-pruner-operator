FROM alpine:3.6

RUN adduser -D cluster-image-pruner-operator
USER cluster-image-pruner-operator

ADD tmp/_output/bin/cluster-image-pruner-operator /usr/local/bin/cluster-image-pruner-operator
