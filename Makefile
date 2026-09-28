VERSION ?= dev
COMMIT_SHA ?=
TOKEN ?=
IMAGE_NAME ?= ghcr.io/arnica-ext/prom-aggregation-gateway
PKG_PATH := github.com/arnica-ext/prom-aggregation-gateway

.PHONY: test ci-golang ci-helm lint test-golang test-helm build build-image build-image-multiarch build-binaries release-binaries build-helm continuous-deploy

test: lint test-golang

lint:
	staticcheck ./...

test-golang:
	CGO_ENABLED=0 go test ./...

test-helm:
	ct --config ./.github/ct.yaml lint ./charts

build:
	go build -ldflags "-X $(PKG_PATH)/config.Version=$(VERSION) -X $(PKG_PATH)/config.CommitSHA=$(COMMIT_SHA)" -o prom-aggregation-gateway .

build-image:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT_SHA=$(COMMIT_SHA) \
		-t $(IMAGE_NAME):$(VERSION) \
		-t $(IMAGE_NAME):latest \
		.

build-image-multiarch:
	docker buildx build \
		--platform linux/arm64,linux/amd64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT_SHA=$(COMMIT_SHA) \
		-t $(IMAGE_NAME):$(VERSION) \
		-t $(IMAGE_NAME):latest \
		.

push-image: build-image-multiarch
	docker push $(IMAGE_NAME)

PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/386 linux/arm linux/arm64 linux/ppc64le linux/s390x windows/amd64

build-binaries:
	@mkdir -p _dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output="_dist/prom-aggregation-gateway-$(VERSION)-$${os}-$${arch}"; \
		if [ "$${os}" = "windows" ]; then output="$${output}.exe"; fi; \
		echo "Building $${os}/$${arch}..."; \
		GOFLAGS="-trimpath" CGO_ENABLED=0 GOOS=$${os} GOARCH=$${arch} \
			go build -ldflags "-X $(PKG_PATH)/config.Version=$(VERSION) -X $(PKG_PATH)/config.CommitSHA=$(COMMIT_SHA)" \
			-o "$${output}" . || exit 1; \
	done

release-binaries: build-binaries
	GH_TOKEN=$(TOKEN) gh release upload $(VERSION) ./_dist/*

build-helm:
	cr --config .github/cr.yaml package charts/*
	mkdir -p .cr-index
	git config --global user.email "41898282+github-actions[bot]@users.noreply.github.com"
	git config --global user.name "github-actions[bot]"
	git fetch --prune --unshallow || true
	CR_TOKEN=$(TOKEN) cr --config .github/cr.yaml upload --token $(TOKEN) --skip-existing
	CR_TOKEN=$(TOKEN) cr --config .github/cr.yaml index --token $(TOKEN) --push

continuous-deploy: build-helm
