BUILD_TOOL=docker
FILE=build/router.dockerfile
REGISTRY=ghcr.io
OWNER=iskorotkov
REPO=router
TAG=dev

.PHONY: build push

build:
	CI=true $(BUILD_TOOL) build -t $(REGISTRY)/$(OWNER)/$(REPO):$(TAG) -f $(FILE) .

push: build
	CI=true $(BUILD_TOOL) push $(REGISTRY)/$(OWNER)/$(REPO):$(TAG)
