#!/bin/bash
set -e
cd "$(dirname "$0")"
source init.sh

if [[ "$1" == "-h" || "$1" == "-help" || "$1" == "--help" ]]; then
  # echo "usage: $0 [-debug] [<go-build-option> ...]" >&2
  echo "usage: $0 [-run] [<go-build-option> ...]" >&2
  echo "usage: $0 -h|-help" >&2
  exit 1
fi

run=false
if [[ "$1" == "-run" ]]; then
  run=true
  shift
fi

# DEBUG=false
# DEBUG_STR="false"
# if [[ "$1" == "-debug" ]]; then
#   shift
#   DEBUG=true
#   DEBUG_STR="true"
# fi

GITREV=$(git rev-parse --short=10 HEAD)
VERSION=$(cat version.txt)

# ghp
pushd ghp >/dev/null
echo "build ghp"
go build \
  -buildmode=exe \
  -ldflags="-X main.ghpVersion=$VERSION -X main.ghpVersionGit=$GITREV" \
  -pkgdir "$SRCDIR/gopath" \
  -o $SRCDIR/build/ghp \
  "$@"
popd >/dev/null

# pushd pub/example >/dev/null
# echo "build pub/example"
# go build \
#   -buildmode=plugin \
#   -o $SRCDIR/build/plugins/example.so
# popd >/dev/null

if $run; then
  echo ./build/ghp
  ./build/ghp
fi
