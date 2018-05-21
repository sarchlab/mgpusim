"""Ground Truth"""

import re
import subprocess

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
    process = subprocess.Popen("make", shell=True, cwd="empty_kernel",
                               stdout=subprocess.DEVNULL)
    process.wait()

    data_columns = ['benchmark', 'time']
    data = pd.DataFrame(columns=data_columns)

    for i in range(0, 20):
        time = run_benchmark('./kernel', 'empty_kernel')
        data = data.append(
            pd.DataFrame([['empty_kernel', time]], columns=data_columns))

    data = data.reset_index()

    plt.figure()
    data.plot()
    plt.savefig('empty_kernel.pdf')
    data.to_csv('ground_truth.csv')


if __name__ == '__main__':
    main()
