#!/bin/bash
SRCDIR_REL=$(dirname "${BASH_SOURCE[0]}")

# BUILD_DIR=$SRCDIR_REL/build
# if [[ "${BUILD_DIR:0:2}" == "./" ]]; then
#   BUILD_DIR=${BUILD_DIR:2}
# fi

pushd "$SRCDIR_REL" >/dev/null
SRCDIR=$(pwd)
popd >/dev/null

if [[ ":$GOPATH:" != *":$SRCDIR/gopath:"* ]]; then
  export GOPATH=$SRCDIR/gopath:$GOPATH
  # export GOBIN=$GOPATH/bin
  if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    echo "Source this script to setup the environment:" >&2
    if [ "$0" == "./init.sh" ]; then
      # pretty format for common case
      echo "  source init.sh"
    else
      echo "  source '$0'"
    fi
  fi
fi

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Subshell
  set -e
  cd "$SRCDIR_REL"

  pushd ghp
  go get -d -v .
  popd

  echo "DONE."
# else
#   # sourced
fi
