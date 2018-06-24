import subprocess
import os
import sys

class colors:
    '''Colors class:
    reset all colors with colors.reset
    two subclasses fg for foreground and bg for background.
    use as colors.subclass.colorname.
    i.e. colors.fg.red or colors.bg.green
    also, the generic bold, disable, underline, reverse, strikethrough,
    and invisible work with the main class
    i.e. colors.bold
    '''
    reset='\033[0m'
    bold='\033[01m'
    disable='\033[02m'
    underline='\033[04m'
    reverse='\033[07m'
    strikethrough='\033[09m'
    invisible='\033[08m'
    class fg:
        black='\033[30m'
        red='\033[31m'
        green='\033[32m'
        orange='\033[33m'
        blue='\033[34m'
        purple='\033[35m'
        cyan='\033[36m'
        lightgrey='\033[37m'
        darkgrey='\033[90m'
        lightred='\033[91m'
        lightgreen='\033[92m'
        yellow='\033[93m'
        lightblue='\033[94m'
        pink='\033[95m'
        lightcyan='\033[96m'
    class bg:
        black='\033[40m'
        red='\033[41m'
        green='\033[42m'
        orange='\033[43m'
        blue='\033[44m'
        purple='\033[45m'
        cyan='\033[46m'
        lightgrey='\033[47m'


def compile(path):
    fp = open(os.devnull, 'w')
    p = subprocess.Popen('go build', shell=True, cwd=path, stdout=fp, stderr=fp)
    p.wait()
    if p.returncode == 0:
        print(colors.fg.green + "Compiled " + path + colors.reset)
        return False
    else:
        print(colors.fg.red + "Compile failed " + path + colors.reset)
        return True

def run_test(name, cmd, cwd):
    fp = open(os.devnull, 'w')
    p = subprocess.Popen(cmd, shell=True, cwd=cwd, stdout=fp, stderr=fp)
    p.wait()

    if p.returncode == 0:
        print(colors.fg.green + name + " Passed." + colors.reset)
        return False
    else:
        print(colors.fg.red + name + " Failed." + colors.reset)
        return True


def main():
    error = False;

    error != compile('insts/gcn3disassembler')

    error |= compile('samples/firsim/')
    error |= run_test("FIR Disasm", '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff kernels.disasm -', 'samples/firsim')
    error |= run_test("FIR Emu", './firsim -verify -dataSize=8192', 'samples/firsim')
    error |= run_test("FIR Sim", './firsim -timing -verify -dataSize=8192', 'samples/firsim')
    error |= run_test("FIR Parallel Emu", './firsim -parallel -verify -dataSize=8192', 'samples/firsim')
    error |= run_test("FIR Parallel Sim", './firsim -timing -parallel -verify -dataSize=8192', 'samples/firsim')

    error |= compile('samples/kmeanssim/')
    error |= run_test("KMeans Disasm", '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff kernels.disasm -', 'samples/kmeanssim')
    error |= run_test("KMeans Emu", './kmeanssim -verify -points=1024 -features=32 -clusters=5', 'samples/kmeanssim')
    error |= run_test("KMeans Sim", './kmeanssim -timing -verify -points=1024 -features=32 -clusters=5', 'samples/kmeanssim')
    error |= run_test("KMeans Parallel Emu", './kmeanssim -parallel -verify -points=1024 -features=32 -clusters=5', 'samples/kmeanssim')
    error |= run_test("KMeans Parallel Sim", './kmeanssim -timing -parallel -verify -points=1024 -features=32 -clusters=5', 'samples/kmeanssim')

    error |= compile('samples/matrixtranspose/')
    error |= run_test("MatrixTranspose Disasm", '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff kernels.disasm -', 'samples/matrixtranspose')
    error |= run_test("MatrixTranspose Emu", './matrixtranspose -verify -width=256', 'samples/matrixtranspose')
    error |= run_test("MatrixTranspose Sim", './matrixtranspose -timing -verify -width=256', 'samples/matrixtranspose')
    error |= run_test("MatrixTranspose Parallel Emu", './matrixtranspose --parallel -verify -width=256', 'samples/matrixtranspose')
    error |= run_test("MatrixTranspose Parallel Sim", './matrixtranspose -timing --parallel -verify -width=256', 'samples/matrixtranspose')

    error |= compile('samples/bitonicsort/')
    # error |= run_test("BitonicSort Disasm", '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff kernels.disasm -', 'samples/bitonicsort')
    error |= run_test("BitonicSort Emu", './bitonicsort -verify', 'samples/bitonicsort')
    error |= run_test("BitonicSort Sim", './bitonicsort -timing -verify', 'samples/bitonicsort')
    error |= run_test("BitonicSort Parallel Emu", './bitonicsort --parallel -verify', 'samples/bitonicsort')
    error |= run_test("BitonicSort Parallel Sim", './bitonicsort -timing --parallel -verify', 'samples/bitonicsort')

    if error:
        sys.exit(1)

if __name__ == '__main__':
    main()
