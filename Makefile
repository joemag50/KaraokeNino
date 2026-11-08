# KaraokeNino build tasks.
#
#   make build        compile for this computer
#   make test         unit tests
#   make test-linux   all tests on Linux with a real mpv (Docker)
#   make test-e2e     Linux end-to-end: auto-mount, scan, embedded video (Docker --privileged)
#   make snapshots    render the screens to PNG (visual check) in $(SNAPDIR)
#   make release      Linux package in dist/ (Docker; ARCH=amd64 or 386)
#   make catalog      import a Musica.data list into data/catalog.db (MUSICA=path)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
ARCH    ?= amd64
LDFLAGS := -s -w -X main.version=$(VERSION)
MUSICA  ?= Musica.data
SNAPDIR ?= snapshots
PKG     := karaokenino-$(VERSION)-linux-$(ARCH)
DOCKER_GO := docker run --rm -v "$(CURDIR)":/src -w /src -e GOFLAGS=-buildvcs=false \
	-v karaokenino-gocache:/root/.cache -v karaokenino-gomod:/go/pkg/mod

.PHONY: build test test-linux test-e2e snapshots release catalog clean

build:
	go build -ldflags "$(LDFLAGS)" -o karaokenino ./cmd/karaokenino
	go build -ldflags "$(LDFLAGS)" -o importcatalog ./cmd/importcatalog

test:
	go vet ./...
	go test ./...

test-linux:
	docker build -q -t karaokenino-test -f deploy/Dockerfile.build --target test deploy
	$(DOCKER_GO) karaokenino-test go test -tags mpv ./...

test-e2e:
	mkdir -p deploy/e2e/out
	docker build -q -t karaokenino-test -f deploy/Dockerfile.build --target test deploy
	docker run --rm --privileged -v "$(CURDIR)":/src:ro -v "$(CURDIR)/deploy/e2e/out":/out \
		-v karaokenino-gocache:/root/.cache -v karaokenino-gomod:/go/pkg/mod -e GOFLAGS=-buildvcs=false \
		karaokenino-test sh -c 'sh /src/deploy/e2e/run.sh; code=$$?; losetup -D; exit $$code'
	@echo "Capturas en deploy/e2e/out/"

snapshots:
	mkdir -p $(SNAPDIR)
	KN_SNAPSHOTS=$(CURDIR)/$(SNAPDIR) go test -count=1 -run 'TestBrowse|TestScales' ./internal/ui/

# Built on Debian bookworm (glibc 2.36): runs on Debian 12+ and bookworm-based Puppy.
release:
	docker build -q --platform linux/$(ARCH) -t karaokenino-build-$(ARCH) -f deploy/Dockerfile.build --target build deploy
	$(DOCKER_GO) --platform linux/$(ARCH) karaokenino-build-$(ARCH) sh -c '\
		go build -ldflags "$(LDFLAGS)" -o dist/$(PKG)/karaokenino ./cmd/karaokenino && \
		go build -ldflags "$(LDFLAGS)" -o dist/$(PKG)/importcatalog ./cmd/importcatalog'
	mkdir -p dist/$(PKG)/config dist/$(PKG)/deploy dist/$(PKG)/karaokenino-update
	cp -r docs README.md dist/$(PKG)/
	cp config/config.example.yaml dist/$(PKG)/config/
	cp deploy/*.sh deploy/*.desktop dist/$(PKG)/deploy/
	cp dist/$(PKG)/karaokenino deploy/kiosk-start.sh deploy/update.sh dist/$(PKG)/karaokenino-update/
	cd dist/$(PKG)/karaokenino-update && sha256sum karaokenino > karaokenino.sha256
	COPYFILE_DISABLE=1 tar --no-xattrs -C dist -czf dist/$(PKG).tar.gz $(PKG)
	@echo "Listo: dist/$(PKG).tar.gz"
	@echo "  Instalar: descomprimir en la rockola y ejecutar sudo ./deploy/install.sh"
	@echo "  Actualizar: copiar dist/$(PKG)/karaokenino-update/ a la raíz del disco de música"

catalog: build
	./importcatalog -musica "$(MUSICA)"

clean:
	rm -rf dist karaokenino importcatalog $(SNAPDIR)
