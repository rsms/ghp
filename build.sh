#!/bin/bash
set -e
cd "$(dirname "$0")"
source scripts/util.sh
SRCDIR=$(pwd)

OPT_HELP=false
OPT_NOGET=false
OPT_GOUPDATE=false
OPT_FORCE=false

# parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
  -h|-help|--help)
    OPT_HELP=true
    shift
    ;;
  -noget)
    OPT_NOGET=true
    shift
    ;;
  -force)
    OPT_FORCE=true
    shift
    ;;
  -update-go)
    OPT_GOUPDATE=true
    shift
    ;;
  *)
    echo "$0: Unknown option $1" >&2
    OPT_HELP=true
    shift
    ;;
  esac
done
if $OPT_HELP; then
  echo "usage: $0 [options]" >&2
  echo "options:" >&2
  echo "  -h, -help    Show help" >&2
  echo "  -noget       Do NOT fetch or update dependencies" >&2
  echo "  -force       Build even if products are up-to-date" >&2
  echo "  -update-go   Fetch and install go (even if it's already up to date)" >&2
  exit 1
fi

BUILDTAG=src
if which git >/dev/null && [[ -d .git ]]; then
  BUILDTAG=$(git rev-parse --short=10 HEAD)
fi
VERSION=$(cat version.txt)

if [[ -z $GHP_GO_VERSION ]]; then
  export GHP_GO_VERSION=1.11.2
fi

export GOROOT=$SRCDIR/go
export GOPATH=$SRCDIR/gopath
export PATH=$GOROOT/bin:$PATH


# needs go?
if ! $OPT_GOUPDATE; then
  if [[ ! -d "$GOROOT" ]] || [[ ! -f "$GOROOT/VERSION" ]]; then
    echo "$GOROOT is missing" >&2
    OPT_GOUPDATE=true
  elif [[ "$(cat $GOROOT/VERSION)" != "go$GHP_GO_VERSION" ]]; then
    echo "$GOROOT is out of date (has $(cat $GOROOT/VERSION); wants go$GHP_GO_VERSION)" >&2
    OPT_GOUPDATE=true
  fi
fi

if $OPT_GOUPDATE; then
  FROMSOURCE=false
  if [[ -z $GOOS ]] || [[ "$GOOS" == "" ]] || [[ -z $GOARCH ]] || [[ "$GOARCH" == "" ]]; then
    shopt -s nocasematch
    case $(uname) in
    # see https://github.com/golang/go/blob/master/src/go/build/syslist.go
    *darwin*)
      export GOOS=darwin
      export GOARCH=amd64  # note: go only supports amd64 on mac
      ;;
    *linux*)
      # Linux ip-10-0-0-181 4.15.0 #21-Ubuntu x86_64 x86_64 x86_64 GNU/Linux
      export GOOS=linux
      case $(uname -i) in
        *x86_64*)
          export GOARCH=amd64
          ;;
        *i386*)
          export GOARCH=386
          ;;
        *)
      esac
      ;;
    *)
      echo "Unable to infer system. Fetching and building go from source."
      FROMSOURCE=true
      ;;
    esac
  fi

  # Fetch binary build, e.g.
  # https://dl.google.com/go/go1.11.2.linux-amd64.tar.gz
  # https://dl.google.com/go/go1.11.2.linux-386.tar.gz
  #
  GOAR_URL=https://dl.google.com/go/go$GHP_GO_VERSION.$GOOS-$GOARCH.tar.gz
  if $FROMSOURCE; then
    # Oh well, fetch source dist and we'll compile it
    GOAR_URL=https://dl.google.com/go/go$GHP_GO_VERSION.src.tar.gz
  fi

  # temporary directory
  rm -rf .go-tmp
  mkdir .go-tmp
  pushd .go-tmp >/dev/null

  echo "Fetching $GOAR_URL -> $PWD"
  curl '-#' -o go.tar.gz "$GOAR_URL"
  tar -xzf go.tar.gz
  rm go.tar.gz

  if $FROMSOURCE; then
    echo "Building go at $PWD/go"
    pushd go/src >/dev/null
    bash all.bash
    popd >/dev/null
  fi

  # test
  cat << 'EOF' > test.go
package main
import "fmt"
func main() {
  fmt.Printf("test ok\n")
}
EOF
  GOROOT=$PWD/go PATH=$PWD/go/bin:$PATH go build -o test
  ./test

  # clean up go directory
  pushd go/src >/dev/null
  rm -rf test misc doc api favicon.ico
  popd >/dev/null

  # replace ghp/go
  rm -rf ../go
  mv go ../go

  # exit temp dir and remove it
  popd >/dev/null
  rm -rf .go-tmp
fi


GHP_PROG=$SRCDIR/bin/ghp

# Decide if we want to be lazy and skip rebuilding if product is
# newer than source.
PRODUCT_OUTDATED=true
if ! $OPT_FORCE && \
   ! $OPT_GOUPDATE && \
   ! has_newer "$GHP_PROG" ghp '*.go' && \
   ! has_newer "$GHP_PROG" . '*.go'
then
  PRODUCT_OUTDATED=false
fi


if ! $PRODUCT_OUTDATED; then
  echo "$GHP_PROG is up to date -- build not required"
else

  pushd ghp >/dev/null

  if ! $OPT_NOGET; then
    echo "go get ."
    go get -d -v .
  fi


  echo "go build $GHP_PROG.tmp"
  go build \
    -buildmode=exe \
    -ldflags="-X main.ghpVersion=$VERSION -X main.ghpBuildTag=$BUILDTAG" \
    -pkgdir "$SRCDIR/gopath" \
    -o "$GHP_PROG.tmp"
  
  # CGO_ENABLED=0 go build \
  #   -buildmode=exe \
  #   -installsuffix cgo \
  #   -ldflags="-X main.ghpVersion=$VERSION -X main.ghpBuildTag=$BUILDTAG" \
  #   -pkgdir "$SRCDIR/gopath" \
  #   -o "$GHP_PROG.tmp"

  popd >/dev/null

  mv -vf "$GHP_PROG.tmp" "$GHP_PROG"

fi

