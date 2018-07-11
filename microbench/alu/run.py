import re
import subprocess
import numpy as np
import pandas as pd
import os
import argparse
import sys
sys.path.append(os.getcwd() + '/../common/')
from benchmarking import *

data_columns = ['benchmark', 'env', 'inst', 'numwf', 'numwg', 'count', 'time']

def generate_benchmark(inst, count):
    with open('microbench/kernels_template.asm', 'r') as template_file:
        template = template_file.read()

    with open('microbench/kernels.asm', 'w') as kernel_file:
        kernel_file.write(template)
        for i in range(0, count):
            kernel_file.write(inst + '\n')
        kernel_file.write('s_endpgm\n')

    p = subprocess.Popen('make kernels.hsaco', cwd='microbench',
                         shell=True)
    p.wait()


def run_on_simulator(inst, num_inst, num_wf, num_wg):
    """ run benchmark and retuns a data frame that represents its result """
    data = pd.DataFrame(columns=data_columns)

    process = subprocess.Popen("go build", shell=True, cwd='.',
                            stdout=subprocess.DEVNULL)
    process.wait()

    duration = run_benchmark_on_simulator(
        './alu -timing -num-wf {0} -num-wg {1}'.format(num_wf, num_wg),
        os.getcwd())
    entry = ['alu', 'sim', inst, num_wf, num_wg, num_inst, duration]
    print(entry)
    data = data.append(
        pd.DataFrame([entry], columns=data_columns),
        ignore_index=True,
    )

    return data


def run_on_gpu(inst, num_inst, num_wf, num_wg, repeat):
    data = pd.DataFrame(columns=data_columns)

    for i in range(0, repeat):
        duration = run_benchmark_on_gpu(
            './kernel {0} {1}'.format(num_wf, num_wg),
            os.getcwd() + '/microbench/')
        entry = ['alu', 'gpu', inst, num_wf, num_wg, num_inst, duration]
        print(entry)
        data = data.append(
            pd.DataFrame([entry], columns=data_columns),
            ignore_index=True,
        )

    return data

def parse_args():
    parser = argparse.ArgumentParser(description='ALU microbenchmark')
    parser.add_argument('--gpu', dest='gpu', action='store_true')
    parser.add_argument('--sim', dest='sim', action='store_true')
    parser.add_argument('--repeat', type=int, default=20)
    args = parser.parse_args()
    return args


def main():
    args = parse_args()

    insts = ['v_add_f32 v1, v2, v3']
    numInst = range(0, 129, 4)
    numWf = [1, 2, 3, 4]
    numWG = [1, 2, 3, 4, 5, 6, 7, 8]

    if args.gpu:
        data = pd.DataFrame(columns=data_columns)
        for inst in insts:
            for num_insts in numInst:
                generate_benchmark(inst, num_insts)
                for wf in numWf:
                    for wg in numWG:
                        results = run_on_gpu(inst, num_insts, wf, wg, args.repeat)
                        data = data.append(results, ignore_index=True)
        data.to_csv('gpu.csv')

    if args.sim:
        data = pd.DataFrame(columns=data_columns)
        for inst in insts:
            for num_insts in numInst:
                generate_benchmark(inst, num_insts)
                for wf in numWf:
                    for wg in numWG:
                        results = run_on_simulator(inst, num_insts, wf, wg)
                        data = data.append(results, ignore_index=True)
        data.to_csv('sim.csv')


if __name__ == '__main__':
    main()

