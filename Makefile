REGISTRY_NAME?=docker.io/iejalapeno
IMAGE_VERSION?=latest

.PHONY: all linkstate-edge-v4 container push clean test

ifdef V
TESTARGS = -v -args -alsologtostderr -v 5
else
TESTARGS =
endif

all: linkstate-edge-v4

linkstate-edge-v4:
	mkdir -p bin
	$(MAKE) -C ./cmd compile-linkstate-edge-v4

linkstate-edge-v4-container: linkstate-edge-v4
	docker build -t $(REGISTRY_NAME)/linkstate-edge-v4:$(IMAGE_VERSION) -f ./build/Dockerfile.linkstate-edge-v4 .

push: linkstate-edge-v4-container
	docker push $(REGISTRY_NAME)/linkstate-edge-v4:$(IMAGE_VERSION)

clean:
	rm -rf bin

test:
	GO111MODULE=on go test `go list ./... | grep -v 'vendor'` $(TESTARGS)
	GO111MODULE=on go vet `go list ./... | grep -v vendor`
