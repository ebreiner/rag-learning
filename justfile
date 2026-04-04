build:
	#!/usr/bin/env bash
	set -euxo pipefail
	export CGO_CFLAGS="-I$HOME/.local/include"
	export CGO_LDFLAGS="-L$HOME/.local/lib -lkreuzberg_ffi"
	CGO_ENABLED=1 go build --tags "fts5" -o ./main main.go
	chmod a+x ./main

scrape: build
	./main scrape

convert: build
	./main scrape -s

chunk: build
	./main chunk

dummy: build
	./main dummy

embed: build
	./main embed

load-db: build
	rm -f ./data/data.db*
	./main load-db

test: build
	./main test

serve: build
	./main serve mcp
