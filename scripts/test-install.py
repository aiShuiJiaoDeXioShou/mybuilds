#!/usr/bin/env python3
"""隔离验收：初始化、幂等、凭据权限、在线Agent与拒绝覆盖。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import tarfile
import tempfile
import time

p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--archive',type=Path,required=True)
a=p.parse_args()
root=Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix='mybuilds-install-test-') as tmp:
    base=Path(tmp)
    package=base/'package'
    with tarfile.open(a.archive) as archive:archive.extractall(package)
    env={k:v for k,v in os.environ.items() if not k.startswith('MYBUILDS_')}
    def call(args,ok=True):
        result=subprocess.run([str(v) for v in args],capture_output=True,text=True,env=env,timeout=45)
        assert (result.returncode==0)==ok,(args,result.stdout,result.stderr)
        return result.stdout
    dest=base/'with space'
    bin=dest/'bin'; cfg=dest/'config'; skills=dest/'skills'
    with socket.socket() as s:s.bind(('127.0.0.1',0)); port=s.getsockname()[1]
    install=['bash',package/'scripts/install.sh','server','--with-agent','--service','none','--port',port,'--bin-dir',bin,'--config-dir',cfg,'--skills-dir',skills,'--no-path']
    first=call(install)
    before={f.name:f.read_bytes() for f in cfg.glob('*.yml')}
    assert len(before)==3
    for name in ('client','agent'):
        value=json.loads((cfg/(name+'.yml')).read_text())
        assert value['token'] not in first
        assert (cfg/(name+'.yml')).stat().st_mode & 0o777 == 0o600
    assert json.loads(before['client.yml'])['token'] != json.loads(before['agent.yml'])['token']
    call(install)
    assert before=={f.name:f.read_bytes() for f in cfg.glob('*.yml')}
    print('PASS 初始化、独立身份、私有文件、空格路径、重复安装',flush=True)
    processes=[]
    logs=[]
    try:
        for role in ('server','agent'):
            f=(base/(role+'.log')).open('w');logs.append(f)
            processes.append(subprocess.Popen([str(bin/('mybuilds-'+role)),'--config',str(cfg/(role+'.yml')),'serve'],env=env,stdout=f,stderr=f))
            if role=='server':
                for _ in range(40):
                    r=subprocess.run([str(bin/'mybuilds'),'--config',str(cfg/'client.yml'),'status','--json'],capture_output=True,env=env)
                    if r.returncode==0:break
                    time.sleep(.25)
                else:raise AssertionError('控制端未就绪')
        for _ in range(90):
            node=json.loads(call([bin/'mybuilds','--config',cfg/'client.yml','node','show','local','--json']))
            if node.get('session_active'):break
            time.sleep(1)
        else:raise AssertionError('Agent未上线')
        call(install)
        assert before=={f.name:f.read_bytes() for f in cfg.glob('*.yml')}
        print('PASS 真实控制端、Agent在线、在线重复安装',flush=True)
    finally:
        for process in processes:process.terminate()
        for process in processes:
            try:process.wait(timeout=12)
            except subprocess.TimeoutExpired:process.kill();process.wait()
        for f in logs:f.close()
    other=base/'client'
    token=base/'token';token.write_text('x'*40);token.chmod(0o600)
    client=['bash',package/'scripts/install.sh','client','--bin-dir',other/'bin','--config-dir',other/'cfg','--skills-dir',other/'skills','--server-url','http://127.0.0.1:8787','--token-file',token,'--no-path']
    call(client)
    original=(other/'cfg/client.yml').read_bytes()
    token.write_text('y'*40);call(client)
    assert (other/'cfg/client.yml').read_bytes()==original
    token.chmod(0o644);call(client,False)
    token.chmod(0o600)
    program=other/'bin/mybuilds';program.write_text('已有程序')
    call(client,False);assert program.read_text()=='已有程序'
    print('PASS 客户端导入、保留配置、拒绝宽权限凭据和不同已有程序',flush=True)
    # 模拟下载损坏，确保安装器在运行包内代码前拒绝。
    mock=base/'mock';mock.mkdir()
    curl=mock/'curl'
    curl.write_text('#!/usr/bin/env python3\nimport sys,pathlib\na=sys.argv\np=pathlib.Path(a[a.index("-o")+1])\np.write_bytes(b"corrupted package" if p.name.endswith("gz") else b"0  bad-name.tar.gz\\n")\n')
    curl.chmod(0o755)
    badenv=dict(env,PATH=str(mock)+os.pathsep+env['PATH'])
    result=subprocess.run(['bash',str(root/'scripts/install-client.sh'),'--no-path'],capture_output=True,text=True,env=badenv)
    assert result.returncode!=0 and 'SHA-256' in result.stderr
    print('PASS 损坏下载在安装前被拒绝',flush=True)
