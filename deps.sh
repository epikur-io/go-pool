#!/usr/bin/env bash

GO_BIN_PATH="$(go env GOPATH)/bin"
if [[ ":$PATH:" != *":$GO_BIN_PATH:"* ]]; then
  echo "$GO_BIN_PATH is not in your \$PATH variable"
  exit 1
fi

go install github.com/go-task/task/v3/cmd/task@latest
