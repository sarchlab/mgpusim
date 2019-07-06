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

    def __init__(self, path, executable, size_args):
        self.path = path
        self.executable = executable
        self.size_args = size_args

    def test(self):
        err = False
        err |= self.compile()
        err |= self.run_test(False, False, '1')
        err |= self.run_test(False, False, '1,2')
        err |= self.run_test(False, False, '1,2,3,4')
        err |= self.run_test(False, True, '1')
        err |= self.run_test(False, True, '1,2')
        err |= self.run_test(False, True, '1,2,3,4')
        err |= self.run_test(True, False, '1')
        err |= self.run_test(True, False, '1,2')
        err |= self.run_test(True, False, '1,2,3,4')
        err |= self.run_test(True, True, '1')
        err |= self.run_test(True, True, '1,2')
        err |= self.run_test(True, True, '1,2,3,4')
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

    def run_test(self, timing, parallel, gpus):
        fp = open(os.devnull, 'w')
        cmd = ['./'+self.executable, '-verify', '-gpus='+gpus]
        cmd.extend(self.size_args)

        if timing:
            cmd.append('-timing')

        if parallel:
            cmd.append('-parallel')

        cmd_string = 'cd ' + self.path + ' && ' + ' '.join(cmd)
        print('Running ' + cmd_string + ' ...')

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


def main():

    fir = Test('../../samples/fir', 'fir', ['-length=8192'])
    mm = Test('../../samples/matrixmultiplication',
              'matrixmultiplication', ['-x=128', '-y=128', '-z=128'])
    km = Test('../../samples/kmeans', 'kmeans', [
        '-points=1024',
        '-features=32',
        '-clusters=5',
        '-max-iter=5'])
    mt = Test('../../samples/matrixtranspose',
              'matrixtranspose', ['-width=256'])
    bs = Test('../../samples/bitonicsort',
              'bitonicsort', ['-length=4096'])
    aes = Test('../../samples/aes', 'aes', ['-length=16384'])
    sc = Test('../../samples/simpleconvolution', 'simpleconvolution', [])
    re = Test('../../samples/relu', 'relu', [])
    mp = Test('../../samples/maxpooling', 'maxpooling', [])
    cw = Test('../../samples/concurrentworkload', 'concurrentworkload', [])

    err = False
    err |= compile('../../insts/gcn3disassembler')
    err |= fir.test()
    err |= mm.test()
    err |= km.test()
    err |= mt.test()
    err |= aes.test()
    err |= bs.test()
    err |= sc.test()
    err |= re.test()
    err |= mp.test()
    err |= cw.compile()
    err |= cw.run_test(False, False, '1')
    err |= cw.run_test(False, True, '1')
    err |= cw.run_test(True, False, '1')
    err |= cw.run_test(True, True, '1')

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
