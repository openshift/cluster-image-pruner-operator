FROM alpine:3.6

RUN adduser -D cluster-prune-operator
USER cluster-prune-operator

ADD tmp/_output/bin/cluster-prune-operator /usr/local/bin/cluster-prune-operator
