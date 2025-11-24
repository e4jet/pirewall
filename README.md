# pirewall: pi - er - wall

A Raspberry Pi based firewall.

## What

This is known to run well on 4GiB Pi 4 (USB 3 ports used for networks).  Likely to run on Pi 3 and 5 as well.

pirewall is a simple utility that configures a new install of [Raspberry Pi OS Lite](https://www.raspberrypi.com/software/) to act as a firewall.  The target is the 64-bit version, based on Debian 12 (a.k.a. bookworm).

Please note that pirewall **is not** involved in the data path at all.  It simply configures the many features of the linux kernel to provide the functionality.  Its run once and then configures the services.

It is also extremely opinionated (essentially based on my needs).  This uses the built-in ethernet adaptor  and a USB3 based network adaptor. The wifi and bluetooth are disabled.

## Why

I manually rebuild my personal firewall every few years.  The Raspberry Pi devices and Debian based OSes have proven quite good.  The basics don't change, so I've automated them.  This should allow others to leverage this.

## Why not Ansible (or insert your favorite tool)

I'm a big fan of Ansible, however it is overkill for what I'm after here.  The goal of this project is to drop off a tiny binary that configures a Raspberry Pi for use as a firewall.

## Why iptables and not nftables

My provider doesn't support ipv6 yet, nor does it provide enough bandwidth to make some of the efficiencies of nftables worth the switch.  I'm sure we'll get there eventually, but for now, the [tables](examples/rules.v4) configured in pirewall are tried and trusted.

## How does it work

### Configuration

- [ ] Create the installer
- [X] Packages that are not needed are removed
- [ ] Packages that are needed are added
- [X] New Services are started and enabled
  - unattended-upgrades
- [X] Old Services are stopped and disabled
  - bluetooth
  - sound.target
- [X] The kernel is configured for packet routing and safety
- [ ] The network is configured
- [ ] The example rules are put in place
- [ ] Basic QOS
- [X] Device is configured
- [ ] Automate service fixes

## Examples

The examples directory contains some simple examples of configuration files.

- [rules.v4](examples/rules.v4) provides an iptables rule set for ipv4
  - copy to `/etc/iptables`
- [rules.v6](examples/rules.v6) provides an iptables rule set for ipv6 (drop everything)
  - copy to `/etc/iptables`
- [dns.conf](examples/dns.conf) provides basic dns settings for dnsmasq
  - copy to `/etc/dnsmasq.d`
- [dhcp.conf](examples/dhcp.conf) provides basic dhcp settings for dnsmasq
  - copy to `/etc/dnsmasq.d`
- [01-network.yaml](examples/01-network.yaml) provides a basic 2 interface example.  One interface has a static ip and the other uses dhcp (usually provided by the ISP)
  - copy to `/etc/netplan`

## netplan

The [Raspberry Pi OS](https://www.raspberrypi.com/software/) comes with [Network Manager](https://networkmanager.dev/).  [NetPlan](https://netplan.readthedocs.io/en/stable/) can leverage Network Manager as a backend.  This project leverages [NetPlan](https://netplan.readthedocs.io/en/stable/) for interface management.  This allows for easy configuration of advanced features like vlans and bridges.

## iptables

Network Address Translation (nat) is used by the firewall to allow multiple hosts behind the firewall to "share" a single public ip address (which is assigned to eth0).

The following diagram depicts the association between tables and interfaces in the default [ruleset](iptables/rules.v4_example).

![iptables](./doc/iptables.png)

When a packet is destined for the firewall, it is handled by the INPUT table, which defaults to DROPping the packet.  Our first rule in the INPUT table is to "jump" to the "public" table when a packet shows up on eth0.  The packet is passed through the 'public' table, which determines if the packet should be ACCEPTED, REJECTED or DROPPED.  In most cases this is DROPPED as the firewall doesn't provide services to the outside world.

![in](./doc/in.png)

Similarly, when a packet is destined for something behind the firewall, it is handled by the FORWARD table, which also defaults to DROPping the packet.  The first rule in the FORWARD table is to "jump" to the "public" table when a packet shows up on eth0 (which is plugged into our provider).  The packet is passed through the 'public' table, which determines if the packet should be ACCEPTED, REJECTED or DROPPED.  If it is ACCEPTED, the packet is routed (having been translated) to the host behind the firewall.

![in](./doc/fwdIn.png)

When a packet is destined for something on the public network, it is also handled by the FORWARD table, which defaults to DROPping the packet.  There is a rule in the FOWARD table that "jumps" to the "trusted" table when a packet is received on eth1 (which is plugged into out internal network).  The packet is passed through the 'trusted' table, which determines if the packet should be ACCEPTED, REJECTED or DROPPED.  If it is ACCEPTED, the packet is routed (having been translated) to the on the public network.

![in](./doc/fwdOut.png)

## dnsmasq

[dnsmasq](https://thekelleys.org.uk/dnsmasq/doc.html) dnsmasq is a versatile and efficient tool for managing DNS and DHCP services in small to medium-sized networks.  The is an optional service that is installed and ready to be configured.

## ddclient

[ddclient](https://ddclient.net/) is a useful tool for a number of use cases.  The is an option service that is installed and ready to be configured.

## Utilities

### rebootOnWatchdog

`bin/rebootOnWatchdog` is a script that is intended to be run out of cron.  It leverages journalctl to watch for watchdog errors on the network devices.  If found, it reboot the os (init 6).

This script also ensures sshd has started.

### mirrorConfig

`bin/mirrorConfig` is a script that copies files into the `~/fw` directory.  To get files "pulled" into this directory, simply touch the filename with the corresponding path.

### backupConfig

`bin/backupConfig` leverages [mirrorConfig](#mirrorconfig) to copy the latest version of the file into `~/fw`.  It then commits the latest version into git.

This script requires root level access.  The git repo must be configured prior to using backupConfig.  The following steps are required.

1. Initialize the repo
   - `~/fw $ git init`
1. Configure git for the the root user
   - `~ $ sudo git config --global user.email "you@example.com"`
   - `~ $ sudo git config --global user.name "Your Name"`

Example run:

```bash
$ sudo ./bin/backupConfig pi
cp /etc/netplan/01-network.yaml /home/pi/fw/etc/netplan/01-network.yaml  ...Success.
cp /etc/sysctl.conf /home/pi/fw/etc/sysctl.conf  ...Success.
cp /etc/ddclient.conf /home/pi/fw/etc/ddclient.conf  ...Success.
cp /etc/dnsmasq.d/host.local /home/pi/fw/etc/dnsmasq.d/host.local  ...Success.
cp /etc/dnsmasq.d/dns.conf /home/pi/fw/etc/dnsmasq.d/dns.conf  ...Success.
cp /etc/dnsmasq.d/dhcp.conf /home/pi/fw/etc/dnsmasq.d/dhcp.conf  ...Success.
cp /etc/ssh/sshd_config /home/pi/fw/etc/ssh/sshd_config  ...Success.
cp /etc/iptables/rules.v6 /home/pi/fw/etc/iptables/rules.v6  ...Success.
cp /etc/iptables/rules.v4 /home/pi/fw/etc/iptables/rules.v4  ...Success.
chmod -R 700 /home/pi/fw ...Success.
chown -R pi /home/pi/fw ...Success.
[master (root-commit) d738497] auto commit
 10 files changed, 344 insertions(+)
 create mode 100755 etc/ddclient.conf
 create mode 100755 etc/dnsmasq.d/dhcp.conf
 create mode 100755 etc/dnsmasq.d/dns.conf
 create mode 100755 etc/dnsmasq.d/host.local
 create mode 100755 etc/iptables/rules.v4
 create mode 100755 etc/iptables/rules.v6
 create mode 100755 etc/netplan/01-network.yaml
 create mode 100755 etc/ssh/sshd_config
 create mode 100755 etc/sysctl.conf
 create mode 100755 var/lib/misc/dnsmasq.leases
```

### Tools

#### bmon

Leverage bmon.

`/usr/bin/bmon`

## Install

Leverage the [Raspberry Pi installer](https://www.raspberrypi.com/software/) to install Raspberry Pi OS Lite.  Using this tool, configure a different user name (not pi), add an ssh key, and disable ssh interactive authentication.

After the new image is used to boot the pi, download [install.tgz](https://github.com/e4jet/pirewall/raw/refs/heads/main/install.tgz).

```bash
$ sha512sum install.tgz
2249b4f0c30113f45407932a087585f5429eb67d1e36e68ce97f358364da36f34e03e3cd44c5424b14dc20e26130d89a798618031cb19fc0d52aa7def4a88e23  install.tgz
```

Use tar to extract the tools and then remove the archive.

- `$ tar -xvf install.tgz`
- `$ cd install/`
- `$ mv bin ../`
- `$ mv fw ../`
- `$ cd ..`
- `$ rmdir install/`

Run the pirewall binary.

```bash
$ sudo ./bin/pirewall
pirewall
Adjusting settings using raspi-conf.
	running 👉/usr/bin/raspi-config nonint do_blanking 0
		😎 process with pid: 25857 finished successfully
	running 👉/usr/bin/raspi-config nonint do_fan 0 14 80
		😎 process with pid: 25871 finished successfully
	running 👉/usr/bin/raspi-config nonint do_net_names 0
		😎 process with pid: 25894 finished successfully
	running 👉/usr/bin/raspi-config nonint do_change_locale en_US.UTF-8 UTF-8
		😎 process with pid: 25908 finished successfully
	running 👉/usr/bin/raspi-config nonint do_change_timezone America/New_York
		😎 process with pid: 26479 finished successfully
Removing packages that aren\'t needed.
	running 👉/usr/bin/apt-get purge -y libx11.* libqt.* aardvark-dns wireless-* triggerhappy avahi-daemon
		😎 process with pid: 26650 finished successfully
	running 👉/usr/bin/apt-get update
		😎 process with pid: 26653 finished successfully
	running 👉/usr/bin/apt-get upgrade -y
		😎 process with pid: 26994 finished successfully
	running 👉/usr/bin/apt-get autopurge -y
		😎 process with pid: 26997 finished successfully
	running 👉/usr/bin/apt-get upgrade -y
		😎 process with pid: 27000 finished successfully
Adding useful packages.
	running 👉/usr/bin/apt-get update
		😎 process with pid: 27003 finished successfully
	running 👉/usr/bin/apt-get upgrade -y
		😎 process with pid: 27344 finished successfully
	running 👉/usr/bin/apt-get --yes install -yqq bmon dnsmasq dnsutils iptables-persistent git unattended-upgrades apt-listchanges vlan netplan.io ddclient
		😎 process with pid: 27347 finished successfully
Enabling new services.
	running 👉/usr/bin/systemctl start unattended-upgrades
		😎 process with pid: 27350 finished successfully
	running 👉/usr/bin/systemctl enable unattended-upgrades
		😎 process with pid: 27351 finished successfully
Disabling unneeded services.
	running 👉/usr/bin/systemctl stop bluetooth
		😎 process with pid: 27412 finished successfully
	running 👉/usr/bin/systemctl disable bluetooth
		😎 process with pid: 27415 finished successfully
	running 👉/usr/bin/systemctl stop sound.target
		😎 process with pid: 27476 finished successfully
	running 👉/usr/bin/systemctl disable sound.target
		😎 process with pid: 27477 finished successfully
Adjusting /etc/sysctl.conf.
Done!
```

## Cleanup

### sshd

Make sure sshd is only listening on the private interface.  Example below.

``` bash
$ grep ListenAddress /etc/ssh/sshd_config
ListenAddress 10.10.10.1
```

### config cron

Leverage root's cron table to backup the configuration.

`sudo crontab -e`

``` crontab
* * * * * /home/pi/bin/backupConfig pi > /tmp/backupConfig 2>&1
* * * * * /home/pi/bin/rebootOnWatchdog > /var/tmp/rebootOnWatchdog 2>&1
```

Note that a better username should be used above instead of pi.

### Fix systemd targets

- Change `network.target` to `network-online.target` in `/etc/systemd/system/multi-user.target.wants/ssh.service`
- Change `network.target` to `network-online.target` in `/etc/systemd/system/multi-user.target.wants/dnsmasq.service`
