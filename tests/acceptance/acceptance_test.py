import subprocess
import os
import sys
import argparse
from termcolor import cprint


def compile(path):
    fp = open(os.devnull, 'w')
    p = subprocess.Popen('go build', shell=True,
                         cwd=path, stdout=fp, stderr=fp)
    p.wait()
    if p.returncode == 0:
        cprint("Compiled " + path, 'green')
        return False
    else:
        cprint("Compile failed " + path, 'red')
        return True


class Test(object):
    """ define a benchmark to test　"""

    def __init__(self, path, executable, size_args, benchmark_path):
        self.path = path
        self.executable = executable
        self.size_args = size_args
        self.benchmark_path = benchmark_path

    def test(self,
             test_disassemble=False,
             test_unified_multi_gpu=False,
             test_multi_gpu=False,
             use_unified_memory=False,
             ):
        err = False

        if test_disassemble:
            err |= self.test_disassemble()
            return err

        err |= self.compile()

        if test_unified_multi_gpu:
            err |= self.run_test(True, False, True,
                                 use_unified_memory, '1,2')
            err |= self.run_test(
                True, False, True,
                use_unified_memory, '1,2,3,4')
            err |= self.run_test(True, True, True,
                                 use_unified_memory, '1,2')
            err |= self.run_test(True, True, True,
                                 use_unified_memory, '1,2,3,4')
        elif test_multi_gpu:
            if not use_unified_memory:
                err |= self.run_test(False, False, False,
                                     use_unified_memory, '1,2')
                err |= self.run_test(False, False, False,
                                     use_unified_memory, '1,2,3,4')
                err |= self.run_test(False, True, False,
                                     use_unified_memory, '1,2')
                err |= self.run_test(False, True, False,
                                     use_unified_memory, '1,2,3,4')
            err |= self.run_test(True, False, False,
                                 use_unified_memory, '1,2')
            err |= self.run_test(True, False, False,
                                 use_unified_memory, '1,2,3,4')
            err |= self.run_test(True, True, False,
                                 use_unified_memory, '1,2')
            err |= self.run_test(True, True, False,
                                 use_unified_memory, '1,2,3,4')
        else:
            err |= self.run_test(False, False, False, use_unified_memory, '1')
            err |= self.run_test(False, True, False, use_unified_memory, '1')
            err |= self.run_test(True, False, False, use_unified_memory, '1')
            err |= self.run_test(True, True, False, use_unified_memory, '1')

        return err

    def compile(self):
        fp = open(os.devnull, 'w')
        p = subprocess.Popen('go build', shell=True,
                             cwd=self.path, stdout=fp, stderr=fp)
        p.wait()
        if p.returncode == 0:
            cprint("Compiled " + self.path, 'green')
            return False
        else:
            cprint("Compile failed " + self.path, 'red')
            return True

    def run_test(self,
                 timing, parallel,
                 unified_multi_gpu,
                 unified_memory,
                 gpus):
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

        if unified_memory:
            cmd.append('-use-unified-memory')

        cmd_string = 'cd ' + self.path + ' && ' + ' '.join(cmd)
        print('Running ' + cmd_string)

        p = subprocess.Popen(cmd, shell=False,
                             cwd=self.path,
                             stdout=fp, stderr=fp
                             )
        p.wait()

        if p.returncode == 0:
            cprint('Passed.', 'green')
            return False
        else:
            cprint('Failed.', 'red')
            return True

    def test_disassemble(self):
        output_filename = self.benchmark_path + '/disasm.disasm'
        fp = open(output_filename, 'w')
        cmd = ['../../insts/gcn3disassembler/gcn3disassembler',
               self.benchmark_path + '/kernels.hsaco']

        cmd_string = ' '.join(cmd)
        print('Running ' + cmd_string + ' > ' +
              output_filename + ' 2>&1 ' + ' ')

        p = subprocess.Popen(cmd, shell=False,
                             stdout=fp, stderr=fp
                             )
        p.wait()
        if p.returncode != 0:
            cprint(' Failed.', 'red')
            return True

        fp = open(self.benchmark_path + '/diff.debug', 'w')
        cmd = ['diff', 'kernels.disasm', 'disasm.disasm']
        p = subprocess.Popen(cmd, shell=False,
                             cwd=self.benchmark_path,
                             stdout=fp, stderr=fp)
        p.wait()

        if p.returncode == 0:
            cprint('Passed.', 'green')
            return False
        else:
            cprint('Failed.', 'red')
            return True


def parseArgs():
    parser = argparse.ArgumentParser()
    parser.add_argument("--unified-multi-gpu",
                        help="Run unified multi-GPU tests",
                        action="store_true")
    parser.add_argument("--discrete-multi-gpu",
                        help="Run discrete multi-GPU tests",
                        action="store_true")
    parser.add_argument("--unified-memory",
                        help="Use unified memory",
                        action="store_true")
    args = parser.parse_args()
    return args


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

    args = parseArgs()

    err = False

    if args.unified_multi_gpu:
        err |= atax.test(test_unified_multi_gpu=True,
                         use_unified_memory=args.unified_memory)
        err |= bicg.test(test_unified_multi_gpu=True,
                         use_unified_memory=args.unified_memory)
        err |= aes.test(test_unified_multi_gpu=True,
                        use_unified_memory=args.unified_memory)
        err |= fir.test(test_unified_multi_gpu=True,
                        use_unified_memory=args.unified_memory)
        err |= km.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= pagerank.test(test_unified_multi_gpu=True,
                             use_unified_memory=args.unified_memory)
        err |= mm.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= mt.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= bs.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= sc.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= fw.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= re.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= mp.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= bfs.test(test_unified_multi_gpu=True,
                        use_unified_memory=args.unified_memory)
        err |= st.test(test_unified_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
    elif args.discrete_multi_gpu:
        err |= aes.test(test_multi_gpu=True,
                        use_unified_memory=args.unified_memory)
        err |= fir.test(test_multi_gpu=True,
                        use_unified_memory=args.unified_memory)
        err |= km.test(test_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= pagerank.test(test_multi_gpu=True,
                             use_unified_memory=args.unified_memory)
        err |= mm.test(test_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= mt.test(test_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= bs.test(test_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= sc.test(test_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= re.test(test_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
        err |= mp.test(test_multi_gpu=True,
                       use_unified_memory=args.unified_memory)
    else:
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


if __name__ == '__main__':
    main()
