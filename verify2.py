import paramiko

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect('121.40.193.74', username='root', password='pT89SWni4D*D~J%a122', timeout=30)

stdin, stdout, stderr = ssh.exec_command('ps aux | grep aleiyun_controller | grep -v grep')
print('controller procs:', stdout.read().decode())

stdin, stdout, stderr = ssh.exec_command('cat /var/lib/aleiyun/admin.json')
print('admin json:', stdout.read().decode())

for pwd in ['as789456', 'a101365e8f86a8dfc67857748da07345', '22f2ab84889980e435dbf108442f5ad5']:
    login = f'''curl -s -c /tmp/cookie_{pwd[:6]}.txt -X POST -H "Content-Type: application/json" -d '{{"username":"linsme","password":"{pwd}"}}' http://127.0.0.1:52888/api/admin/login -w "\\nHTTP:%{{http_code}}\\n"'''
    stdin, stdout, stderr = ssh.exec_command(login)
    print('password', pwd[:20], '->', stdout.read().decode().strip())

ssh.close()
