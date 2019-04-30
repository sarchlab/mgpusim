import subprocess
import os
import sys


def compile(path):
    fp = open(os.devnull, 'w')
    p = subprocess.Popen('go build', shell=True,
                         cwd=path, stdout=fp, stderr=fp)
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
    error = False

    error |= compile('insts/gcn3disassembler')



    error |= compile('samples/fir/')
    # error |= run_test("FIR Disasm", '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff kernels.disasm -', 'samples/fir')
    error |= run_test(
        "FIR Emu",
        './fir -verify -length=8192',
        'samples/fir')
    error |= run_test(
        "FIR Sim",
        './fir -timing -verify -length=8192',
        'samples/fir')
    error |= run_test(
        "FIR Parallel Emu",
        './fir -parallel -verify -length=8192',
        'samples/fir')
    error |= run_test(
        "FIR Parallel Sim",
        './fir -timing -parallel -verify -length=8192',
        'samples/fir')

    error |= compile('samples/kmeans/')
    # error |= run_test("KMeans Disasm", '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff kernels.disasm -', 'samples/kmeans')
    error |= run_test(
        "KMeans Emu",
        './kmeans -verify -points=1024 -features=32 -clusters=5 -max-iter=5',
        'samples/kmeans')
    error |= run_test(
        "KMeans Sim",
        './kmeans -timing -verify -points=1024 -features=32 -clusters=5 -max-iter=5',
        'samples/kmeans')
    error |= run_test(
        "KMeans Parallel Emu",
        './kmeans -parallel -verify -points=1024 -features=32 -clusters=5 -max-iter=5',
        'samples/kmeans')
    error |= run_test(
        "KMeans Parallel Sim",
        './kmeans -timing -parallel -verify -points=1024 -features=32 -clusters=5 -max-iter=5',
        'samples/kmeans')

    error |= compile('samples/matrixtranspose/')
    # error |= run_test("MatrixTranspose Disasm", '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff kernels.disasm -', 'samples/matrixtranspose')
    error |= run_test(
        "MatrixTranspose Emu",
        './matrixtranspose -verify -width=256',
        'samples/matrixtranspose')
    error |= run_test(
        "MatrixTranspose Sim",
        './matrixtranspose -timing -verify -width=256',
        'samples/matrixtranspose')
    error |= run_test(
        "MatrixTranspose Parallel Emu",
        './matrixtranspose --parallel -verify -width=256',
        'samples/matrixtranspose')
    error |= run_test(
        "MatrixTranspose Parallel Sim",
        './matrixtranspose -timing --parallel -verify -width=256',
        'samples/matrixtranspose')

    error |= compile('samples/bitonicsort/')
    # error |= run_test("BitonicSort Disasm", '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff kernels.disasm -', 'samples/bitonicsort')
    error |= run_test(
        "BitonicSort Emu",
        './bitonicsort -length=4096 -verify',
        'samples/bitonicsort')
    error |= run_test(
        "BitonicSort Sim",
        './bitonicsort -length=4096 -timing -verify',
        'samples/bitonicsort')
    error |= run_test(
        "BitonicSort Parallel Emu",
        './bitonicsort -length=4096 -parallel -verify',
        'samples/bitonicsort')
    error |= run_test(
        "BitonicSort Parallel Sim",
        './bitonicsort -length=4096 -timing -parallel -verify',
        'samples/bitonicsort')

    error |= compile('samples/aes/')
    # # error |= run_test("AES Disasm",
    # # '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff
    # # kernels.disasm -', 'samples/aes')
    error |= run_test("AES Emu", './aes -verify', 'samples/aes')
    error |= run_test("AES Sim", './aes -timing -verify', 'samples/aes')
    error |= run_test("AES Parallel Emu",
                      './aes --parallel -verify', 'samples/aes')
    error |= run_test("AES Parallel Sim",
                      './aes -timing --parallel -verify', 'samples/aes')

    error |= compile('samples/simpleconvolution/')
    # # error |= run_test("AES Disasm",
    # # '../../insts/gcn3disassembler/gcn3disassembler kernels.hsaco | diff
    # # kernels.disasm -', 'samples/aes')
    error |= run_test(
        "Simple Convolution Emu",
        './simpleconvolution -verify',
        'samples/simpleconvolution')
    error |= run_test(
        "Simple Convolution Sim",
        './simpleconvolution -timing -verify',
        'samples/simpleconvolution')
    error |= run_test(
        "Simple Convolution Parallel Emu",
        './simpleconvolution --parallel -verify',
        'samples/simpleconvolution')
    error |= run_test(
        "Simple Convolution Parallel Sim",
        './simpleconvolution -timing --parallel -verify',
        'samples/simpleconvolution')

    error |= compile('samples/relu/')
    error |= run_test(
        "Relu Emu",
        './relu -verify',
        'samples/relu')
    error |= run_test(
        "Relu Sim",
        './relu -timing -verify',
        'samples/relu')
    error |= run_test(
        "Relu Parallel Emu",
        './relu -parallel -verify',
        'samples/relu')
    error |= run_test(
        "Relu Parallel Sim",
        './relu -timing -parallel -verify',
        'samples/relu')

    error |= compile('samples/maxpooling/')
    error |= run_test(
        "MaxPooling Emu",
        './maxpooling -verify',
        'samples/maxpooling')
    error |= run_test(
        "MaxPooling Sim",
        './maxpooling -timing -verify',
        'samples/maxpooling')
    error |= run_test(
        "MaxPooling Parallel Emu",
        './maxpooling -parallel -verify',
        'samples/maxpooling')
    error |= run_test(
        "MaxPooling Parallel Sim",
        './maxpooling -timing -parallel -verify',
        'samples/maxpooling')

    error |= compile('samples/matrixmultiplication/')
    error |= run_test(
        "Matrix Multiplication Emu",
        './matrixmultiplication -length=256 -verify',
        'samples/matrixmultiplication')
    error |= run_test(
        "Matrix Multiplication Sim",
        './matrixmultiplication -length=256 -timing -verify',
        'samples/matrixmultiplication')
    error |= run_test(
        "Matrix Multiplication Parallel Emu",
        './matrixmultiplication -length=256 -parallel -verify',
        'samples/matrixmultiplication')
    error |= run_test(
        "Matrix Multiplication Parallel Sim",
        './matrixmultiplication -length=256 -timing -parallel -verify',
        'samples/matrixmultiplication')

    error |= compile('samples/concurrentworkload/')
    error |= run_test(
        "Concurrent Workload Sim",
        './concurrentworkload -timing -verify',
        'samples/concurrentworkload')
    error |= run_test(
        "Concurrent Workload Sim",
        './concurrentworkload -timing -parallel -verify',
        'samples/concurrentworkload')


    error |= compile('acceptancetests/cupipelinedraining')
    error |= run_test('CU Pipeline Draining',
                      './cupipelinedraining -timing',
                      'acceptancetests/cupipelinedraining')
    error |= run_test('CU Pipeline Draining Parallel',
                      './cupipelinedraining -timing -parallel',
                      'acceptancetests/cupipelinedraining')

    # error |= compile('acceptancetests/tlbshootown')
    # error |= run_test('TLB Shootdown',
    #                   './tlbshootdown -timing',
    #                   'acceptancetestes/tlbshootdown')
    if error:
        sys.exit(1)


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
    reset = '\033[0m'
    bold = '\033[01m'
    disable = '\033[02m'
    underline = '\033[04m'
    reverse = '\033[07m'
    strikethrough = '\033[09m'
    invisible = '\033[08m'

    class fg:
        black = '\033[30m'
        red = '\033[31m'
        green = '\033[32m'
        orange = '\033[33m'
        blue = '\033[34m'
        purple = '\033[35m'
        cyan = '\033[36m'
        lightgrey = '\033[37m'
        darkgrey = '\033[90m'
        lightred = '\033[91m'
        lightgreen = '\033[92m'
        yellow = '\033[93m'
        lightblue = '\033[94m'
        pink = '\033[95m'
        lightcyan = '\033[96m'

    class bg:
        black = '\033[40m'
        red = '\033[41m'
        green = '\033[42m'
        orange = '\033[43m'
        blue = '\033[44m'
        purple = '\033[45m'
        cyan = '\033[46m'
        lightgrey = '\033[47m'


if __name__ == '__main__':
    main()
