IMAGE_NAME ?= helm-controller
ARCH ?= amd64

.DEFAULT_GOAL := ci
.PHONY: build test validate package clean generate-crd

build:  generate-crd
	DOCKER_BUILDKIT=1 docker build \
		--target binary \
		--output type=local,dest=. .

generate-crd:
	DOCKER_BUILDKIT=1 docker build \
		--target crds \
		--output type=local,dest=./pkg/crds/yaml/generated .

validate:
	docker build --target dev --build-arg ARCH=$(ARCH) -t $(IMAGE_NAME)-dev .
	docker run --rm $(IMAGE_NAME)-dev ./scripts/validate

test:
	docker build --target dev --build-arg ARCH=$(ARCH) -t $(IMAGE_NAME)-dev .
	docker run --rm $(IMAGE_NAME)-dev ./scripts/test

package: SHELL:=/bin/bash
package: 
	docker build --target artifacts --build-arg ARCH=$(ARCH) --output type=local,dest=. .
	source ./scripts/version &&	IMAGE=$${REPO}/helm-controller:$${TAG}; \
		docker build -t $${IMAGE} --target production .; \
		echo $${IMAGE} > bin/helm-controller-image.txt; \
		echo Built $${IMAGE}

clean:
	rm -rf bin dist

ci: build validate test package
