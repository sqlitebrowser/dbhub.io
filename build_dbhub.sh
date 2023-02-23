#!/bin/sh

# Useful variables
DEST=${PWD}/local
export PKG_CONFIG_PATH=${DEST}/lib/pkgconfig
export GOBIN=${DEST}/bin

# If this script is passed an argument of "clean", then delete the
# locally compiled pieces
if [ "$1" = "clean" ]; then
  echo "Removing local SQLite and compiled DBHub.io executables"
  rm -rf ${DEST} other/cache
  exit
fi

# Builds a local SQLite
if [ ! -e "${DEST}/lib/libsqlite3.so" ]; then
  if [ ! -d "other/cache" ]; then
    mkdir -p other/cache
  fi
  cd other/cache
  if [ ! -f sqlite-autoconf-3400100.tar.gz ]; then
    echo "Downloading SQLite source code"
    curl -sOL https://sqlite.org/2022/sqlite-autoconf-3400100.tar.gz
  fi
  if [ ! -f sqlite-autoconf-3400100.tar.gz ]; then
    echo "Downloading the SQLite source code did not work"
    exit 1
  fi
  echo "Compiling local SQLite"
  tar xfz sqlite-autoconf-3400100.tar.gz
  cd sqlite-autoconf-3400100
  ./configure --prefix=${DEST} --enable-dynamic-extensions=no
  make -j9
  make install
  cd ..
  rm -rf sqlite-autoconf-3400100
  cd ../..
fi

# Compile JSX files and build webpack bundle
yarn
yarn run babel webui/jsx --out-dir webui/js --presets babel-preset-react-app/prod
yarn run webpack -c webui/webpack.config.js

# Builds the Go binaries
if [ -d "${GOBIN}" ]; then
  echo "Compiling DBHub.io API executable"
  cd api
  go install .
  cd ..
  echo "Compiling DBHub.io DB4S API executable"
  cd db4s
  go install .
  cd ..
  echo "Compiling DBHub.io web User Interface executable"
  cd webui
  go install .
  cd ..
fi
