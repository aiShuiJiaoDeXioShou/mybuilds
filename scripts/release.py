#!/usr/bin/env python3
"""构建已指定平台的发行包；仅打包，不自动推送或发布。"""
import argparse
import hashlib
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile
from datetime import datetime, timezone

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--version', required=True)
p.add_argument('--target', action='append', help='如 darwin/arm64；默认当前平台，可重复')
p.add_argument('--output', type=Path, required=True)
a = p.parse_args()
if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?', a.version):
    p.error('版本需为 vX.Y.Z 或其预发行版本')
root = Path(__file__).resolve().parent.parent
targets = a.target or [platform.system().lower() + '/' + {'aarch64':'arm64','x86_64':'amd64'}.get(platform.machine().lower(), platform.machine().lower())]
a.output.mkdir(parents=True, exist_ok=True)
commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
flags = f'-s -w -X mybuilds/internal/version.Version={a.version} -X mybuilds/internal/version.Commit={commit} -X mybuilds/internal/version.BuildDate={datetime.now(timezone.utc).isoformat()}'
for target in targets:
    system, arch = target.split('/')
    if system not in ('darwin','linux','windows') or arch not in ('arm64','amd64'):
        p.error('仅支持 darwin/linux/windows 的 arm64/amd64')
    if system == 'darwin' and platform.system() != 'Darwin':
        p.error('macOS 包需在 macOS 上启用 cgo 构建')
    with tempfile.TemporaryDirectory() as tmp:
        package = Path(tmp)
        (package/'bin').mkdir()
        env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED='1' if system=='darwin' else '0')
        names = ['mybuilds'] if system=='windows' else ['mybuilds','mybuilds-server','mybuilds-agent']
        for name in names:
            dest = package/'bin'/(name + ('.exe' if system=='windows' else ''))
            subprocess.run(['go','build','-trimpath','-ldflags',flags,'-o',str(dest),'./cmd/'+name],cwd=root,env=env,check=True)
        shutil.copytree(root/'skills',package/'skills')
        (package/'scripts').mkdir()
        for name in ('install.sh','install.py'):
            shutil.copy2(root/'scripts'/name,package/'scripts'/name)
        (package/'VERSION').write_text(a.version+'\n')
        base = f'mybuilds_{a.version}_{system}_{arch}'
        if system=='windows':
            output=a.output/(base+'.zip')
            with zipfile.ZipFile(output,'w',zipfile.ZIP_DEFLATED) as z:
                for f in package.rglob('*'):
                    if f.is_file():z.write(f,f.relative_to(package))
        else:
            output=a.output/(base+'.tar.gz')
            with tarfile.open(output,'w:gz') as t:
                for f in sorted(package.iterdir()):t.add(f,arcname=f.name)
        print(output,flush=True)
# 汇总输出目录所有平台包，允许分批构建。
artifacts=sorted(list(a.output.glob(f'mybuilds_{a.version}_*.tar.gz'))+list(a.output.glob(f'mybuilds_{a.version}_*.zip')))
(a.output/'SHA256SUMS').write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest()+'  '+f.name+'\n' for f in artifacts))
