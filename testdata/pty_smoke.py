#!/usr/bin/env python3
"""Run with a compiled app test binary; clipboard uses the injected test writer."""
import fcntl, json, os, pathlib, pty, re, select, signal, struct, sys, tempfile, termios, time
binary = sys.argv[1]
ansi = re.compile(rb'\x1b\[[0-?]*[ -/]*[@-~]')

def run_case(stream):
    with tempfile.TemporaryDirectory(prefix='logsee-pty-') as root:
        root = pathlib.Path(root)
        source = root/'input.log'
        source.write_text('one error 한글😀\ntwo\nthree error\n')
        out, copied = root/'sot.log', root/'copy.txt'
        r, w = os.pipe()
        pid, master = pty.fork()
        if pid == 0:
            if stream:
                os.dup2(r, 0)
            os.close(r); os.close(w)
            env = dict(os.environ, TERM='xterm-256color', LOGSEE_PTY_HELPER='1', LOGSEE_TEST_INPUT='-' if stream else str(source), LOGSEE_TEST_OUT=str(out), LOGSEE_TEST_COPY=str(copied), LOGSEE_TEST_HOME=str(root))
            os.execve(binary, [binary, '-test.run=^TestPTYHelper$', '-test.v'], env)
        os.close(r)
        before = termios.tcgetattr(master)
        fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack('HHHH', 12, 110, 0, 0))
        output = bytearray()
        def pump():
            if select.select([master], [], [], 0.01)[0]:
                try:
                    data=os.read(master,65536);output.extend(data)
                    if b'\x1b]11;?' in data:os.write(master,b'\x1b]11;rgb:0000/0000/0000\x1b\\')
                    if b'\x1b]10;?' in data:os.write(master,b'\x1b]10;rgb:ffff/ffff/ffff\x1b\\')
                    if b'\x1b[6n' in data:os.write(master,b'\x1b[1;1R')
                except OSError: pass
        def wait(needle, timeout=2):
            end = time.monotonic()+timeout
            while time.monotonic()<end:
                pump()
                if needle.encode() in ansi.sub(b'', output): return
            raise AssertionError(f'missing {needle!r}: {ansi.sub(b"", output)[-1500:]!r}')
        metrics = {'stream':stream}
        try:
            wait('FILTER INPUT')
            if stream:
                wait('auto~')
                raw = '11-20 12:34:56.789 123 456 E Tag: error 한글😀\r\n'.encode()
                began = time.monotonic(); os.write(w, raw); wait('한글😀', 1)
                metrics['idle_line_ms'] = round((time.monotonic()-began)*1000, 2)
                assert out.read_bytes()==raw
                os.close(w);w=-1;wait('type:adb')
            else:
                wait('한글😀')
                os.write(master,b'G ');wait('sel:1')
                os.write(master,b'\x1b[H c')
                end=time.monotonic()+2
                while not copied.exists() and time.monotonic()<end:pump()
                assert copied.read_text()=='one error 한글😀\nthree error', copied.read_text()
                os.write(master,b'/');wait('> _')
                os.write(master,b'error#red err#green\r')
                for _ in range(8):pump()
                os.write(master,b'?');wait('Movement:')
                os.write(master,b'\x1b[6~');wait('Selection:')
                os.write(master,b'\x1b')
            fcntl.ioctl(master,termios.TIOCSWINSZ,struct.pack('HHHH',18,140,0,0));os.kill(pid,signal.SIGWINCH)
            for _ in range(8):pump()
            os.write(master,b'\x03')
            end=time.monotonic()+2;status=None
            while time.monotonic()<end:
                pump();child,status=os.waitpid(pid,os.WNOHANG)
                if child:break
            else:raise AssertionError('quit did not finish')
            assert os.waitstatus_to_exitcode(status)==0,output[-1200:]
            after=termios.tcgetattr(master)
            mask=termios.ECHO|termios.ICANON
            assert before[3]&mask==after[3]&mask, 'terminal mode not restored'
            assert b'\x1b[?1049l' in output, 'alternate screen not restored'
            metrics['terminal_restored']=True
            print(json.dumps(metrics),flush=True)
        finally:
            if w>=0:os.close(w)
            try:os.kill(pid,signal.SIGKILL)
            except ProcessLookupError:pass
            try:os.waitpid(pid,0)
            except ChildProcessError:pass
            os.close(master)
for stream in (False,True):run_case(stream)
