#!/usr/bin/env python3
"""Run a command with stderr on a pseudo terminal and stdout on a pipe.

The equivalence suite uses it to see how a tool behaves when stderr is a terminal,
for example when it turns colors on. The bytes that the command writes to stderr go
to the stderr of this script. The exit code of the command is the exit code of this script.

Usage: pty-run.py <command> [<argument> ...]
"""
import os
import pty
import subprocess
import sys


def main() -> int:
    master, slave = pty.openpty()
    proc = subprocess.Popen(sys.argv[1:], stderr=slave)
    os.close(slave)
    while True:
        try:
            data = os.read(master, 65536)
        except OSError:
            break
        if not data:
            break
        os.write(2, data)
    return proc.wait()


if __name__ == "__main__":
    sys.exit(main())
