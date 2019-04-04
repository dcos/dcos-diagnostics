DEFAULT_TARGET: build
CURRENT_DIR=$(shell pwd)
BUILD_DIR=build
PKG_DIR=/dcos-diagnostics
BINARY_NAME=dcos-diagnostics
IMAGE_NAME=dcos-diagnostics-dev

all: test install

.PHONY: docker
docker:
ifndef NO_DOCKER
	docker build -t $(IMAGE_NAME) .
endif

.PHONY: build
build: docker
	mkdir -p $(BUILD_DIR)
	$(call inDocker,go build -mod=vendor -v -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY_NAME))

.PHONY: test
test: docker
		$(call inDocker,bash -x -c './scripts/test.sh')


# install does not run in a docker container to build for the correct OS
.PHONY: install
install:
	go install -mod=vendor -v -ldflags '$(LDFLAGS)'

.PHONY: clean
clean:
	rm -rf ./$(BUILD_DIR)

ifdef NO_DOCKER
  define inDocker
    $1
  endef
else
  define inDocker
    docker run \
      -v $(CURRENT_DIR):$(PKG_DIR) \
      -w $(PKG_DIR) \
      --rm \
      $(IMAGE_NAME) \
      $1
  endef
endif