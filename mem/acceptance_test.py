import subprocess
import os
import sys


class colors:
    """Colors class:
    reset all colors with colors.reset
    two subclasses fg for foreground and bg for background.
    use as colors.subclass.colorname.
    i.e. colors.fg.red or colors.bg.green
    also, the generic bold, disable, underline, reverse, strikethrough,
    and invisible work with the main class
    i.e. colors.bold
    """
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


def compile_test(path):
    fp = open(os.devnull, 'w')
    p = subprocess.Popen('go build', shell=True, cwd=path, stdout=fp)
    p.wait()
    if p.returncode == 0:
        print(colors.fg.green + "Compiled " + path + colors.reset)
        return False
    else:
        print(colors.fg.red + "Compile failed " + path + colors.reset)
        return True


def run_test(name, cmd, cwd):
    fp = open(os.devnull, 'w')
    p = subprocess.Popen(cmd, shell=True, cwd=cwd, stdout=fp)
    p.wait()

    if p.returncode == 0:
        print(colors.fg.green + name + " Passed." + colors.reset)
        return False
    else:
        print(colors.fg.red + name + " Failed." + colors.reset)
        return True


def main():
    error = False

    error |= compile_test('acceptancetests/idealmemcontroller')
    error |= run_test("Ideal Memory controller 1",
                      './idealmemcontroller -max-address=64 -num-access=10000', 'acceptancetests/idealmemcontroller')
    error |= run_test("Ideal Memory controller 2",
                      './idealmemcontroller -max-address=1024 -num-access=10000', 'acceptancetests/idealmemcontroller')
    error |= run_test("Ideal Memory controller 3",
                      './idealmemcontroller -max-address=1048576 -num-access=10000', 'acceptancetests/idealmemcontroller')
    error |= run_test("Ideal Memory controller 4",
                      './idealmemcontroller -max-address=64 -parallel -num-access=10000', 'acceptancetests/idealmemcontroller')
    error |= run_test("Ideal Memory controller 5",
                      './idealmemcontroller -max-address=1024 -parallel -num-access=10000', 'acceptancetests/idealmemcontroller')
    error |= run_test("Ideal Memory controller 6",
                      './idealmemcontroller -max-address=1048576 -parallel -num-access=10000', 'acceptancetests/idealmemcontroller')

    error |= compile_test('acceptancetests/writebackcache')
    error |= run_test("Write-back cache 1",
                      './writebackcache -max-address=64 -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 2",
                      './writebackcache -max-address=1024 -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 3",
                      './writebackcache -max-address=1048576 -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 4",
                      './writebackcache -max-address=64 -parallel -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 5",
                      './writebackcache -max-address=1024 -parallel -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 6",
                      './writebackcache -max-address=1048576 -parallel -num-access=10000', 'acceptancetests/writebackcache')

    error |= compile_test('acceptancetests/writebackcache')
    error |= run_test("Write-back cache 1",
                      './writebackcache -max-address=64 -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 2",
                      './writebackcache -max-address=1024 -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 3",
                      './writebackcache -max-address=1048576 -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 4",
                      './writebackcache -max-address=64 -parallel -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 5",
                      './writebackcache -max-address=1024 -parallel -num-access=10000', 'acceptancetests/writebackcache')
    error |= run_test("Write-back cache 6",
                      './writebackcache -max-address=1048576 -parallel -num-access=10000', 'acceptancetests/writebackcache')

    error |= compile_test('acceptancetests/dram')
    error |= run_test(
        "DRAM cache 1", './dram -max-address=64 -num-access=10000', 'acceptancetests/dram')
    error |= run_test(
        "DRAM cache 2", './dram -max-address=1024 -num-access=10000', 'acceptancetests/dram')
    error |= run_test(
        "DRAM cache 3", './dram -max-address=1048576 -num-access=10000', 'acceptancetests/dram')
    error |= run_test(
        "DRAM cache 4", './dram -max-address=64 -parallel -num-access=10000', 'acceptancetests/dram')
    error |= run_test(
        "DRAM cache 5", './dram -max-address=1024 -parallel -num-access=10000', 'acceptancetests/dram')
    error |= run_test(
        "DRAM cache 6", './dram -max-address=1048576 -parallel -num-access=10000', 'acceptancetests/dram')

    error |= compile_test('acceptancetests/writeevictcache')
    error |= run_test("Write-evict cache 1",
                      './writeevictcache -max-address=64 -num-access=10000', 'acceptancetests/writeevictcache')
    error |= run_test("Write-evict cache 2",
                      './writeevictcache -max-address=1024 -num-access=10000', 'acceptancetests/writeevictcache')
    error |= run_test("Write-evict cache 3",
                      './writeevictcache -max-address=1048576 -num-access=10000', 'acceptancetests/writeevictcache')
    error |= run_test("Write-evict cache 4",
                      './writeevictcache -max-address=64 -parallel -num-access=10000', 'acceptancetests/writeevictcache')
    error |= run_test("Write-evict cache 5",
                      './writeevictcache -max-address=1024 -parallel -num-access=10000', 'acceptancetests/writeevictcache')
    error |= run_test("Write-evict cache 6",
                      './writeevictcache -max-address=1048576 -parallel -num-access=10000', 'acceptancetests/writeevictcache')

    error |= compile_test('acceptancetests/writethroughcache')
    error |= run_test("Write-through cache 1",
                      './writethroughcache -max-address=64 -num-access=10000', 'acceptancetests/writethroughcache')
    error |= run_test("Write-through cache 2",
                      './writethroughcache -max-address=1024 -num-access=10000', 'acceptancetests/writethroughcache')
    error |= run_test("Write-through cache 3",
                      './writethroughcache -max-address=1048576 -num-access=10000', 'acceptancetests/writethroughcache')
    error |= run_test("Write-through cache 4",
                      './writethroughcache -max-address=64 -parallel -num-access=10000', 'acceptancetests/writethroughcache')
    error |= run_test("Write-through cache 5",
                      './writethroughcache -max-address=1024 -parallel -num-access=10000', 'acceptancetests/writethroughcache')
    error |= run_test("Write-through cache 6",
                      './writethroughcache -max-address=1048576 -parallel -num-access=10000', 'acceptancetests/writethroughcache')

    error |= compile_test('acceptancetests/writearoundcache')
    error |= run_test("Write-around cache 1",
                      './writearoundcache -max-address=64 -num-access=10000', 'acceptancetests/writearoundcache')
    error |= run_test("Write-around cache 2",
                      './writearoundcache -max-address=1024 -num-access=10000', 'acceptancetests/writearoundcache')
    error |= run_test("Write-around cache 3",
                      './writearoundcache -max-address=1048576 -num-access=10000', 'acceptancetests/writearoundcache')
    error |= run_test("Write-around cache 4",
                      './writearoundcache -max-address=64 -parallel -num-access=10000', 'acceptancetests/writearoundcache')
    error |= run_test("Write-around cache 5",
                      './writearoundcache -max-address=1024 -parallel -num-access=10000', 'acceptancetests/writearoundcache')
    error |= run_test("Write-around cache 6",
                      './writearoundcache -max-address=1048576 -parallel -num-access=10000', 'acceptancetests/writearoundcache')

    if error:
        sys.exit(1)


if __name__ == '__main__':
    main()
