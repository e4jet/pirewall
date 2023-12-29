# pirewall: pi - er - wall

A Raspberry Pi based firewall.

## What

This is known to run well on 4GiB Pi 4 (USB 3 ports used for networks).  Likely to run on Pi 3 and 5 as well.

pirewall is a simple utility that configures a new install of [Raspberry Pi OS Lite](https://www.raspberrypi.com/software/) to act as a firewall.  The target is the 64-bit version, based on Debian 12 (a.k.a. bookworm).

Please note that pirewall **is not** involved in the data path at all.  It simply configures the many features of the linux kernel to provide the functionality.

It is also extremely opinionated (essentially based on my needs).  The built-in ethernet adaptor should "face" the public network (plug this into your cable modem).  The USB3 based network adaptor(s) should "face" your internal network(s).  The wifi and bluetooth are disabled.

## Why

I manually rebuild my personal firewall every few years.  The Raspberry Pi devices and Debian based OSes have proven quite good.  The basics don't change, so I've automated them.  This should allow others to leverage this.

## Why not Ansible (or insert your favorite tool)

I'm a big fan of Ansible, however it is overkill for what I'm after here.  The goal of this project is to drop off a tiny binary that configures a Raspberry Pi for use as a firewall.

## Why iptables and not nftables

My provider doesn't support ipv6 yet, nor does it provide enough bandwidth to make some of the efficiencies of nftables worth the switch.  I'm sure we'll get there eventually, but for now, the [tables](iptables/rules.v4_example) configured in pirewall are tried and trusted.

## How does it work

### Configuration

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

## iptables

Network Address Translation (nat) is used by the firewall to allow multiple hosts behind the firewall to "share" a single public ip address (which is assigned to eth0).

The following diagram depicts the association between tables and interfaces in the default [ruleset](iptables/rules.v4_example).

![iptables](./doc/iptables.png)

When a packet is destined for the firewall, it is handled by the INPUT table, which defaults to DROPping the packet.  Our first rule in the INPUT table is to "jump" to the "public" table when a packet shows up on eth0.  The packet is passed through the 'public' table, which determines if the packet should be ACCEPTED, REJECTED or DROPPED.  In most cases this is DROPPED as the firewall doesn't provide services to the outside world.

![in](./doc/in.png)

Similarly, when a packet is destined for something behind the firewall, it is handled by the FORWARD table, which also defaults to DROPping the packet.  The first rule in the FORWARD table is to "jump" to the "public" table when a packet shows up on eth0 (which is plugged into our provider).  The packet is passed through the 'public' table, which determines if the packet should be ACCEPTED, REJECTED or DROPPED.  If it is ACCEPTED, the packet is routed (having been translated) to the host behind the firewall.

![in](./doc/fwdin.png)

When a packet is destined for something on the public network, it is also handled by the FORWARD table, which defaults to DROPping the packet.  There is a rule in the FOWARD table that "jumps" to the "trusted" table when a packet is received on eth1 (which is plugged into out internal network).  The packet is passed through the 'trusted' table, which determines if the packet should be ACCEPTED, REJECTED or DROPPED.  If it is ACCEPTED, the packet is routed (having been translated) to the on the public network.

![in](./doc/fwdOut.png)