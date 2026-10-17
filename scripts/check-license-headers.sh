#!/usr/bin/env bash
# check-license-headers.sh — Verifies all Go and .proto source files have the correct license header.
set -euo pipefail

PROTO_GEN_PATTERNS=("_grpc.pb.go" ".pb.go")
ERRORS=0

check_file() {
    local file="$1"

    # Read first two lines
    local line1 line2
    line1=$(head -1 "$file")
    line2=$(head -2 "$file" | tail -1)

    if [[ "$line1" != "// This file is part of Vassago." ]]; then
        echo "MISSING HEADER: $file"
        return 1
    fi

    if [[ "$line2" != "// See LICENSE for license information." ]]; then
        echo "WRONG LICENSE: $file (got: $line2)"
        return 1
    fi

    return 0
}

is_proto_gen() {
    local file="$1"
    for pattern in "${PROTO_GEN_PATTERNS[@]}"; do
        if [[ "$file" == *"$pattern"* ]]; then
            return 0
        fi
    done
    return 1
}

echo "Checking license headers..."

while IFS= read -r file; do
    # Skip protoc-generated Go files
    if is_proto_gen "$file"; then
        continue
    fi
    check_file "$file" || ((ERRORS++))
done < <(find . -name "*.go" -not -path "./.git/*" -not -path "*/vendor/*" -not -path "*/.venv/*" -not -path "*/node_modules/*" | sort)

# Check .proto files
while IFS= read -r file; do
    check_file "$file" || ((ERRORS++))
done < <(find . -name "*.proto" -not -path "./.git/*" -not -path "*/.venv/*" -not -path "*/node_modules/*" | sort)

if [ $ERRORS -gt 0 ]; then
    echo ""
    echo "FAIL: $ERRORS file(s) missing or wrong license header"
    exit 1
fi

echo "PASS: All source files have correct license headers"
