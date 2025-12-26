How to connect the pluto to Ethernet?

This is a really stupid issue with the pluto plus SDR or any of its variants...
This costed me a lot of time to figure out including reverse engineering what happens inside the pluto... 

Debug steps below!

To work this comfortably, you need (you can do with less, but it is less comfortable):
1 network cable, 
2 usb cables. 
1 usb-c power adapter, please remark that this can better be stronger then 500mA

Power the board using the power adapter through the 1 usb-c port meant for power. 
If you don't know which one it is, it is closest to the network port.

Then connect it to your pc using the other usb-c port (the usb-c port closest to the edge). 
It mounts the sdcard in "this pc", and open config.txt

Under the network settings you will see (as default):
```bash
[NETWORK]
hostname = plutosdr
ipaddr = 192.168.2.1
ipaddr_host = 192.168.2.10
netmask = 255.255.255.0

[WLAN]
ssid_wlan = 
pwd_wlan = 
ipaddr_wlan = 

[USB_ETHERNET]
ipaddr_eth = 
netmask_eth = 255.255.255.0

[SYSTEM]
xo_correction = 
udc_handle_suspend = 0
# USB Communication Device Class Compatibility Mode [rndis|ncm|ecm]
usb_ethernet_mode = rndis

[ACTIONS]
diagnostic_report = 0
dfu = 0
reset = 0
calibrate = 0
```
Empty the pluto SDR ethernet ip adress settings. 
Emptying makes it go in DHCP mode. 
Save the file. 

EJECT the sdcard(right mouse button in windows, click EJECT)!!!

Connect the network cable 
If all is well you will recieve an IP adress. 


If all is not well, which often is the case:
- Check the usb-c powersupply (is it large enough?), too small,  too little power. 
It needs more power then a phone! Buy a large thing. 
It will give ethernet stutters (or one minute it is there, next minute not). 

Next step:
- Powercycle the pluto, power-off, power-on. 

If it still not works:
- Seperate the ethernet settings. choose a different subnet for the USB adapter to make sure no colisions are there. 
For instance 192.168.2.23 for the ethernet adapter (with a subnet 255.255.255.0) 
For the usb subnet then for instance choose ip range/subnet 192.168.3.1 with 255.255.255.0

- Use nslookup (in cmd) to check if the dns resolves
If you don't space it appart, it will look like this:
```batch
> nslookup plutosdr.home
Server:  mijnmodem.kpn
Address:  ipv6:xxxx:xxxx:0:xxxx:xxxx:xxxx:xxxx

Name:    plutosdr.home
Addresses:  192.168.2.50
          192.168.2.23
```

when done well it looks like this:
username: root, default password: analog
```bash
Welcome to:
______ _       _        _________________
| ___ \ |     | |      /  ___|  _  \ ___ \
| |_/ / |_   _| |_ ___ \ `--.| | | | |_/ /
|  __/| | | | | __/ _ \ `--. \ | | |    /
| |   | | |_| | || (_) /\__/ / |/ /| |\ \
\_|   |_|\__,_|\__\___/\____/|___/ \_| \_|

v0.38-1-g6f8e-dirty
https://wiki.analog.com/university/tools/pluto
# ifconfig
eth0      Link encap:Ethernet  HWaddr 66:49:BD:2C:8F:2A
          inet addr:192.168.2.23  Bcast:0.0.0.0  Mask:255.255.255.0
          UP BROADCAST MULTICAST  MTU:1500  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)
          Interrupt:35 Base address:0xb000

lo        Link encap:Local Loopback
          inet addr:127.0.0.1  Mask:255.0.0.0
          UP LOOPBACK RUNNING  MTU:65536  Metric:1
          RX packets:18 errors:0 dropped:0 overruns:0 frame:0
          TX packets:18 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:1979 (1.9 KiB)  TX bytes:1979 (1.9 KiB)

usb0      Link encap:Ethernet  HWaddr 00:05:F7:6F:C4:23
          inet addr:192.168.3.1  Bcast:0.0.0.0  Mask:255.255.255.0
          UP BROADCAST RUNNING MULTICAST  MTU:1500  Metric:1
          RX packets:506 errors:0 dropped:168 overruns:0 frame:0
          TX packets:221 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:67447 (65.8 KiB)  TX bytes:144068 (140.6 KiB)

```
If you cannot find it then:
- us the arp table
in CMD type:
```batch
arp -a
```
it will look like this:
```batch
Interface: 192.168.2.8 --- 0x1b
  Internet Address      Physical Address      Type
  192.168.2.2           10-2c-6b-45-c1-5c     dynamic
  192.168.2.6           00-02-9b-f4-e5-68     dynamic
  192.168.2.19          f6-39-0c-a6-b0-5a     dynamic
  192.168.2.23          f6-39-0c-a6-b0-5a     dynamic
  192.168.2.254         64-cc-22-6e-9d-2f     dynamic
  192.168.2.255         ff-ff-ff-ff-ff-ff     static
  224.0.0.2             01-00-5e-00-00-02     static
  224.0.0.251           01-00-5e-00-00-fb     static
  224.0.0.252           01-00-5e-00-00-fc     static
  239.255.255.250       01-00-5e-7f-ff-fa     static
  255.255.255.255       ff-ff-ff-ff-ff-ff     static
```

If you have plugged it in multiple times (on and off power) it might have gotten a new IP adress, look for ip adresses that have the same Physical adress. 
In my example the 192.168.2.19 and the .23, check both. The physical adresses don't nessesarily match the pluto.

- Wireshark: 
If you still cant find the pluto or the arp table is huge or spanning a large subnet... then use wireshark.
disconnect the pluto from the ethernetport. 
Setup wireshark. 
I used this filter...:

```
(ip.dst_host matches pluto)  || (ip.src_host matches pluto)
```
Plug the pluto in, and find your sdr. 


- Powershell TCP ping
In order to check if the right port is available.
```powershell
Using powershell you can then test if the tcp port is open:
Test-NetConnection 192.168.2.23 -Port 30431
```


