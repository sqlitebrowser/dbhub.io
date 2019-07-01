This is just simple research code, to determine if the current
state of TinyGo wasm can usefully present visualisation data for
DBHub.io purposes.

To compile the WebAssembly file:

    $ tinygo build -target wasm -no-debug -o barchart.wasm barchart.go

To strip the custom name section from the end (reducing file size
further):

    $ wasm2wat barchart.wasm -o barchart.wat
    $ wat2wasm barchart.wat -o barchart.wasm
