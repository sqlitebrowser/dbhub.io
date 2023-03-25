#!/bin/sh

# TODO: Consider adding the same extensions to the SQLite compile here, that we use
#       in our macOS, AppImage (etc) builds.  Then add the function names to the allowed
#       list for the API Query() function

# Useful variables
DEST=${PWD}/local
export PKG_CONFIG_PATH=${DEST}/lib/pkgconfig
export GOBIN=${DEST}/bin

# Load NVM (shouldn't be needed, but is. Ugh.)
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"

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
  cd other/cache  || exit 1
  if [ ! -f sqlite.tar.gz ]; then
    echo "Downloading SQLite source code"
    TARBALL=$(curl -s https://sqlite.org/download.html | awk '/<!--/,/-->/ {print}' | grep 'sqlite-autoconf' | cut -d ',' -f 3)
    SHA3=$(curl -s https://sqlite.org/download.html | awk '/<!--/,/-->/ {print}' | grep 'sqlite-autoconf' | cut -d ',' -f 5)
    curl -LsS -o sqlite.tar.gz https://sqlite.org/${TARBALL}
    VERIFY=$(openssl dgst -sha3-256 sqlite.tar.gz | cut -d ' ' -f 2)
    if [ "$SHA3" != "$VERIFY" ]; then exit 2 ; fi
  fi
  if [ ! -f sqlite.tar.gz ]; then
    echo "Downloading the SQLite source code did not work"
    exit 3
  fi
  echo "Compiling local SQLite"
  tar xfz sqlite.tar.gz
  cd sqlite-autoconf-* || exit 4
  ./configure --prefix=${DEST} --enable-dynamic-extensions=no
  make -j9
  make install
  cd ..
  rm -rf sqlite-autoconf-*
  cd ../..
fi

# Compile JSX files and build webpack bundle
yarn
yarn run babel webui/jsx --out-dir webui/js --presets babel-preset-react-app/prod
yarn run webpack -c webui/webpack.config.js

# Builds the Go binaries
if [ -d "${GOBIN}" ]; then
  echo "Compiling DBHub.io API executable"
  cd api || exit 5
  go install .
  cd ..
  echo "Compiling DBHub.io DB4S end point executable"
  cd db4s || exit 6
  go install .
  cd ..
  echo "Compiling DBHub.io Live executable"
  cd live || exit 7
  go install .
  cd ..
  echo "Compiling DBHub.io Web User Interface executable"
  cd webui || exit 8
  go install .
  cd ..
fi
