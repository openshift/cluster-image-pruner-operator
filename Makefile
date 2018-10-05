IMAGE ?= docker.io/openshift/origin-cluster-prune-operator:latest
PROG  := cluster-prune-operator

.PHONY: all generate build build-image build-devel-image clean

all: generate build build-image

generate:
	./tmp/codegen/update-generated.sh

build:
	./tmp/build/build.sh

build-image:
	docker build -t "$(IMAGE)" .

build-devel-image: build
	docker build -t "$(IMAGE)" -f Dockerfile.dev .

clean:
	rm -- "$(PROG)"
