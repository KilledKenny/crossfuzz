
GO_CLI_INPUTS=$(shell go list -deps -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}{{"\n"}}{{end}}' ./cmd/crossfuzz/ )

bin/:
	mkdir $@
	touch $@

ALL_TARGET=bin/crossfuzz
bin/crossfuzz: bin/ $(GO_CLI_INPUTS)
	go build  -o $@ ./cmd/crossfuzz


JAVA_HARNES_INPUT=$(find harness/java/ -type f \( -name "*.java" -o -name "*.gradle" \))

HARNESS_TARGET+= harness/java/build/libs/crossfuzz.jar
harness/java/build/libs/crossfuzz.jar: $(JAVA_HARNES_INPUT)
	cd ./harness/java && ./gradlew jar


PYTHON_HARNESS_VENV := harness/python/.venv

HARNESS_TARGET+=$(PYTHON_HARNESS_VENV)
$(PYTHON_HARNESS_VENV):
	python3 -m venv $(PYTHON_HARNESS_VENV)
	$(PYTHON_HARNESS_VENV)/bin/pip install --quiet -r harness/python/requirements.txt
	touch $@

HARNESS_TARGET+= harness/rust/target/release/libcrossfuzz_harness.rlib
harness/rust/target/release/libcrossfuzz_harness.rlib: harness/rust/Cargo.toml $(wildcard harness/rust/src/*.rs)
	cd harness/rust && cargo build --release

HARNESS_TARGET+= harness/js/node_modules
harness/js/node_modules: harness/js/package.json harness/js/bun.lock
	cd harness/js && bun install
	touch $@

.PHONY: test
test:
	go test -race ./...

E2E_INPUTS=$(shell find e2e -name '*.go' 2>/dev/null)
bin/crossfuzz-e2e: bin/ $(E2E_INPUTS)
	go build -o $@ ./e2e

.PHONY: test-e2e
test-e2e: $(ALL_TARGET) bin/crossfuzz-e2e
	./bin/crossfuzz-e2e $(E2E_ARGS)


ALL_TARGET+= $(HARNESS_TARGET)
.PHONY: harness
harness: $(HARNESS_TARGET)

HARNESS_BUILD_DIR ?= harness/build
PREFIX            ?= /usr/local

.PHONY: install-c-harnes
install-c-harnes:
	cmake -B $(HARNESS_BUILD_DIR) harness/
	cmake --install $(HARNESS_BUILD_DIR) --prefix $(PREFIX)

.PHONY: uninstall-c-harnes
uninstall-c-harnes:
	cmake --build $(HARNESS_BUILD_DIR) --target uninstall 2>/dev/null || \
	  xargs rm -f < $(HARNESS_BUILD_DIR)/install_manifest.txt

CI_IMAGE ?= crossfuzz-ci:latest

.PHONY: ci-image
ci-image: bin/ nix/docker.nix nix/packages.nix nix/nixpkgs.nix
	nix-build nix/docker.nix -o bin/ci-image
	docker load --input bin/ci-image

.PHONY: test-e2e-docker
test-e2e-docker:
	docker run --rm \
	  -v "$(CURDIR)":/w \
	  -w /w \
	  $(CI_IMAGE) \
	  bash -c "set -euo pipefail; make harness; make test; make test-e2e"

.DEFAULT_GOAL := all
.PHONY: all
all: $(ALL_TARGET)
