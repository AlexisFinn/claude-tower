GO_ENV := GOPROXY=off GOFLAGS=-mod=readonly
PREFIX ?= $(HOME)/.local

ifeq ($(shell uname),Linux)
TAGS := -tags webkit2_41
else
TAGS :=
endif

.PHONY: build test install uninstall

build:
	$(GO_ENV) wails build $(TAGS)

test:
	@test -d frontend/node_modules || { echo "frontend/node_modules est absent : lancez npm install dans frontend" >&2; exit 1; }
	$(GO_ENV) go vet ./internal/...
	$(GO_ENV) go test ./internal/...
ifeq ($(shell uname),Linux)
	$(GO_ENV) go vet -tags webkit2_41 .
endif
	cd frontend && npx vitest run && npm run check

ifeq ($(shell uname),Linux)
install: build
	install -Dm755 build/bin/claude-tower "$(PREFIX)/bin/claude-tower"
	install -Dm644 build/appicon.png "$(PREFIX)/share/pixmaps/claude-tower.png"
	mkdir -p "$(PREFIX)/share/applications"
	sed "s|@PREFIX@|$$(printf '%s' "$(PREFIX)" | sed 's/[|&\\]/\\&/g')|g" build/linux/claude-tower.desktop.in > "$(PREFIX)/share/applications/claude-tower.desktop"

uninstall:
	rm -f "$(PREFIX)/bin/claude-tower" "$(PREFIX)/share/pixmaps/claude-tower.png" "$(PREFIX)/share/applications/claude-tower.desktop"
else
install: build
	rm -rf /Applications/claude-tower.app
	cp -R build/bin/claude-tower.app /Applications/
	mkdir -p "$(PREFIX)/bin"
	ln -sf /Applications/claude-tower.app/Contents/MacOS/claude-tower "$(PREFIX)/bin/claude-tower"

uninstall:
	rm -rf /Applications/claude-tower.app "$(PREFIX)/bin/claude-tower"
endif
