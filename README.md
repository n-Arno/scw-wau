scw-wau (Watch and Update)
==========================

This small utility read Scaleway Instance metadata and update PN nics according to a configuration file.

It is intended to be run in the background and will watch for change in private nics metadata and update nics ip and routes accordingly.

Building
--------

You can use gnu/make or just run `go build`.

If you are using gnu/make, you can run `make install` to build, install binary in /usr/local/bin and install service.

In any case, once build, the service can be installed/started/stopped using daemon commands:


```
# scw-wau help

Usage: scw-wau install | remove | start | stop | status | help
```

Once installed, the service can also be managed through standard `systemd` commands:

```
# systemctl status scw-wau

● scw-wau.service - Scaleway PN Watch and Update
     Loaded: loaded (/etc/systemd/system/scw-wau.service; enabled; vendor preset: enabled)
     Active: active (running) since Sun 2022-12-18 12:59:37 UTC; 5s ago
    Process: 25250 ExecStartPre=/bin/rm -f /var/run/scw-wau.pid (code=exited, status=0/SUCCESS)
   Main PID: 25251 (scw-wau)
      Tasks: 8 (limit: 1112)
     Memory: 1.3M
        CPU: 15ms
     CGroup: /system.slice/scw-wau.service
             └─25251 /usr/local/bin/scw-wau
```

Config
------

An example config file is provided:

```
pns:
  - id: 47d39527-08ea-4956-b581-ed7a2e73b69e
    ip: 192.168.0.1/24
  - id: 513a6cf1-8f47-403b-9aaa-6815c5654fb6
    ip: 192.168.1.1/24
    ex: "echo done > /tmp/updated-pn"
routes:
  - 192.168.3.0/24 via 192.168.1.2
```

In pns, multiple nic can be configured using the following values:

- `id`: Mandatory private network id
- `ip`: Mandatory ip address for the corresponding nic, in CIDR format
- `ex`: Optionnal command to execute if nic is configured, for example to start an associated service

Some routes can be configured using the `routes` list. Each route must be in the format `<destination network> via <gateway ip>`.

The default route can be configured using `default` as destination network, but be aware it delete the existing default route. Use at your own risk.

Running
-------

scw-wau can be run standalone, as a background process or as a service (once installed).

Here is the sample log output of the command:

```
2022/12/18 13:09:46 Reading config file pn.yaml.example...
2022/12/18 13:09:46 Done!
2022/12/18 13:09:46 Starting pooling every 10 seconds
2022/12/18 13:09:46 New private nics state: [{47d39527-08ea-4956-b581-ed7a2e73b69e 02:00:00:11:c4:68} {513a6cf1-8f47-403b-9aaa-6815c5654fb6 02:00:00:11:c3:dc}]
2022/12/18 13:09:46 Found interface ens6 with mac address 02:00:00:11:c3:dc
2022/12/18 13:09:46 Associated config exists for ens6 (192.168.1.1/24)
2022/12/18 13:09:46 Found interface ens5 with mac address 02:00:00:11:c4:68
2022/12/18 13:09:46 Associated config exists for ens5 (192.168.0.1/24)
2022/12/18 13:09:46 Added 192.168.1.1/24 to ens6
2022/12/18 13:09:46 Added 192.168.0.1/24 to ens5
2022/12/18 13:09:46 Run 'echo done > /tmp/updated-pn' with success
2022/12/18 13:09:46 Added route 192.168.3.0/24 via 192.168.1.2
```

Two flags can be passed to the main command:

```
# scw-wau --help

Usage of scw-wau:
  -c string
    	config filename (default "/etc/scw-wau/pn.yaml")
  -p int
    	pooling time (default 10)
```

Preparation
-----------

Scaleway instances already have some mecanism to help private network nics configuration.

To use scw-wau, it is preferable to deactivate these mecanism, here is an example on how on ubuntu instances:

```
rm -f /lib/systemd/system/scw-vpc-iface@.service
systemctl daemon-reload

cat<<EOF>/etc/cloud/cloud.cfg.d/99-custom-networking.cfg
{config: disabled}
EOF

cat<<EOF>/etc/netplan/custom-config.yaml
network:
  version: 2
  ethernets:
    ens2:
      dhcp4: true
EOF

rm -f /etc/netplan/50-cloud-init.yaml
netplan generate
netplan apply
```
