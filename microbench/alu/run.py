import re
import subprocess
import numpy as np
import pandas as pd

data_columns = ['benchmark', 'env', 'inst', 'count', 'time']

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


def run_benchmark_on_gpu():
    process = subprocess.Popen('./kernel', shell=True,
                               cwd='microbench/',
                               stdout=subprocess.PIPE)
    (stdout, _) = process.communicate()

    m = re.search(r'Kernel [0-9\.]+ - [0-9\.]+: ([0-9\.]+)', str(stdout))
    return float(m.group(1))

def run_on_gpu(inst):
    data = pd.DataFrame(columns=data_columns)

    for num_inst in range(0, 1025, 4):
        print('On GPU: {0}, {1}'.format(inst, num_inst))
        generate_benchmark(inst, num_inst)
        for i in range(0, 20):
            duration = run_benchmark_on_gpu()
            data = data.append(
                pd.DataFrame([['alu', 'gpu', inst, num_inst, duration]],
                             columns=data_columns),
                ignore_index=True,
            )

    return data


def main():
    data = pd.DataFrame(columns=data_columns)
    insts = ['v_add_f32 v1, v2, v3']

    for inst in insts:
        results = run_on_gpu(inst)
        data = data.append(results, ignore_index=True)

    data.to_csv('data.csv')


if __name__ == '__main__':
    main()

