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

cleanup:
	rm -f data/data.db

extract input-dir: build
	./main extract -i {{input-dir}}

chunk: build
	./main chunk

embed: build
	./main embed

retrieve user-query: build
	./main retrieve -q '{{user-query}}'

inspect: build
	./main inspect

test:
      #!/usr/bin/env bash
      set -euxo pipefail
      CGO_ENABLED=1 go test --tags "fts5" $(go list ./... | grep -v '/cmd/scrape')

###############
# scrape-cli
###############
scrape-build:
	go build -o ./scrape cmd/scrape
	chmod a+x ./scrape

mw: scrape-build
	./scrape mw -o data/mw-download -a "https://wiki.krumedia.com/api.php"

