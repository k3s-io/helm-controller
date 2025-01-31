IMAGE_NAME ?= helm-controller
.DEFAULT_GOAL := ci

.PHONY: build test validate package clean

build:
	DOCKER_BUILDKIT=1 docker build \
		--target binary \
		--output type=local,dest=. .

test:
	docker build --target dev -t $(IMAGE_NAME)-dev .
	docker run --rm $(IMAGE_NAME)-dev ./scripts/test

validate:
	docker build --target dev -t $(IMAGE_NAME)-dev .
	docker run --rm $(IMAGE_NAME)-dev ./scripts/validate

package: SHELL:=/bin/bash
package: 
	docker build --target artifacts --output type=local,dest=. .

	source ./scripts/version &&	IMAGE=$${REPO}/helm-controller:$${TAG}; \
		docker build -t $${IMAGE} --target production .; \
		echo $${IMAGE} > bin/helm-controller-image.txt; \
		echo Built $${IMAGE}

clean:
	rm -rf bin/* dist/*

ci: build test validate package
