"""Ground Truth"""

import re
import subprocess

import numpy as np
import pandas as pd
import matplotlib
matplotlib.use('agg')
import matplotlib.pyplot as plt


def run_benchmark(cmd, cwd):
    process = subprocess.Popen(cmd, shell=True, cwd=cwd,
                               stdout=subprocess.PIPE)
    (stdout, _) = process.communicate()

    m = re.search(r'Kernel [0-9\.]+ - [0-9\.]+: ([0-9\.]+)', str(stdout))
    return float(m.group(1))


def main():
    """ main function """
    data_columns = ['benchmark','time']
    data = pd.DataFrame(columns=data_columns)

    benchmark_arr = ['empty_kernel','atomics','barrier','bitwise_ops',
                     'branching','dp_add','dp_div','dp_mul','valu_add',
                     'valu_div','valu_mul',
                    ]
    for bm in benchmark_arr:
        process = subprocess.Popen("make", shell=True, cwd=bm,
                                stdout=subprocess.DEVNULL)
        process.wait()

        for i in range(1, 21):
            time = run_benchmark('./kernel', bm)
            data = data.append(
                pd.DataFrame([[bm, time]], columns=data_columns),
                ignore_index=True,
            )


    data.to_csv('ground_truth.csv')

    summary = data.groupby("benchmark").aggregate([np.mean, np.std])
    summary.columns = ['_'.join(col).strip() for col in summary.columns.values]
    summary.to_csv('ground_truth_summary.csv')


if __name__ == '__main__':
    main()
