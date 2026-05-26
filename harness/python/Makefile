# Build and publish the `crossfuzz` package to PyPI.
#
# Usage (run from harness/python/):
#   make build            # produce dist/ artifacts
#   make check            # twine check (README rendering, metadata)
#   make test-install     # install built wheel in a scratch venv and import
#   make upload-testpypi  # upload to TestPyPI
#   make upload           # upload to PyPI
#   make sync-license     # copy ../../LICENSE here (run after license changes)
#   make clean

VENV     := .venv
PYTHON   := $(VENV)/bin/python
PIP      := $(VENV)/bin/pip
VERSION  := $(shell $(PYTHON) -c "import tomllib; print(tomllib.load(open('pyproject.toml','rb'))['project']['version'])")
DIST     := dist/crossfuzz-$(VERSION)-py3-none-any.whl dist/crossfuzz-$(VERSION).tar.gz

.PHONY: build check test-install upload-testpypi upload sync-license clean tools

tools:
	$(PIP) install --quiet --upgrade build twine

LICENSE: ../../LICENSE
	cp ../../LICENSE LICENSE

sync-license: LICENSE

build: tools LICENSE
	rm -rf dist build *.egg-info
	$(PYTHON) -m build
	@ls -1 dist/

check: build
	$(PYTHON) -m twine check dist/*

test-install: build
	rm -rf $${TMPDIR:-/tmp}/crossfuzz-test
	python3 -m venv $${TMPDIR:-/tmp}/crossfuzz-test
	$${TMPDIR:-/tmp}/crossfuzz-test/bin/pip install --quiet $(CURDIR)/dist/crossfuzz-$(VERSION)-py3-none-any.whl
	cd $${TMPDIR:-/tmp}/crossfuzz-test && bin/python -c "import crossfuzz, os; assert 'site-packages' in crossfuzz.__file__, crossfuzz.__file__; print('OK:', crossfuzz.__file__)"
	rm -rf $${TMPDIR:-/tmp}/crossfuzz-test

upload-testpypi: check
	$(PYTHON) -m twine upload --repository testpypi dist/*

upload: check
	@echo "Uploading crossfuzz $(VERSION) to PyPI..."
	$(PYTHON) -m twine upload dist/*

clean:
	rm -rf dist build *.egg-info
