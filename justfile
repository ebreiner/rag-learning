build:
	#!/usr/bin/env bash
	set -euxo pipefail
	export CGO_CFLAGS="-I$HOME/.local/include"
	export CGO_LDFLAGS="-L$HOME/.local/lib -lkreuzberg_ffi"
	CGO_ENABLED=1 go build --tags "fts5" -o ./main main.go
	chmod a+x ./main

mw: build
	./main mw -o data/mw-download -a "https://wiki.krumedia.com/api.php"

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
