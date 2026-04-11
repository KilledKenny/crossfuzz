
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
	cd ./harness/java && gradle jar


HARNESS_TARGET+= harness/js/node_modules
harness/js/node_modules: harness/js/package.json harness/js/bun.lock
	cd harness/js && bun install
	touch $@

.PHONY: test
test:
	go test ./...


ALL_TARGET+= $(HARNESS_TARGET)
.PHONY: harness
harness: $(HARNESS_TARGET)

.DEFAULT_GOAL := all
.PHONY: all
all: $(ALL_TARGET)
