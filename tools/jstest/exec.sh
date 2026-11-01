#!/bin/sh
# Runs a GOOS=js test binary under node with a stub DOM, so packages that
# import Ebitengine (cmd/hexes) can run plain-Go tests like the editor
# layout check. Use: go test -exec=$PWD/tools/jstest/exec.sh ./cmd/hexes
here=$(cd "$(dirname "$0")" && pwd)
exec node -r "$here/fakedom.js" "$(go env GOROOT)/lib/wasm/wasm_exec_node.js" "$@"
