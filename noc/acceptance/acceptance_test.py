import os
import subprocess


class Test:
    """A Test represents a single acceptance test"""

    def __init__(self, name):
        self.name = name
        dir = os.path.dirname(os.path.realpath(__file__))
        self.path = dir + '/' + name

    def run(self):
        succeed = True
        print(self.name + ':')
        succeed = self.__build() and succeed
        succeed = self.__test() and succeed
        return succeed

    def __build(self):
        print('\tBuilding ' + self.name + ' ... ', end='')
        process = subprocess.run('go build', shell=True, cwd=self.path)

        if process.returncode != 0:
            print('\tFailed')
            return False

        print('\tSucceed')
        return True

    def __test(self):
        print('\tTesting ' + self.name + ' ... ', end='')
        process = subprocess.run('./' + self.name,
                                 shell=True, cwd=self.path,
                                 stderr=subprocess.DEVNULL,
                                 stdout=subprocess.DEVNULL)

        if process.returncode != 0:
            print('\tFailed')
            return False

        print('\tSucceed')
        return True


def main():
    tests = [
        Test('pcie_p2p'),
        Test('pcie_random'),
        Test('dgx_single_p2p'),
        Test('dgx_single_random'),
    ]

    succeed = True
    for t in tests:
        succeed = t.run() and succeed

    if not succeed:
        exit(1)


if __name__ == '__main__':
    main()
