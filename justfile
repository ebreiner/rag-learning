set dotenv-load

config := env("RAG_CONFIG", "config.yaml")
export OTEL_EXPORTER_OTLP_ENDPOINT := env("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
export OTEL_SERVICE_NAME := "rag-cli-dev"


build:
        #!/usr/bin/env bash
        set -euxo pipefail
        CGO_ENABLED=1 go build --tags "fts5" -o ./main main.go
        chmod a+x ./main

cleanup:
        rm -f data/data.db*

ingest: build
        ./main -c {{config}} ingest

ingest-clean: cleanup ingest

# delete all chunks and embeddings, keep the extractions, chunk and embed again
ingest-rechunk: build
        ./main -c {{config}} ingest --rechunk

retrieve user-query: build
        ./main -c {{config}} retrieve -q '{{user-query}}'

eval baseline="": build
        ./main -c {{config}} eval {{ if baseline != "" { "--baseline " + baseline } else { "" } }}

serve-mcp: build
        ./main -c {{config}} serve mcp

test:
        #!/usr/bin/env bash
        set -euxo pipefail
        CGO_ENABLED=1 go test --tags "fts5" ./...

lint:
        CGO_ENABLED=1 go vet --tags "fts5" ./...
        golangci-lint run -c golangci-lint.yaml

