import paramiko, sys, os, time

host = "121.40.193.74"
user = "root"
password = "pT89SWni4D*D~J%a122"
remote_dir = "/opt/SDWAN"

files = [
    ("D:/DDesktop/Code/SDWAN/aleiyun_controller_Linux_X64", f"{remote_dir}/aleiyun_controller_Linux_X64"),
    ("D:/DDesktop/Code/SDWAN/aleiyun_relay_Linux_X64", f"{remote_dir}/aleiyun_relay_Linux_X64"),
]

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(host, username=user, password=password, timeout=30)

# 确保目录存在并检查写入权限
stdin, stdout, stderr = ssh.exec_command(f"mkdir -p {remote_dir} && ls -ld {remote_dir}")
print("mkdir:", stdout.read().decode().strip())
print("mkdir err:", stderr.read().decode().strip())

sftp = ssh.open_sftp()
for local, remote in files:
    print(f"upload {local} -> {remote}")
    sftp.put(local, remote)
    sftp.chmod(remote, 0o755)
sftp.close()

cmds = [
    f"cd {remote_dir} && ./aleiyun_controller_Linux_X64 --help >/dev/null 2>&1 && echo controller ok || echo controller fail",
    f"cd {remote_dir} && ./aleiyun_relay_Linux_X64 --help >/dev/null 2>&1 && echo relay ok || echo relay fail",
]
for cmd in cmds:
    print(f"run: {cmd}")
    stdin, stdout, stderr = ssh.exec_command(cmd)
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    print(out)
    if err:
        print("stderr:", err)

ssh.close()
print("done")
