#!/usr/bin/env python3
"""安装发行包中的程序、私有配置与平台服务；不自动升级或重置已有数据。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import plistlib
import pwd
import shlex
import shutil
import subprocess
import sys
import tempfile
import time


def fail(message):
    raise RuntimeError(message)


def run(args, capture=False):
    result = subprocess.run([str(x) for x in args], capture_output=True, text=True,
                            env={k:v for k,v in os.environ.items() if not k.startswith('MYBUILDS_')})
    if result.returncode:
        # 子命令可能处理凭据，不回显其输出。
        fail(f'{Path(str(args[0])).name} 执行失败（退出码 {result.returncode}）；请检查配置或服务日志')
    return result.stdout if capture else None


def private_dir(path):
    if path.is_symlink():
        fail(f'拒绝符号链接目录：{path}')
    path.mkdir(parents=True, exist_ok=True, mode=0o700)
    if path.stat().st_uid != os.getuid():
        fail(f'目录不属于当前用户：{path}')
    path.chmod(0o700)


def write_new(path, data, mode=0o600):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    with os.fdopen(fd, 'wb') as stream:
        stream.write(data.encode() if isinstance(data, str) else data)


def write_json(path, value):
    write_new(path, json.dumps(value, ensure_ascii=False, indent=2) + '\n')


def hash_file(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def install_file(source, destination, mode=0o755):
    if destination.is_symlink():
        fail(f'拒绝覆盖符号链接：{destination}')
    if destination.exists():
        if not destination.is_file() or hash_file(source) != hash_file(destination):
            fail(f'已存在不同文件，停止后请先备份并移走：{destination}')
        return
    write_new(destination, source.read_bytes(), mode)


def service(args, role, binary, config, home):
    label = 'io.mybuilds.' + role
    log = args.config_dir / 'logs' / role
    command = [str(binary), '--config', str(config), 'serve']
    env_path = str(args.bin_dir) + ':/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin'
    if platform.system() == 'Darwin':
        system = args.service == 'system'
        domain = 'system' if system else f'gui/{os.getuid()}'
        prefix = ['sudo', '-n'] if system else []
        folder = Path('/Library/LaunchDaemons') if system else home / 'Library/LaunchAgents'
        dest = folder / (label + '.plist')
        value = {'Label': label, 'ProgramArguments': command, 'RunAtLoad': True,
                 'KeepAlive': True, 'ThrottleInterval': 10, 'WorkingDirectory': str(home),
                 'EnvironmentVariables': {'HOME': str(home), 'PATH': env_path},
                 'StandardOutPath': str(log) + '.log', 'StandardErrorPath': str(log) + '.err.log'}
        if system:
            value['UserName'] = pwd.getpwuid(os.getuid()).pw_name
        data = plistlib.dumps(value)
        if dest.exists():
            if dest.read_bytes() != data:
                fail(f'已有不同服务定义，拒绝覆盖：{dest}')
        elif system:
            with tempfile.NamedTemporaryFile() as tmp:
                tmp.write(data); tmp.flush()
                run(prefix + ['install', '-o', 'root', '-g', 'wheel', '-m', '644', tmp.name, str(dest)])
        else:
            folder.mkdir(parents=True, exist_ok=True)
            write_new(dest, data, 0o644)
        if subprocess.run(prefix + ['launchctl', 'print', domain + '/' + label], capture_output=True).returncode:
            run(prefix + ['launchctl', 'bootstrap', domain, str(dest)])
        print(f'服务：{domain}/{label}；日志：{log}.err.log')
    else:
        system = args.service == 'system'
        prefix = ['sudo', '-n'] if system else []
        ctl = prefix + ['systemctl'] + ([] if system else ['--user'])
        folder = Path('/etc/systemd/system') if system else home / '.config/systemd/user'
        dest = folder / (label + '.service')
        # systemd 的 % 不是路径字面量；双引号内也必须转义。
        def quote(value):
            return '"' + str(value).replace('\\', '\\\\').replace('"', '\\"').replace('%', '%%') + '"'
        user = 'User=' + pwd.getpwuid(os.getuid()).pw_name + '\n' if system else ''
        data = ('[Unit]\nDescription=mybuilds ' + role + '\nAfter=network.target\n\n[Service]\n' + user +
                'ExecStart=' + ' '.join(quote(v) for v in command) + '\nWorkingDirectory=' + quote(home) +
                '\nEnvironment=' + quote('HOME=' + str(home)) + ' ' + quote('PATH=' + env_path) +
                '\nRestart=on-failure\nRestartSec=5\nUMask=0077\n\n[Install]\nWantedBy=' +
                ('multi-user.target' if system else 'default.target') + '\n').encode()
        if dest.exists():
            if dest.read_bytes() != data:
                fail(f'已有不同服务定义，拒绝覆盖：{dest}')
        elif system:
            with tempfile.NamedTemporaryFile() as tmp:
                tmp.write(data); tmp.flush()
                run(prefix + ['install', '-o', 'root', '-g', 'root', '-m', '644', tmp.name, str(dest)])
        else:
            folder.mkdir(parents=True, exist_ok=True)
            write_new(dest, data, 0o644)
        run(ctl + ['daemon-reload'])
        run(ctl + ['enable', '--now', label + '.service'])
        print('服务：' + label + '.service；日志使用 journalctl' + ('' if system else ' --user'))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('role', choices=['client', 'server'])
    parser.add_argument('--bin-dir', type=Path, default=Path.home() / '.local/bin')
    parser.add_argument('--config-dir', type=Path, default=Path.home() / '.mybuilds')
    parser.add_argument('--skills-dir', type=Path, default=Path.home() / '.codex/skills')
    parser.add_argument('--server-url')
    parser.add_argument('--token-file', type=Path)
    parser.add_argument('--ca-file', type=Path)
    parser.add_argument('--with-agent', action='store_true')
    parser.add_argument('--port', type=int, default=8787)
    parser.add_argument('--service', choices=['user', 'system', 'none'], default='user')
    parser.add_argument('--no-path', action='store_true', help='不修改 shell 启动文件')
    args = parser.parse_args()
    home = Path.home()
    if platform.system() not in ('Darwin', 'Linux') or os.getuid() == 0:
        fail('请以普通用户在 macOS/Linux 运行；系统服务使用 --service system 调用 sudo')
    if not 1 <= args.port <= 65535:
        fail('端口必须在 1–65535')
    if bool(args.server_url) != bool(args.token_file):
        fail('--server-url 与 --token-file 必须一起提供')
    if args.role == 'server' and (args.server_url or args.ca_file):
        fail('连接参数仅用于客户端安装')
    if args.role == 'client' and args.with_agent:
        fail('--with-agent 仅用于服务端安装')
    package = Path(__file__).resolve().parent.parent
    for name in ('bin_dir', 'config_dir', 'skills_dir'):
        path = getattr(args, name).expanduser().absolute()
        if any(ord(c) < 32 for c in str(path)) or '$' in str(path):
            fail('安装路径不支持控制字符或 $')
        # resolve 前先拒绝叶子符号链接。
        if path.is_symlink():
            fail(f'拒绝符号链接路径：{path}')
        setattr(args, name, path.resolve())
    token = None
    if args.token_file:
        st = args.token_file.lstat()
        if args.token_file.is_symlink() or not args.token_file.is_file() or st.st_uid != os.getuid() or st.st_mode & 0o777 != 0o600:
            fail('token 文件必须属于当前用户、为0600普通文件')
        token = args.token_file.read_text().strip()
        if not 32 <= len(token) <= 4096 or any(ord(c) < 33 or ord(c) > 126 for c in token):
            fail('token 格式无效')
        from urllib.parse import urlsplit
        url = urlsplit(args.server_url)
        if url.scheme != 'https' and not (url.scheme == 'http' and url.hostname in ('127.0.0.1', 'localhost', '::1')):
            fail('服务地址必须是 HTTPS 或回环 HTTP')
        if not url.hostname or url.username or url.password or url.query or url.fragment:
            fail('服务地址格式无效')
    if args.ca_file and not args.ca_file.is_file():
        fail('CA 文件不存在')
    if args.role == 'server' and args.service != 'none':
        if not shutil.which('git'):
            fail('服务端需要 Git，请先安装')
        if args.service == 'system':
            run(['sudo', '-n', 'true'])
        elif platform.system() == 'Darwin':
            run(['launchctl', 'print', f'gui/{os.getuid()}'])
        else:
            run(['systemctl', '--user', 'show-environment'])
    args.bin_dir.mkdir(parents=True, exist_ok=True)
    private_dir(args.config_dir)
    lock = args.config_dir / '.install-lock'
    try:
        lock.mkdir(mode=0o700)
    except FileExistsError:
        fail(f'另一次安装未结束；确认没有安装进程后再移除 {lock}')
    try:
        state_file = args.config_dir / ('install-' + args.role + '.json')
        state = {'version': (package / 'VERSION').read_text().strip(), 'bin_dir': str(args.bin_dir)}
        if state_file.exists() and json.loads(state_file.read_text()) != state:
            fail('已有不同版本或安装目录；请按升级说明停机、备份后处理')
        server_cfg, client_cfg, agent_cfg = [args.config_dir / (n + '.yml') for n in ('server', 'client', 'agent')]
        if any(p.is_symlink() for p in (state_file, server_cfg, client_cfg, agent_cfg)):
            fail('拒绝符号链接配置或安装记录')
        if args.role == 'server' and server_cfg.exists() and not state_file.exists():
            fail('已有手工配置或未完成的初始化；请保留数据并手动完成，不自动接管')
        names = ['mybuilds'] if args.role == 'client' else ['mybuilds', 'mybuilds-server', 'mybuilds-agent']
        # 先检查全部目标，避免不同已有文件导致部分覆盖。
        for name in names:
            src, dst = package / 'bin' / name, args.bin_dir / name
            if not src.is_file() or dst.is_symlink() or (dst.exists() and hash_file(src) != hash_file(dst)):
                fail(f'安装文件缺失或目标已有不同版本：{name}')
        for name in names:
            install_file(package / 'bin' / name, args.bin_dir / name)
        client, server = args.bin_dir / 'mybuilds', args.bin_dir / 'mybuilds-server'
        if args.role == 'client' and token and not client_cfg.exists():
            value = {'server': args.server_url, 'token': token, 'timeout': '30s'}
            if args.ca_file:
                value['ca_file'] = str(args.ca_file.expanduser().resolve())
            write_json(client_cfg, value)
        if args.role == 'server' and not state_file.exists():
            if client_cfg.exists() or agent_cfg.exists():
                fail('配置目录已有客户端或Agent配置；请使用新的服务端配置目录')
            write_json(server_cfg, {'listen': f'127.0.0.1:{args.port}', 'data_dir': str(args.config_dir / 'server'),
                                    'concurrency': 1, 'database': {'driver': 'sqlite'}})
            result = json.loads(run([server, '--config', server_cfg, 'token', 'create', '--role', 'admin', '--json'], True))
            write_json(client_cfg, {'server': f'http://127.0.0.1:{args.port}', 'token': result['token'], 'timeout': '30s'})
        if args.role == 'server' and args.with_agent and not agent_cfg.exists():
            if state_file.exists():
                fail('同机Agent仅在首次安装时自动初始化；已有服务请按文档通过客户端登记并配置Agent')
            result = json.loads(run([server, '--config', server_cfg, 'node', 'create', 'local', '--capacity', '1', '--json'], True))
            write_json(agent_cfg, {'server': json.loads(client_cfg.read_text())['server'], 'node': 'local',
                                  'token': result['token'], 'capacity': 1, 'data_dir': str(args.config_dir / 'agent')})
        if not state_file.exists():
            write_json(state_file, state)
        for name in ('mybuilds-deploy', 'mybuilds-operate'):
            dest = args.skills_dir / name
            dest.mkdir(parents=True, exist_ok=True)
            install_file(package / 'skills' / name / 'SKILL.md', dest / 'SKILL.md', 0o644)
        if not args.no_path:
            line = '\n# mybuilds 用户命令\nexport PATH=' + shlex.quote(str(args.bin_dir)) + ':"$PATH"\n'
            for rc in ('.profile', '.bashrc', '.zshrc'):
                dest = home / rc
                if dest.is_symlink():
                    fail(f'启动文件为符号链接，请手动配置PATH：{dest}')
                old = dest.read_text() if dest.exists() else ''
                if line.strip() not in old:
                    with dest.open('a') as stream:
                        stream.write(line)
        if args.role == 'server' and args.service != 'none':
            private_dir(args.config_dir / 'logs')
            service(args, 'server', server, server_cfg, home)
            if args.with_agent:
                service(args, 'agent', args.bin_dir / 'mybuilds-agent', agent_cfg, home)
            for _ in range(30):
                check = subprocess.run([str(client), '--config', str(client_cfg), 'status', '--json'], capture_output=True,
                                       env={k:v for k,v in os.environ.items() if not k.startswith('MYBUILDS_')})
                if check.returncode == 0:
                    break
                time.sleep(1)
            else:
                fail('服务状态检查失败；配置已保留，请检查服务日志')
        print(f'安装完成：{args.bin_dir}；配置：{args.config_dir}；AI skills：{args.skills_dir}')
        print('重新打开终端后可使用 mybuilds；凭据未输出，已有配置保持原样。')
    finally:
        lock.rmdir()


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, RuntimeError) as error:
        print('安装失败：' + str(error), file=sys.stderr)
        sys.exit(1)
