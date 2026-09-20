"""Exercise the actual executable in a POSIX pseudo-terminal."""

import fcntl
import json
import os
import pathlib
import pty
import select
import signal
import struct
import subprocess
import sys
import termios
import time

binary, home, action = sys.argv[1:]
config = pathlib.Path(home, '.atlas', 'config.yaml')
config.parent.mkdir(parents=True, exist_ok=True)
config.write_text('''default_model: test/model
providers:
  - name: test
    type: responses
    base_url: https://example.invalid
    api_key: unused-test-key
    models:
      - value: model
''')
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 32, 100, 0, 0))
before = termios.tcgetattr(slave)
environment = {**os.environ, 'HOME': home, 'TERM': 'xterm-256color'}
environment.pop('NO_COLOR', None)
if action == 'no_color':
    environment['NO_COLOR'] = ''
if action == 'dumb':
    environment['TERM'] = 'dumb'
process = subprocess.Popen([binary], stdin=slave, stdout=slave,
                           stderr=subprocess.PIPE, cwd=home, env=environment)
output = bytearray()
sent = False
started = time.monotonic()
try:
    while time.monotonic() - started < 15:
        if select.select([master], [], [], 0.1)[0]:
            output.extend(os.read(master, 65536))
        if not sent and time.monotonic() - started > 1 and b'Message Atlas' in output:
            if action == 'quit':
                # First Enter accepts slash completion, second submits it.
                os.write(master, b'/quit\r')
                time.sleep(0.2)
                os.write(master, b'\r')
            elif action == 'sigint':
                process.send_signal(signal.SIGINT)
            elif action == 'sigterm':
                process.send_signal(signal.SIGTERM)
            sent = True
        if process.poll() is not None:
            break
    timed_out = process.poll() is None
    if timed_out:
        process.kill()
    process.wait(timeout=5)
    after = termios.tcgetattr(slave)
    print(json.dumps({
        'exitCode': process.returncode,
        'timedOut': timed_out,
        'restored': before[:6] == after[:6],
        'rendered': b'Message Atlas' in output,
        'cursorRestored': b'\x1b[?25h' in output,
        'alternateScreenLeft': b'\x1b[?1049l' in output,
        'hasEscapes': b'\x1b' in output,
        'stderr': process.stderr.read().decode(),
    }))
finally:
    if process.poll() is None:
        process.kill()
        process.wait()
    os.close(master)
    os.close(slave)
