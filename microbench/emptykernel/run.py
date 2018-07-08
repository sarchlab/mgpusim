import re
import subprocess
import numpy as np
import pandas as pd
import os
import argparse
import sys
sys.path.append(os.getcwd() + '/../common/')
from benchmarking import *

data_columns = ['benchmark', 'env', 'numWf', 'numWG', 'time']

def run_benchmark_on_simulator():
    pass


def run_on_simulator():
    """ run benchmark and retuns a data frame that represents its result """
    data = pd.DataFrame(columns=data_columns)

    process = subprocess.Popen("go build", shell=True, cwd='.',
                            stdout=subprocess.DEVNULL)
    process.wait()

    for num_inst in range(0, 128, 4):
        print('On GPU: {0}, {1}'.format(inst, num_inst))
        generate_benchmark(inst, num_inst)
        for i in range(0, 100):
            duration = run_benchmark_on_simulator()
            data = data.append(
                pd.DataFrame([['alu', 'simulator', inst, num_inst, duration]],
                             columns=data_columns),
                ignore_index=True,
            )

    return data


def run_on_gpu(inst, repeat):
    data = pd.DataFrame(columns=data_columns)

    for numWfPerWG in range(1, 9):
        print('numWfPerWG', numWfPerWG)
        for numWG in range(128, 1025, 128):
            print('numWG', numWG)
            for i in range(0, repeat):
                duration = run_benchmark_on_gpu(
                    './kernel {0} {1}'.format(numWG, numWfPerWG),
                    os.getcwd() + '/microbench/')
                data = data.append(
                    pd.DataFrame([['alu', 'gpu', numWfPerWG, numWG, duration]],
                                columns=data_columns),
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

    if args.gpu:
        for inst in insts:
            data = pd.DataFrame(columns=data_columns)
            results = run_on_gpu(inst, args.repeat)
            data = data.append(results, ignore_index=True)
            data.to_csv('gpu.csv')


if __name__ == '__main__':
    main()

