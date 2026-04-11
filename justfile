###############
# rag-cli
###############
build:
	#!/usr/bin/env bash
	set -euxo pipefail
	export CGO_CFLAGS="-I$HOME/.local/include"
	export CGO_LDFLAGS="-L$HOME/.local/lib -lkreuzberg_ffi"
	CGO_ENABLED=1 go build --tags "fts5" -o ./main main.go
	chmod a+x ./main

extract input-dir output-dir: build
	./main extract -i {{input-dir}} -o {{output-dir}}

chunk input-dir output-dir: build
	./main chunk -i {{input-dir}} -o {{output-dir}}

embed input-dir output-dir: build
	./main embed -i {{input-dir}} -o {{output-dir}}

load-db: build
	rm -f ./data/data.db*
	./main load-db

test: build
	./main test

serve: build
	./main serve mcp


###############
# scrape-cli
###############
scrape-build:
	go build -o ./scrape cmd/scrape
	chmod a+x ./scrape

mw: scrape-build
	./scrape mw -o data/mw-download -a "https://wiki.krumedia.com/api.php"

