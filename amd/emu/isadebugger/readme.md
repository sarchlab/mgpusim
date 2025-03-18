# ISA Debugger

ISA Debugger is a tool that can facilitate debugging instruction emulation in MGPUSim.

## How to use the tool

1. Run a MGPUSim emulation. Make sure you run without the `-timing` option and with the `-debug-isa` option. The emulation will be slower as it dumps the execution traces for each instruction.
2. Locate a generated `.debug` file, copy it to this folder and rename is as `isa.debug.json`.
3. Start an http server. I am using `python3 -m http.server [port_number]`
4. Open your browser and type in `localhost:[port_number]`
5. Click on the `Next` and `Prev` button to check the register state after executing each instruction.

## Compile

We commit the compiled javascript as part of the delivery. So you do not need to compile it if you just want to run the tool. In case you need to modify the TypeScript file, you need to compile it. First of all, you need to install the TypeScript compiler to be able to compile the code. Assuming you have the `tsc` executable in your path, run `make` to compile the typescript file into the javascript file.
