#!/usr/bin/env bash

set -euo pipefail

file=$1

base=$(basename -- "$file" .html)
echo "converting $base"
weasyprint $file "${base}.pdf"
curl -X POST http://localhost:5001/v1/convert/file -F "files=@${base}.pdf" -F "to_formats=json" | jq > "${base}_extracted.json"
