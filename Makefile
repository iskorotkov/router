BUILD_TOOL=docker
FILE=build/router.dockerfile
REGISTRY=ghcr.io
OWNER=iskorotkov
REPO=router
TAG=dev

.PHONY: build push run

build:
	CI=true $(BUILD_TOOL) build -t $(OWNER)/$(REPO):$(TAG) -f $(FILE) .

push: build
	CI=true $(BUILD_TOOL) push $(OWNER)/$(REPO):$(TAG) docker://$(REGISTRY)/$(OWNER)/$(REPO):$(TAG)
