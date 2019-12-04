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


class Test(object):
    """ define a benchmark to test　"""

    def __init__(self, path, executable, size_args, benchmark_path):
        self.path = path
        self.executable = executable
        self.size_args = size_args
        self.benchmark_path = benchmark_path

    def test(self,
             test_disassemble="true",
             test_unified_multi_gpu="true",
             test_multi_gpu="true",
             ):
        err = False

        if test_disassemble:
            err |= self.test_disassemble()

        err |= self.compile()

        err |= self.run_test(False, False, False, '1')
        err |= self.run_test(False, True, False, '1')
        err |= self.run_test(True, False, False, '1')
        err |= self.run_test(True, True, False, '1')

        if test_unified_multi_gpu:
            err |= self.run_test(False, False, True, '1,2')
            err |= self.run_test(False, False, True, '1,2,3,4')
            err |= self.run_test(False, True, True, '1,2')
            err |= self.run_test(False, True, True, '1,2,3,4')
            err |= self.run_test(True, False, True, '1,2')
            err |= self.run_test(True, False, True, '1,2,3,4')
            err |= self.run_test(True, True, True, '1,2')
            err |= self.run_test(True, True, True, '1,2,3,4')

        if test_multi_gpu:
            err |= self.run_test(False, False, False, '1,2')
            err |= self.run_test(False, False, False, '1,2,3,4')
            err |= self.run_test(False, True, False, '1,2')
            err |= self.run_test(False, True, False, '1,2,3,4')
            err |= self.run_test(True, False, False, '1,2')
            err |= self.run_test(True, False, False, '1,2,3,4')
            err |= self.run_test(True, True, False, '1,2')
            err |= self.run_test(True, True, False, '1,2,3,4')

        return err

    def compile(self):
        fp = open(os.devnull, 'w')
        p = subprocess.Popen('go build', shell=True,
                             cwd=self.path, stdout=fp, stderr=fp)
        p.wait()
        if p.returncode == 0:
            print(colors.fg.green + "Compiled " + self.path + colors.reset)
            return False
        else:
            print(colors.fg.red + "Compile failed " + self.path + colors.reset)
            return True

    def run_test(self, timing, parallel, unified_multi_gpu, gpus):
        fp = open(os.devnull, 'w')
        cmd = ['./'+self.executable, '-verify']
        cmd.extend(self.size_args)

        if unified_multi_gpu:
            cmd.append('-unified-gpus='+gpus)
        else:
            cmd.append('-gpus='+gpus)

        if timing:
            cmd.append('-timing')

        if parallel:
            cmd.append('-parallel')

        cmd_string = 'cd ' + self.path + ' && ' + ' '.join(cmd)
        print('Running ' + cmd_string)

        p = subprocess.Popen(cmd, shell=False,
                             cwd=self.path,
                             stdout=fp, stderr=fp
                             )
        p.wait()

        if p.returncode == 0:
            print(colors.fg.green + 'Passed.' + colors.reset)
            return False
        else:
            print(colors.fg.red + ' Failed.' + colors.reset)
            return True

    def test_disassemble(self):
        output_filename = self.benchmark_path + '/disasm.disasm'
        fp = open(output_filename, 'w')
        cmd = ['../../insts/gcn3disassembler/gcn3disassembler',
               self.benchmark_path + '/kernels.hsaco']

        cmd_string = ' '.join(cmd)
        print('Running ' + cmd_string + ' > ' +
              output_filename + ' 2>&1 ' + ' ...')

        p = subprocess.Popen(cmd, shell=False,
                             stdout=fp, stderr=fp
                             )
        p.wait()
        if p.returncode != 0:
            print(colors.fg.red + ' Failed.' + colors.reset)
            return True

        fp = open(self.benchmark_path + '/diff.debug', 'w')
        cmd = ['diff', 'kernels.disasm', 'disasm.disasm']
        p = subprocess.Popen(cmd, shell=False,
                             cwd=self.benchmark_path,
                             stdout=fp, stderr=fp)
        p.wait()

        if p.returncode == 0:
            print(colors.fg.green + 'Passed.' + colors.reset)
            return False
        else:
            print(colors.fg.red + ' Failed.' + colors.reset)
            return True


def main():

    atax = Test('../../samples/atax',
                'atax',
                ['-x=256', '-y=256'],
                '../../benchmarks/polybench/atax'
                )
    bicg = Test('../../samples/bicg',
                'bicg',
                ['-x=256', '-y=256'],
                '../../benchmarks/polybench/bicg'
                )
    fir = Test('../../samples/fir',
               'fir',
               ['-length=8192'],
               '../../benchmarks/heteromark/fir')
    aes = Test('../../samples/aes',
               'aes',
               ['-length=16384'],
               '../../benchmarks/heteromark/aes')
    km = Test('../../samples/kmeans', 'kmeans',
              [
                  '-points=1024',
                  '-features=32',
                  '-clusters=5',
                  '-max-iter=5'
              ],
              '../../benchmarks/heteromark/kmeans')
    pagerank = Test('../../samples/pagerank', 'pagerank',
                    [
                        '-node=64',
                        '-sparsity=0.5',
                        '-iterations=2',
                    ],
                    '../../benchmarks/heteromark/pagerank')
    mm = Test('../../samples/matrixmultiplication',
              'matrixmultiplication',
              ['-x=128', '-y=128', '-z=128'],
              '../../benchmarks/amdappsdk/matrixmultiplication')
    mt = Test('../../samples/matrixtranspose',
              'matrixtranspose',
              ['-width=256'],
              '../../benchmarks/amdappsdk/matrixtranspose')
    bs = Test('../../samples/bitonicsort',
              'bitonicsort',
              ['-length=4096'],
              '../../benchmarks/amdappsdk/bitonicsort')
    sc = Test('../../samples/simpleconvolution',
              'simpleconvolution',
              [],
              '../../benchmarks/amdappsdk/simpleconvolution')
    fw = Test('../../samples/floydwarshall',
              'floydwarshall',
              [],
              '../../benchmarks/amdappsdk/floydwarshall')
    re = Test('../../samples/relu',
              'relu',
              [],
              '../../benchmarks/dnn/relu')
    mp = Test('../../samples/maxpooling',
              'maxpooling',
              [],
              '../../benchmarks/dnn/maxpooling')
    bfs = Test('../../samples/bfs',
               'bfs',
               ['-node=1024'],
               '../../benchmarks/shoc/bfs')
    st = Test('../../samples/stencil2d',
              'stencil2d',
              [],
              '../../benchmarks/shoc/stencil2d')
    cw = Test('../../samples/concurrentworkload',
              'concurrentworkload',
              [],
              '')

    err = False
    err |= compile('../../insts/gcn3disassembler')
    err |= atax.test(test_multi_gpu=False)
    err |= bicg.test(test_multi_gpu=False)
    err |= aes.test()
    err |= fir.test()
    err |= km.test()
    err |= pagerank.test()
    err |= mm.test()
    err |= mt.test()
    err |= bs.test()
    err |= sc.test()
    err |= fw.test(test_multi_gpu=False)
    err |= re.test()
    err |= mp.test()
    err |= bfs.test(test_multi_gpu=False)
    err |= st.test(test_multi_gpu=False)

    err |= cw.test(test_disassemble=False,
                   test_unified_multi_gpu=False,
                   test_multi_gpu=False)

    # error |= compile('acceptancetests/cupipelinedraining')
    # error |= run_test('CU Pipeline Draining',
    #                   './cupipelinedraining -timing',
    #                   'acceptancetests/cupipelinedraining')
    # error |= run_test('CU Pipeline Draining Parallel',
    #                   './cupipelinedraining -timing -parallel',
    #                   'acceptancetests/cupipelinedraining')

    # error |= compile('acceptancetests/tlbshootown')
    # error |= run_test('TLB Shootdown',
    #                   './tlbshootdown -timing',
    #                   'acceptancetestes/tlbshootdown')

    if err:
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
