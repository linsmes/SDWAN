import paramiko, time

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect('121.40.193.74', username='root', password='pT89SWni4D*D~J%a122', timeout=30)

# restart controller to clear in-memory fail limit
ssh.exec_command('pkill -f aleiyun_controller_Linux_X64; sleep 1')
time.sleep(2)
ssh.exec_command('cd /opt/SDWAN && nohup ./aleiyun_controller_Linux_X64 -addr :52888 -data /var/lib/aleiyun/data.json -admin-db /var/lib/aleiyun/admin.json -admin-user linsme -admin-pass as789456 > /var/log/aleiyun_controller.log 2>&1 &')
time.sleep(2)

login = '''curl -s -c /tmp/cookie.txt -X POST -H "Content-Type: application/json" -d '{"username":"linsme","password":"a101365e8f86a8dfc67857748da07345"}' http://127.0.0.1:52888/api/admin/login -w "\\nHTTP:%{http_code}\\n"'''
stdin, stdout, stderr = ssh.exec_command(login)
print('login:', stdout.read().decode())

stdin, stdout, stderr = ssh.exec_command('curl -s -b /tmp/cookie.txt http://127.0.0.1:52888/api/relays')
print('relays:', stdout.read().decode())

ssh.close()
