export OTEL_EXPORTER_OTLP_ENDPOINT := "http://127.0.0.1:4318"
export OTEL_SERVICE_NAME := "rag-cli-dev"


build:
	#!/usr/bin/env bash
	set -euxo pipefail
	CGO_ENABLED=1 go build --tags "fts5" -o ./main main.go
	chmod a+x ./main

cleanup:
	rm -f data/data.db*

extract input-dir: build
	./main --db-path data/data.db extract -i {{input-dir}} 

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
      CGO_ENABLED=1 go test --tags "fts5" ./...

lint:
	go vet ./...
	golangci-lint run -c golangci-lint.yaml


e2e input-dir: build
	#!/usr/bin/env bash
	set -euxo pipefail
	./main --db-path ./data/data.db extract -i {{input-dir}} --docling-url "http://127.0.0.1:5001"
	./main --db-path ./data/data.db chunk
	./main --db-path ./data/data.db embed --openai-url "http://127.0.0.1:11434" --model bge-m3 --dim 1024

e2e-clear input-dir: build cleanup
	#!/usr/bin/env bash
	set -euxo pipefail
	./main --db-path ./data/data.db extract -i {{input-dir}} --docling-url "http://127.0.0.1:5001"
	./main --db-path ./data/data.db chunk
	./main --db-path ./data/data.db embed --openai-url "http://127.0.0.1:11434" --model bge-m3 --dim 1024

serve-mcp: build
	#!/usr/bin/env bash
	set -euxo pipefail
	export RAG_CLI_API_TOKEN=super-super-sicher
	./main --db-path ./data/data.db serve mcp  --openai-url http://127.0.0.1:11434 --model bge-m3 --dim 1024

