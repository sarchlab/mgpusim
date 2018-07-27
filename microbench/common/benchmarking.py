import os
import subprocess
import re

def parse_kernel_time(filename):
    pattern = re.compile(r'[a-zA-Z0-9_]+[\s]+[0-9xa-fA-F]+[\s]+([0-9]+)[\s]+([0-9]+)[\s]+[gfx0-9]+[\s]+{[0-9]+}[\s]+[0-9]+[\s]+[0-9]+')
    fp = open(filename, 'r')
    kernel_time = 0
    for line in fp:
        match = pattern.match(line)
        if match != None:
            start = float(match.group(1))
            end = float(match.group(2))
            duration = (end-start)*1e-9
            kernel_time += duration
    return kernel_time


def run_benchmark_on_gpu(cmd, cwd):
    filename = cwd + 'exp'
    fp = open(filename + '_stdout.out', 'w')
    command = 'rcprof -o ' + filename + '_trace.atp -A -w ' + cwd + ' ' \
                + cmd,
    print(command)

    p = subprocess.Popen(command, shell=True, cwd=cwd, stdout=fp, stderr=fp)
    p.wait()
    res = parse_kernel_time(filename + '_trace.atp')
    return res


def run_benchmark_on_simulator(cmd, cwd):
    print(cmd)
    filename = cwd + 'sim_exp.out'
    fp = open(filename, 'w')
    process = subprocess.Popen(cmd, shell=True, cwd=cwd,
                               stdout=fp, stderr=fp)
    process.wait()

    fp = open(filename, 'r')
    kernel_time = 0
    for line in fp:
        m = re.match(r'Kernel: \[([0-9.]+) - ([0-9.]+)]', str(line))
        if m != None:
            kernel_time +=  float(m.group(2)) - float(m.group(1))

    return kernel_time



