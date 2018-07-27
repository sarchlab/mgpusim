"""Ground Truth"""

import re
import subprocess

import numpy as np
import pandas as pd

def run_benchmark(cmd, cwd):
    process = subprocess.Popen(cmd, shell=True, cwd=cwd,
                               stdout=subprocess.PIPE)
    (stdout, _) = process.communicate()

    m = re.search(r'Kernel [0-9\.]+ - [0-9\.]+: ([0-9\.]+)', str(stdout))
    return float(m.group(1))


def run():
    """ run benchmark and retuns a data frame that represents its result """
    data_columns = ['benchmark', 'numWfPerWG', 'numWG', 'time']
    data = pd.DataFrame(columns=data_columns)

    process = subprocess.Popen("make", shell=True, cwd='.',
                            stdout=subprocess.DEVNULL)
    process.wait()

    for numWfPerWG in range(1, 9):
        print('numWfPerWG', numWfPerWG)
        for numWG in range(128, 1025, 128):
            print('numWG', numWG)
            for i in range(1, 100):
                time = run_benchmark(
                    './kernel {0} {1}'.format(numWG, numWfPerWG), '.')
                data = data.append(
                    pd.DataFrame([['empty_kernel_with_register_initialization', numWfPerWG, numWG, time]],
                                 columns=data_columns),
                    ignore_index=True,
                )

    return data

def main():
    data = run()
    data.to_csv('r9nano.csv')
    grouped = data.groupby(['numWG', 'numWfPerWG'])
    print(data.describe())

if __name__ == '__main__':
    main()

