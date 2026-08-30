set dotenv-load

export OTEL_EXPORTER_OTLP_ENDPOINT := "http://127.0.0.1:4318"
export OTEL_SERVICE_NAME := "rag-cli-dev"


build:
	#!/usr/bin/env bash
	set -euxo pipefail
	CGO_ENABLED=1 go build --tags "fts5" -o ./main main.go
	chmod a+x ./main

cleanup:
	rm -f data/data.db*

extract input-dir collection-name collection-weight: build
	./main --db-path data/data.db extract -i {{input-dir}} --collection-name {{collection-name}} --collection-weight {{collection-weight}}

chunk: build
	./main chunk

embed: build
	./main embed

retrieve user-query: build
	./main retrieve -q '{{user-query}}' --dim 1024 --model bge-m3 --openai-url http://127.0.0.1:11434 --db-path ./data/data.db

inspect: build
	./main inspect

test:
      #!/usr/bin/env bash
      set -euxo pipefail
      CGO_ENABLED=1 go test --tags "fts5" ./...

lint:
	go vet ./...
	golangci-lint run -c golangci-lint.yaml

e2e input-dir collection-name collection-weight: build
	#!/usr/bin/env bash
	set -euxo pipefail
	./main --db-path ./data/data.db extract -i {{input-dir}} --docling-url "http://127.0.0.1:5001" --collection-name {{collection-name}} --collection-weight {{collection-weight}}
	./main --db-path ./data/data.db chunk
	./main --db-path ./data/data.db embed --openai-url "http://127.0.0.1:11434" --model bge-m3 --dim 1024

e2e-clear: build cleanup
	#!/usr/bin/env bash
	set -euxo pipefail
	./main --db-path ./data/data.db extract -i build/manual --docling-url "http://127.0.0.1:5001" --collection-weight 0.8  --collection-name manual
	./main --db-path ./data/data.db extract -i build/changelogs --docling-url "http://127.0.0.1:5001" --collection-weight 0.4  --collection-name changelogs
	./main --db-path ./data/data.db chunk
	./main --db-path ./data/data.db embed --openai-url "http://127.0.0.1:11434" --model bge-m3 --dim 1024

serve-mcp: build
	#!/usr/bin/env bash
	set -euxo pipefail
	export RAG_CLI_API_TOKEN=super-super-sicher
	./main --db-path ./data/data.db serve mcp  --openai-url http://127.0.0.1:11434 --model bge-m3 --dim 1024 --query-log ./data/query.log

