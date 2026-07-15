import paramiko

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect('121.40.193.74', username='root', password='pT89SWni4D*D~J%a122', timeout=30)

md5cmd = "python3 -c 'import hashlib; print(hashlib.md5(\"as789456\".encode()).hexdigest())'"
stdin, stdout, stderr = ssh.exec_command(md5cmd)
md5pw = stdout.read().decode().strip()
print('md5pw', md5pw)

login_cmd = f"curl -s -c /tmp/cookie.txt -X POST -H 'Content-Type: application/json' -d '{{\"username\":\"linsme\",\"password\":\"{md5pw}\"}}' http://127.0.0.1:52888/api/admin/login"
stdin, stdout, stderr = ssh.exec_command(login_cmd)
print('login:', stdout.read().decode(), stderr.read().decode())

stdin, stdout, stderr = ssh.exec_command('curl -s -b /tmp/cookie.txt http://127.0.0.1:52888/api/relays')
print('relays:', stdout.read().decode(), stderr.read().decode())

ssh.close()
