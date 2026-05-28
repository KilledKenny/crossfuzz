
GO_CLI_INPUTS=$(shell go list -deps -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}{{"\n"}}{{end}}' ./cmd/crossfuzz/ )

bin/:
	mkdir $@
	touch $@

ALL_TARGET=bin/crossfuzz
bin/crossfuzz: bin/ $(GO_CLI_INPUTS)
	go build  -o $@ ./cmd/crossfuzz


JAVA_HARNES_INPUT=$(find harness/java/src/ -name "*.java")
JAVA_HARNES_INPUT+= $(wildcard harness/java/*.gradle)

HARNESS_TARGET+= harness/java/build/libs/crossfuzz.jar
harness/java/build/libs/crossfuzz.jar: $(JAVA_HARNES_INPUT)
	cd ./harness/java && ./gradlew jar


HARNESS_TARGET+= harness/python/.deps_installed
harness/python/.deps_installed: harness/python/.venv/bin/python3 harness/python/requirements.txt
	harness/python/.venv/bin/pip install --quiet -r harness/python/requirements.txt
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

.DEFAULT_GOAL := all
.PHONY: all
all: $(ALL_TARGET)
