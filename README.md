# sliver-clients
## Overview 
- In this repo are multiple custom Sliver clients that extend the current Sliver client.
- While the default Sliver client is very powerful, it does not contains a few required features. This repo attempts to solve those issues.
## Operator config
- These sliver clients (just like the main `sliver-client` require an operator config in order to connection
- On kali install sliver with `sudo apt install sliver`
- Once that is completed, launch the server and create an operator config
````
sliver-server
multiplayer
new-operator -l 127.0.0.1 --name default-local -s /tmp
[*] Generating new client certificate, please wait ... 
[*] Saved new client config to: /tmp/default-local_127.0.0.1.cfg
```` 
## Netstat watcher
- This program allows you to watch/poll connections in a different terminal than your main Sliver client.
- Navigate to `watchers/netstat` and run `go build`
````
go build                                               
go: downloading github.com/bishopfox/sliver v1.15.16
go: downloading github.com/jedib0t/go-pretty/v6 v6.6.1
go: downloading google.golang.org/grpc v1.68.0
--snip--
````
- Help menu
````
./netstat_watcher -h
Usage of ./netstat_watcher:
  -config string
        path to sliver client config file
  -sleep int
        the time to sleep in between process list polling (default 60)
````
- Run the program pointing to your `-config`, optionally specifying a polling interval
````
./netstat_watcher -config /tmp/default-local_127.0.0.1.cfg
2025/04/06 13:24:59 [*] Connected to sliver server
======================================================================
[*] Connections
======================================================================
+----------+-------------------+-----------------+--------+---------------------+
| PROTOCOL | LOCAL ADDRESS     | FOREIGN ADDRESS | STATE  | PID/PROGRAM NAME    |
+----------+-------------------+-----------------+--------+---------------------+
| udp      | 127.0.0.54:53     | 0.0.0.0:0       |        | 126/systemd-resolve |
| udp      | 127.0.0.53:53     | 0.0.0.0:0       |        | 126/systemd-resolve |
| udp      | 192.168.15.119:68 | 0.0.0.0:0       |        | 108/systemd-network |
| tcp      | 127.0.0.1:25      | 0.0.0.0:0       | LISTEN | 426/master          |
| tcp      | 127.0.0.53:53     | 0.0.0.0:0       | LISTEN | 126/systemd-resolve |
| tcp      | 127.0.0.54:53     | 0.0.0.0:0       | LISTEN | 126/systemd-resolve |
| tcp6     | ::1:25            | :::0            | LISTEN | 426/master          |
| tcp6     | :::22             | :::0            | LISTEN | 268/sshd            |
+----------+-------------------+-----------------+--------+---------------------+
````
## Process List Watcher
- Navigate to `watchers/ps` and run `go build`
- See below for help menu
````
./ps_watcher -h                                        
Usage of ./ps_watcher:
  -config string
        path to sliver client config file
  -sleep int
        the time to sleep in between process list polling (default 60)
````
- Run the ps watcher pointing it at your operator config
````
./ps_watcher -config /opt/sliver-clients/default-local_127.0.0.1.cfg
2025/04/06 13:29:51 [*] Connected to sliver server
======================================================================
[*] Process List
======================================================================

PPID        PID         USER             COMMAND
======      ====        =====            =========

1           0           root             /sbin/init 
44          1           root             /usr/lib/systemd/systemd-journald 
108         1           systemd-network  /usr/lib/systemd/systemd-networkd 
126         1           systemd-resolve  /usr/lib/systemd/systemd-resolved 
230         1           root             /usr/sbin/cron -f -P 
231         1           messagebus       @dbus-daemon --system --address=systemd: --nofork --nopidfile --systemd-activation --syslog-only 
234         1           root             /usr/bin/python3 /usr/bin/networkd-dispatcher --run-startup-triggers 
245         1           root             /usr/lib/systemd/systemd-logind 
261         1           root             /sbin/agetty -o -p -- \u --noclear --keep-baud - 115200,38400,9600 linux 
265         1           root             /bin/login -f --      
267         1           root             /sbin/agetty -o -p -- \u --noclear - linux 
268         1           root             sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups 
283         1           syslog           /usr/sbin/rsyslogd -n -iNONE 
426         1           root             /usr/lib/postfix/sbin/master -w 
428         426         postfix          qmgr -l -t unix -u 
449         265         root             -bash 
581         426         postfix          pickup -l -t unix -u -c 
627         268         root             sshd: root@pts/3  
634         1           root             /usr/lib/systemd/systemd --user 
644         627         root             -bash 
662         644         root             test-ubuntu.elf 
672         268         root             sshd: root@pts/4  
676         672         root             -bash 
````
## What the watchers look like while both running 
![image](https://github.com/user-attachments/assets/ec6c8675-ee3c-44d4-b3c7-2ec0aad2950a)

# Coming Soon
- Linux Survey 
- Windows Survey
- Custom downloader client







