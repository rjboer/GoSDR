# Hardware Manual Test Procedures

Use the steps below to validate AD9361/Pluto integrations on real hardware. Each checklist is designed for quick operator execution without deep familiarity with the codebase.

## 1. Verify Network Connection
1. Connect the PlutoSDR USB/Ethernet and confirm the interface IP (default `192.168.2.1`).
2. From the host, ping the device:
   ```bash
   ping -c 3 192.168.2.1
   ```
3. Check that the IIOD port is reachable:
   ```bash
   nc -vz 192.168.2.1 30431
   ```
4. If either check fails, reseat the USB cable, confirm the host network route, and reboot the Pluto if needed.

## 2. Smoke-Test IQ Acquisition
1. Build and run the monopulse binary against hardware:
   ```bash
   go run ./cmd/monopulse \
     --sdr-backend=pluto \
     --sdr-uri=192.168.2.1:30431 \
     --sample-rate=2000000 \
     --rx-lo=2300000000 \
     --num-samples=4096
   ```
2. Watch stdout for a successful initialization log and at least one RX buffer report.
3. If RX fails, rerun with `GODEBUG=netdns=go` to avoid host resolver issues and ensure no other process has the buffer open.

## 3. Inject a Calibration Tone
1. Set up a continuous-wave source (signal generator or second SDR) offset ~200 kHz from the RX LO (e.g., 2.3002 GHz) into both antennas through a splitter.
2. Start the application with a matching `--tone-offset=200000` and known manual gains (e.g., `--rx-gain0=10 --rx-gain1=11`).
3. Confirm the coarse scan or spectrum print shows a clear peak at the injected offset within a few iterations.
4. Adjust generator level to check that the reported peak level scales predictably (roughly 6 dB change per 2x amplitude).

## 4. Check Phase Coherence Across Channels
1. Keep the calibration tone active on both antenna ports with equal cable lengths.
2. Run the binary with telemetry that exposes `GetPhaseDelta` or phase delay reporting.
3. Observe the phase difference between channels; it should be near 0° and stable. If a large static offset exists, note it as a calibration term.
4. Swap antenna cables to ensure the observed phase offset follows the cabling, not the channels; if not, re-seat SMA connectors and repeat.

## 5. Error-Recovery Drill
1. While streaming, briefly disconnect and reconnect the antenna or introduce an attenuator to force low-SNR reads.
2. Confirm the application logs an RX error but continues reading after conditions normalize, without requiring a restart.

Keep operator notes (signal levels, cable lengths, dates) alongside these steps for reproducibility.
