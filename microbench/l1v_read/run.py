import re
import subprocess
import argparse
import os
import sys
sys.path.append(os.getcwd() + '/../common/')
from benchmarking import *


import pandas as pd

data_columns = ['benchmark', 'env', 'num_access', 'time']

def generate_benchmark(count):
    with open('microbench/kernels_template.asm', 'r') as template_file:
        template = template_file.read()

    insts = ''
    for i in range(0, count):
        insts += 'flat_load_dword v0, v[1:2]\n'
        insts += 's_waitcnt vmcnt(0)\n'
    kernel = template.format(insts)

    with open('microbench/kernels.asm', 'w') as kernel_file:
        kernel_file.write(kernel)

    p = subprocess.Popen('make kernels.hsaco', cwd='microbench',
                         shell=True)
    p.wait()


def run_on_simulator(num_access):
    """ run benchmark and retuns a data frame that represents its result """
    data = pd.DataFrame(columns=data_columns)

    process = subprocess.Popen("go build", shell=True, cwd='.',
                            stdout=subprocess.DEVNULL)
    process.wait()

    duration = run_benchmark_on_simulator('./l1v_read -timing', os.getcwd())
    entry = ['l1v_read', 'sim', num_access , duration]
    print(entry)
    data = data.append(
        pd.DataFrame([entry], columns=data_columns),
        ignore_index=True,
    )

    return data


def run_on_gpu(num_access, repeat):
    data = pd.DataFrame(columns=data_columns)

    for i in range(0, repeat):
        duration = run_benchmark_on_gpu(
            './kernel', os.getcwd() + '/microbench/')
        entry = ['l1v_read', 'gpu', num_access, duration]
        print(entry)
        data = data.append(
            pd.DataFrame([entry], columns=data_columns),
            ignore_index=True,
        )

    return data


def parse_args():
    parser = argparse.ArgumentParser(description='L1_Read microbenchmark')
    parser.add_argument('--gpu', dest='gpu', action='store_true')
    parser.add_argument('--sim', dest='sim', action='store_true')
    parser.add_argument('--repeat', type=int, default=20)
    args = parser.parse_args()
    return args


def main():
    args = parse_args()

    num_access_list = range(0, 129, 4)

    if args.gpu:
        data = pd.DataFrame(columns=data_columns)
        for num_access in num_access_list:
            generate_benchmark(num_access)
            results = run_on_gpu(num_access, args.repeat)
            data = data.append(results, ignore_index=True)
        data.to_csv('gpu.csv')

    if args.sim:
        data = pd.DataFrame(columns=data_columns)
        for num_access in num_access_list:
            generate_benchmark(num_access)
            results = run_on_simulator(num_access)
            data = data.append(results, ignore_index=True)
        data.to_csv('sim.csv')


if __name__ == '__main__':
    main()

