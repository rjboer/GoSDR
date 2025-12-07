



if you see this:



```bash



RJBOER\\GoSDR> .\\monopulse.exe

2025/12/06 00:04:07 \[INFO] starting monopulse tracker subsystem=cli config=map\[debug\_mode:true history\_limit:500 log\_format:text log\_level:debug max\_tracks:1 min\_snr:0 mock\_phase\_delta:30 phase\_cal:0 phase\_step:1 rx\_gain0:60 rx\_gain1:60 rx\_lo:2.3e+09 sample\_rate:2e+06 scan\_step:2 sdr\_backend:pluto sdr\_uri:192.168.2.1:30431 spacing:1 tone\_offset:200000 track\_timeout:0s tracking\_length:100 tracking\_mode:single tx\_gain:-10 verbose:false warmup\_buffers:3 web\_addr::8080]

2025/12/06 00:04:07 \[INFO] selecting SDR backend subsystem=cli backend=pluto

2025/12/06 00:04:07 \[INFO] backend selected successfully subsystem=cli backend=pluto

2025/12/06 00:04:07 \[INFO] initializing telemetry hub subsystem=cli

2025/12/06 00:04:07 \[INFO] configuring Pluto SDR event logging subsystem=cli

2025/12/06 00:04:07 \[INFO] starting web server subsystem=cli addr=:8080

2025/12/06 00:04:07 \[INFO] web interface available subsystem=cli subsystem=telemetry addr=:8080

2025/12/06 00:04:07 \[INFO] creating tracker subsystem=cli

2025/12/06 00:04:07 \[INFO] initializing tracker (this may take a few seconds) subsystem=cli

\[PLUTO DEBUG] Init() called with URI=192.168.2.1:30431, SampleRate=2000000

\[PLUTO DEBUG] Attempting to connect to 192.168.2.1:30431...

\[PLUTO DEBUG] About to call iiod.Dial()...

\[PLUTO DEBUG] iiod.Dial() returned, err=<nil>

\[PLUTO DEBUG] Connected successfully!

\[PLUTO DEBUG] Failed to list devices: malformed reply header: "-22"

2025/12/06 00:04:07 \[ERROR] init tracker subsystem=cli subsystem=tracker error=init SDR: list devices: malformed reply header: "-22"



```

this means the return was not valid
The issue is that the IIOD server is returning -22 as the response header. This happens when the protocol parsing fails. 
Different IIOD versions use different command formats. 


There are two modes:
v0 compat (WITH_IIOD_V0_COMPAT=1) → ASCII / text protocol only.
New binary mode (WITH_IIOD_V0_COMPAT=0) → binary protocol enabled.

I currently use the text protocol only. 
I can support the binary protocol if needed (but i require a device that supports it, please gift me one and i will support it).