DEFAULT_TARGET: build
CURRENT_DIR=$(shell pwd)
BUILD_DIR=build
PKG_DIR=/dcos-diagnostics
BINARY_NAME=dcos-diagnostics
IMAGE_NAME=dcos-diagnostics-dev

all: test install

.PHONY: docker
docker:
	docker build -t $(IMAGE_NAME) .

.PHONY: build
build: docker
	mkdir -p $(BUILD_DIR)
	docker run \
		-v $(CURRENT_DIR):$(PKG_DIR) \
		-w $(PKG_DIR) \
		--rm \
		$(IMAGE_NAME) \
		go build -mod=vendor -v -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY_NAME)

.PHONY: test
test: docker
	this will fail

# install does not run in a docker container because it only compiles on linux.
.PHONY: install
install:
	go install -mod=vendor -v -ldflags '$(LDFLAGS)'

.PHONY: clean
clean:
	rm -rf ./$(BUILD_DIR)
