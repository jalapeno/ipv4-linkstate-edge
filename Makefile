REGISTRY_NAME?=docker.io/iejalapeno
IMAGE_VERSION?=latest

.PHONY: all ls-edge container push clean test

ifdef V
TESTARGS = -v -args -alsologtostderr -v 5
else
TESTARGS =
endif

all: ls-edge

ls-edge:
	mkdir -p bin
	$(MAKE) -C ./cmd/ls-edge compile-ls-edge

ls-edge-container: ls-edge
	docker build -t $(REGISTRY_NAME)/ls-edge:$(IMAGE_VERSION) -f ./build/Dockerfile.ls-edge .

push: ls-edge-container
	docker push $(REGISTRY_NAME)/ls-edge:$(IMAGE_VERSION)

clean:
	rm -rf bin

test:
	GO111MODULE=on go test `go list ./... | grep -v 'vendor'` $(TESTARGS)
	GO111MODULE=on go vet `go list ./... | grep -v vendor`
