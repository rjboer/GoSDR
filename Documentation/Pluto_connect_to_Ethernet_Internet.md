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
```
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
- Seperate the ethernet settings. choose a different subnet for the USB adapter to make sure no colisions are there. 
For instance 192.168.2.23 for the ethernet adapter (with a subnet 255.255.255.0) . 
For the usb subnet then for instance choose ip range/subnet 192.168.3.1 with 255.255.255.0













