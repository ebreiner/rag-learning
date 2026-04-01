export CGO_CFLAGS := "-I/home/emil.breiner/.local/include"
export CGO_LDFLAGS := "-L/home/emil.breiner/.local/lib -lkreuzberg_ffi"

build:
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
