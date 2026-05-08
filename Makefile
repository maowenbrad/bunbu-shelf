.PHONY: all build test lint css css-watch setup clean reindex

TAILWIND_BIN := .bin/tailwindcss
CSS_SRC      := web/static/input.css
CSS_OUT      := web/static/app.css

setup:
	mkdir -p .bin
	curl -sLo $(TAILWIND_BIN) \
		https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64
	chmod +x $(TAILWIND_BIN)

css: $(CSS_OUT)

$(CSS_OUT): $(CSS_SRC) tailwind.config.js $(shell find web/templates -name '*.html' 2>/dev/null)
	$(TAILWIND_BIN) -i $(CSS_SRC) -o $(CSS_OUT) --minify

css-watch:
	$(TAILWIND_BIN) -i $(CSS_SRC) -o $(CSS_OUT) --watch

build: css
	mkdir -p bin
	go build -o bin/shelfd ./cmd/shelfd
	go build -o bin/shelf  ./cmd/shelf

build-no-css:
	mkdir -p bin
	go build -o bin/shelfd ./cmd/shelfd
	go build -o bin/shelf  ./cmd/shelf

test:
	go test ./...

lint:
	go vet ./...

reindex:
	go run ./cmd/shelf reindex

clean:
	rm -rf bin/ $(CSS_OUT)
