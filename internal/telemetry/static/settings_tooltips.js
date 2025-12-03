// Comprehensive tooltip data for all settings fields
const tooltipData = {
    sampleRateHz: {
        title: "IQ Sample Rate",
        definition: "The number of complex IQ samples captured per second from the SDR hardware. Each sample contains in-phase (I) and quadrature (Q) components representing the RF signal.",
        details: [
            "<strong>Bandwidth:</strong> Sample rate determines the observable bandwidth (Nyquist: BW = sample_rate / 2)",
            "<strong>CPU Load:</strong> Higher rates require more FFT computations and memory bandwidth",
            "<strong>Latency:</strong> Lower rates may increase processing delay",
            "<strong>Resolution:</strong> Combined with FFT size, determines frequency bin resolution (sample_rate / FFT_size)"
        ],
        examples: [
            { value: "2,000,000", desc: "2 MHz - Narrowband, low CPU, ±1 MHz bandwidth, ideal for single-frequency tracking" },
            { value: "10,000,000", desc: "10 MHz - Wideband, moderate CPU, ±5 MHz bandwidth, good for frequency-hopping signals" },
            { value: "20,000,000", desc: "20 MHz - Maximum bandwidth, high CPU load, full ISM band coverage" }
        ],
        tip: "Start with 2 MHz for most applications. Increase only if you need wider bandwidth or faster signal acquisition. Must match SDR hardware capabilities. For PlutoSDR: 520 kHz - 61.44 MHz range.",
        warning: "Very high sample rates (>30 MHz) may cause USB bandwidth saturation, dropped samples, or system instability on slower computers. Monitor CPU usage in Debug tab."
    },

    rxLoHz: {
        title: "Receiver Local Oscillator Frequency",
        definition: "The center frequency to which the SDR hardware tunes its receiver. This is the RF carrier frequency you want to monitor. The SDR will receive signals within ±(sample_rate/2) of this frequency.",
        details: [
            "<strong>Tuning Range:</strong> Depends on SDR hardware (PlutoSDR: 70 MHz - 6 GHz)",
            "<strong>Accuracy:</strong> Limited by SDR oscillator stability (typically ±1-20 ppm)",
            "<strong>Mixing:</strong> Hardware mixes RF signal down to baseband using this LO frequency"
        ],
        examples: [
            { value: "433,920,000", desc: "433.92 MHz - ISM band (Europe), remote controls, sensors" },
            { value: "915,000,000", desc: "915 MHz - ISM band (US), LoRa, ZigBee" },
            { value: "2,400,000,000", desc: "2.4 GHz - WiFi, Bluetooth, ISM band" }
        ],
        tip: "Use a frequency counter or spectrum analyzer to verify the exact frequency of your target signal. Account for Doppler shift for moving targets."
    },

    toneOffsetHz: {
        title: "Calibration Tone Frequency Offset",
        definition: "The frequency offset from the RX LO where the SDR transmits a calibration tone. This tone is used for system calibration and phase reference. Positive values place the tone above the LO, negative below.",
        details: [
            "<strong>Calibration:</strong> Provides a known reference signal for phase calibration",
            "<strong>Channel Balance:</strong> Helps verify equal gain/phase response between RX channels",
            "<strong>System Check:</strong> Confirms TX/RX chain is functioning"
        ],
        examples: [
            { value: "200,000", desc: "200 kHz - Standard offset, well separated from DC" },
            { value: "-200,000", desc: "-200 kHz - Negative offset, below LO" },
            { value: "500,000", desc: "500 kHz - Wider separation for high sample rates" }
        ],
        tip: "Use 200 kHz for most applications. Ensure the offset is within your sample rate bandwidth and away from DC (0 Hz) to avoid DC offset artifacts.",
        warning: "If tone offset is too close to 0 Hz or exceeds sample_rate/2, the calibration tone may not be visible in the spectrum, causing tracking failures."
    },

    spacingWavelength: {
        title: "Antenna Element Spacing",
        definition: "The physical distance between the two antenna elements, expressed in wavelengths (λ). Wavelength is calculated as λ = c / f, where c = speed of light (3×10⁸ m/s) and f = frequency in Hz.",
        details: [
            "<strong>0.5λ (optimal):</strong> Unambiguous bearing over full ±90° range, maximum sensitivity",
            "<strong>&lt;0.5λ:</strong> Reduced sensitivity, phase differences become smaller and harder to measure",
            "<strong>&gt;0.5λ:</strong> Phase ambiguity - multiple angles produce same phase difference (aliasing)",
            "<strong>Baseline:</strong> Longer baselines improve angular resolution but introduce ambiguity"
        ],
        examples: [
            { value: "0.5", desc: "At 2.4 GHz: λ = 12.5 cm, so 0.5λ = 6.25 cm spacing" },
            { value: "0.5", desc: "At 433 MHz: λ = 69.3 cm, so 0.5λ = 34.65 cm spacing" },
            { value: "0.5", desc: "At 915 MHz: λ = 32.8 cm, so 0.5λ = 16.4 cm spacing" }
        ],
        tip: "Use exactly 0.5λ for your operating frequency if possible. Measure physical spacing accurately (±1mm matters at high frequencies).",
        warning: "Incorrect spacing value will cause systematic bearing errors. If spacing > 0.5λ, you'll see multiple ambiguous solutions."
    },

    rxGain0: {
        title: "Receiver Gain Channel 0",
        definition: "Amplification applied to the received signal in decibels. Higher gain increases sensitivity to weak signals but may cause strong signals to saturate (clip), distorting phase measurements.",
        details: [
            "<strong>Phase Accuracy:</strong> Both channels MUST have identical gain for accurate angle estimation",
            "<strong>Amplitude Balance:</strong> Gain mismatch causes amplitude imbalance, affecting monopulse calculations",
            "<strong>Dynamic Range:</strong> Too high = saturation, too low = poor SNR"
        ],
        examples: [
            { value: "0", desc: "Minimum gain, use for very strong signals (local transmitters)" },
            { value: "40", desc: "Medium gain, general purpose (recommended)" },
            { value: "60", desc: "High gain, for weak/distant signals" }
        ],
        tip: "Start with 40 dB on both channels. Adjust based on signal strength visible in Trace tab. Both channels should show similar amplitude.",
        warning: "RX Gain Ch0 and RX Gain Ch1 MUST be identical! Even 1 dB difference will cause bearing errors."
    },

    rxGain1: {
        title: "Receiver Gain Channel 1",
        definition: "Amplification applied to the received signal in decibels for channel 1. MUST match RX Gain Ch0 exactly for accurate direction finding.",
        details: [
            "<strong>Critical Matching:</strong> Even 1 dB difference between channels causes bearing errors",
            "<strong>Phase Balance:</strong> Identical gains ensure phase measurements are not amplitude-dependent",
            "<strong>Calibration:</strong> Use spectrum view to verify both channels have equal amplitude"
        ],
        tip: "Always set this to the EXACT same value as RX Gain Ch0. Verify amplitude balance in Trace tab spectrum display.",
        warning: "This MUST match RX Gain Ch0! Mismatched gains are the #1 cause of bearing errors in direction finding systems."
    },

    txGain: {
        title: "Transmit Gain for Calibration Tone",
        definition: "Amplification applied to the transmitted calibration tone. Lower values reduce potential interference with other systems.",
        details: [
            "<strong>Interference:</strong> Higher TX gain may interfere with nearby receivers",
            "<strong>Calibration:</strong> Tone only needs to be strong enough for local reception",
            "<strong>Leakage:</strong> Some TX signal may leak into RX path (TX/RX isolation)"
        ],
        examples: [
            { value: "-20", desc: "Minimum power, very local calibration only" },
            { value: "-10", desc: "Low power, typical for bench testing (recommended)" },
            { value: "0", desc: "Medium power, use if calibration tone is too weak" }
        ],
        tip: "Use -10 dB for most applications. Increase only if calibration tone is not visible in spectrum. For MockSDR backend, this parameter is ignored."
    },

    numSamples: {
        title: "FFT Size (Samples)",
        definition: "Number of IQ samples used for each FFT computation. Must be a power of 2 (512, 1024, 2048, 4096, etc.). Determines frequency resolution and processing speed.",
        details: [
            "<strong>Frequency Resolution:</strong> Bin width = sample_rate / FFT_size (smaller bins = better resolution)",
            "<strong>Processing Speed:</strong> Larger FFTs take longer to compute (O(N log N) complexity)",
            "<strong>Time Resolution:</strong> Larger FFTs average over longer time periods",
            "<strong>Memory:</strong> FFT requires N complex numbers (8N bytes)"
        ],
        examples: [
            { value: "512", desc: "Fast updates, coarse frequency resolution, low memory" },
            { value: "1024", desc: "Balanced performance (recommended for most applications)" },
            { value: "2048", desc: "Better frequency resolution, moderate speed" },
            { value: "4096", desc: "High resolution, slower updates, more memory" }
        ],
        tip: "Start with 1024 for most applications. Increase for better frequency resolution, decrease for faster updates. At 2 MHz sample rate, 1024 FFT gives 1.95 kHz bins.",
        warning: "Very large FFTs (>8192) may cause excessive latency and memory usage. Must be power of 2 or FFT will fail."
    },

    bufferSize: {
        title: "SDR Streaming Buffer Size",
        definition: "Number of samples in the SDR USB streaming buffer. Must be power of 2 and ≥ FFT size. Affects USB transfer efficiency and latency.",
        details: [
            "<strong>USB Efficiency:</strong> Larger buffers reduce USB overhead and CPU interrupts",
            "<strong>Latency:</strong> Larger buffers increase delay between signal arrival and processing",
            "<strong>Reliability:</strong> Too small may cause buffer underruns and dropped samples"
        ],
        examples: [
            { value: "2048", desc: "Minimum buffer, lowest latency, may drop samples" },
            { value: "4096", desc: "Balanced (recommended), good efficiency and latency" },
            { value: "8192", desc: "Large buffer, maximum reliability, higher latency" }
        ],
        tip: "Use 4096 for most applications. Increase if you see dropped sample warnings. Must be ≥ FFT size.",
        warning: "Buffer size < FFT size will cause errors. Very large buffers (>16384) increase latency without much benefit."
    },

    warmupBuffers: {
        title: "Warmup Buffer Count",
        definition: "Number of initial sample buffers to discard before starting tracking. Allows SDR hardware, filters, and AGC to stabilize.",
        details: [
            "<strong>Hardware Settling:</strong> SDR needs time to stabilize after tuning to new frequency",
            "<strong>Filter Transients:</strong> Digital filters need time to fill their delay lines",
            "<strong>AGC Convergence:</strong> Automatic gain control needs time to adjust (if enabled)"
        ],
        examples: [
            { value: "0", desc: "No warmup, immediate tracking (may have initial errors)" },
            { value: "3", desc: "Standard warmup (recommended)" },
            { value: "10", desc: "Extended warmup for very stable initial conditions" }
        ],
        tip: "Use 3-5 for most applications. Increase if you see unstable readings at startup. Each buffer takes ~(buffer_size / sample_rate) seconds.",
        warning: "Too few warmup buffers may cause initial tracking errors. Too many delays system startup unnecessarily."
    },

    historyLimit: {
        title: "Telemetry History Limit",
        definition: "Maximum number of telemetry samples stored in memory for charts and history display. Each sample contains angle, SNR, confidence, and timestamp data.",
        details: [
            "<strong>Memory Usage:</strong> Each sample ~100-200 bytes, so 1000 samples ≈ 100-200 KB",
            "<strong>Chart Display:</strong> More history = longer time span visible in charts",
            "<strong>Performance:</strong> Very large histories may slow down chart rendering",
            "<strong>Time Span:</strong> At 60 Hz update rate, 500 samples = ~8 seconds of history"
        ],
        examples: [
            { value: "100", desc: "Minimal history, ~1.5 seconds at 60 Hz, low memory" },
            { value: "500", desc: "Standard history, ~8 seconds (recommended)" },
            { value: "1000", desc: "Extended history, ~16 seconds, more memory" },
            { value: "5000", desc: "Very long history, ~83 seconds, high memory usage" }
        ],
        tip: "Use 500-1000 for most applications. Increase for longer chart history, decrease to reduce memory usage. Consider your update rate when choosing.",
        warning: "Very large values (>10000) may cause UI slowdown and excessive memory usage, especially in multi-target mode."
    },

    trackingLength: {
        title: "Tracking Iterations (Legacy)",
        definition: "Number of tracking loop iterations before stopping. This is a legacy parameter - modern usage runs indefinitely until manually stopped.",
        details: [
            "<strong>Legacy Behavior:</strong> Originally limited tracking to N iterations for testing",
            "<strong>Modern Usage:</strong> Set to high value (10000+) for continuous operation",
            "<strong>Actual Control:</strong> Tracking stops when you stop the application, not when this limit is reached"
        ],
        examples: [
            { value: "50", desc: "Short test run, stops after 50 iterations" },
            { value: "10000", desc: "Effectively continuous (recommended)" },
            { value: "999999", desc: "Infinite for practical purposes" }
        ],
        tip: "Set to 10000 or higher for normal operation. This parameter is mostly ignored in modern versions - use Ctrl+C or stop button to end tracking."
    },

    trackingMode: {
        title: "Tracking Mode Selection",
        definition: "Determines whether the system tracks a single target (strongest signal) or multiple targets simultaneously with independent lifecycles.",
        details: [
            "<strong>Single Mode:</strong> Tracks only the highest SNR target, lowest CPU usage, classic behavior",
            "<strong>Multi Mode:</strong> Tracks up to max_tracks targets simultaneously, higher CPU/memory usage",
            "<strong>Track Lifecycle:</strong> In multi mode, tracks have states: Tentative → Confirmed → Lost",
            "<strong>Association:</strong> Multi mode uses nearest-neighbor association to link detections to tracks"
        ],
        examples: [
            { value: "single", desc: "One target, minimal CPU, simple display" },
            { value: "multi", desc: "Multiple targets, more CPU, comprehensive tracking" }
        ],
        tip: "Use single mode for simple applications or when CPU is limited. Use multi mode when you need to track multiple simultaneous targets or want track persistence.",
        warning: "Multi mode requires significantly more CPU (2-5x) and memory. Monitor system performance in Debug tab when using multi mode with many tracks."
    },

    maxTracks: {
        title: "Maximum Concurrent Tracks",
        definition: "Maximum number of targets to track simultaneously in multi-target mode. Each track maintains its own history, state, and lifecycle.",
        details: [
            "<strong>CPU Impact:</strong> Each track requires monopulse processing, ~linear CPU scaling",
            "<strong>Memory Impact:</strong> Each track stores history (angle, SNR, confidence over time)",
            "<strong>UI Impact:</strong> More tracks = more complex radar display and track table",
            "<strong>Association:</strong> More tracks = more complex track-to-detection association"
        ],
        examples: [
            { value: "5", desc: "Light tracking, low overhead (recommended for most applications)" },
            { value: "16", desc: "Moderate tracking, balanced performance" },
            { value: "32", desc: "Heavy tracking, significant CPU usage" },
            { value: "256", desc: "Maximum capacity, very high CPU/memory usage" }
        ],
        tip: "Start with 5-10 tracks for most applications. Increase only if you need to track more simultaneous targets. Monitor CPU usage in Debug tab.",
        warning: "Values >32 may cause performance degradation on slower systems. Each track adds processing overhead and memory usage."
    },

    trackTimeoutMs: {
        title: "Track Timeout Duration",
        definition: "Time in milliseconds before a track without new detections is marked as Lost and removed from the system. Controls track persistence.",
        details: [
            "<strong>Persistence:</strong> Longer timeout = tracks survive longer gaps in detection",
            "<strong>Responsiveness:</strong> Shorter timeout = faster removal of disappeared targets",
            "<strong>Memory:</strong> Longer timeout may keep more Lost tracks in memory temporarily",
            "<strong>Track States:</strong> Confirmed → Lost (after timeout) → Removed"
        ],
        examples: [
            { value: "1000", desc: "1 second - Very responsive, removes tracks quickly" },
            { value: "3000", desc: "3 seconds - Balanced (recommended)" },
            { value: "10000", desc: "10 seconds - Very persistent, survives long gaps" },
            { value: "30000", desc: "30 seconds - Maximum persistence" }
        ],
        tip: "Use 3000-5000 ms for most applications. Increase for intermittent signals or when targets may be temporarily obscured. Decrease for rapidly changing scenarios.",
        warning: "Very short timeouts (<1000 ms) may cause tracks to flicker on/off. Very long timeouts (>30000 ms) may keep stale tracks visible."
    },

    snrThreshold: {
        title: "SNR Threshold for Track Confirmation",
        definition: "Minimum signal-to-noise ratio in dB required for a detection to be promoted from Tentative to Confirmed track state. Controls track quality.",
        details: [
            "<strong>Sensitivity:</strong> Lower threshold = more sensitive, detects weaker signals",
            "<strong>False Positives:</strong> Lower threshold = more noise-induced false tracks",
            "<strong>Track Quality:</strong> Higher threshold = only strong, reliable signals become tracks",
            "<strong>Confirmation:</strong> Detections below threshold remain Tentative and timeout faster"
        ],
        examples: [
            { value: "3", desc: "Very sensitive, may have false positives from noise" },
            { value: "6", desc: "Balanced sensitivity (recommended)" },
            { value: "10", desc: "Conservative, only strong signals" },
            { value: "15", desc: "Very conservative, only very strong signals" }
        ],
        tip: "Start with 6-8 dB for most applications. Lower for weak/distant signals, raise to reduce false tracks from noise. Monitor track quality in Tracks panel.",
        warning: "Very low thresholds (<3 dB) will create many false tracks from noise. Very high thresholds (>15 dB) may miss legitimate weak signals."
    },

    phaseStepDeg: {
        title: "Monopulse Phase Step",
        definition: "Phase increment in degrees used for monopulse tracking refinement. Smaller steps provide more precise angle estimation but slower convergence.",
        details: [
            "<strong>Precision:</strong> Smaller steps = finer angle resolution",
            "<strong>Convergence:</strong> Larger steps = faster initial acquisition",
            "<strong>Stability:</strong> Too large may cause oscillation around true angle",
            "<strong>Processing:</strong> Step size affects number of iterations needed"
        ],
        examples: [
            { value: "0.1", desc: "Very fine precision, slow convergence" },
            { value: "0.5", desc: "Fine precision (recommended for accurate tracking)" },
            { value: "1.0", desc: "Balanced precision and speed" },
            { value: "2.0", desc: "Fast convergence, coarser precision" }
        ],
        tip: "Use 0.5-1.0 degrees for most applications. Smaller for high-precision requirements, larger for faster acquisition of rapidly moving targets.",
        warning: "Very small steps (<0.1°) may cause slow convergence. Very large steps (>5°) may cause oscillation and reduced accuracy."
    },

    scanStepDeg: {
        title: "Coarse Scan Angular Step",
        definition: "Angular step size in degrees for the initial coarse scan sweep. Determines how finely the system searches for targets across the ±90° field of view.",
        details: [
            "<strong>Scan Speed:</strong> Larger steps = faster scan, fewer computations",
            "<strong>Accuracy:</strong> Smaller steps = more accurate initial angle estimate",
            "<strong>Detection:</strong> Too large may miss targets between scan points",
            "<strong>CPU Load:</strong> Smaller steps = more FFT computations per scan"
        ],
        examples: [
            { value: "0.5", desc: "Very fine scan, slow but accurate" },
            { value: "1.0", desc: "Fine scan (recommended for precision)" },
            { value: "2.0", desc: "Standard scan, balanced speed and accuracy" },
            { value: "5.0", desc: "Coarse scan, fast but may miss targets" }
        ],
        tip: "Use 1-2 degrees for most applications. Smaller for weak signals or high precision, larger for faster acquisition. Scan step should be ≥ phase step.",
        warning: "Very large steps (>10°) may miss targets. Very small steps (<0.5°) significantly increase scan time and CPU usage."
    },

    phaseCalDeg: {
        title: "Phase Calibration Offset",
        definition: "Phase offset in degrees applied to compensate for hardware phase imbalance between the two receiver channels. Corrects systematic bearing errors.",
        details: [
            "<strong>Hardware Imbalance:</strong> Real hardware has slight phase differences between channels",
            "<strong>Calibration:</strong> Measure error with known target angle, adjust this value to correct",
            "<strong>Systematic Error:</strong> Non-zero calibration indicates hardware phase mismatch",
            "<strong>Temperature:</strong> Phase calibration may drift with temperature changes"
        ],
        examples: [
            { value: "0", desc: "No calibration (ideal hardware or MockSDR)" },
            { value: "2.5", desc: "Small correction for minor hardware imbalance" },
            { value: "-5.0", desc: "Negative correction for opposite phase error" }
        ],
        tip: "Start with 0. If bearings show consistent offset (e.g., always 5° too high), adjust this value to compensate. Re-calibrate periodically or after hardware changes.",
        warning: "Incorrect calibration will cause systematic bearing errors. Verify calibration with known target angles before field use."
    },

    phaseDeltaDeg: {
        title: "Initial Phase Delta Estimate",
        definition: "Initial phase difference estimate in degrees between the two receiver channels. Used as starting point for tracking algorithm. For MockSDR, this sets the simulated target angle.",
        details: [
            "<strong>Initialization:</strong> Provides initial guess for tracking algorithm",
            "<strong>MockSDR:</strong> In mock mode, this sets the virtual target's angle",
            "<strong>Convergence:</strong> Good initial estimate speeds up acquisition",
            "<strong>Hardware:</strong> For real SDR, typically start with 0 or last known value"
        ],
        examples: [
            { value: "0", desc: "Boresight (straight ahead), neutral starting point" },
            { value: "30", desc: "30° off boresight, if target location is approximately known" },
            { value: "-45", desc: "-45° off boresight, left side" }
        ],
        tip: "Use 0 for unknown target location. For MockSDR testing, set to desired simulated angle. For hardware, use last known angle if available.",
        warning: "For MockSDR, this value is overridden by the Live Control slider if you adjust it. For hardware, incorrect initial estimate only affects acquisition speed, not final accuracy."
    },

    sdrBackend: {
        title: "SDR Backend Selection",
        definition: "Selects which SDR hardware or simulator to use. Mock provides simulated signals for testing without hardware. Pluto supports ADALM-Pluto and AD9361-based SDRs.",
        details: [
            "<strong>Mock:</strong> Software simulation, no hardware required, perfect for development/testing",
            "<strong>Pluto:</strong> ADALM-Pluto SDR, 70 MHz - 6 GHz, full-duplex capable",
            "<strong>Hardware Requirements:</strong> Pluto requires physical SDR connected via USB or network",
            "<strong>Capabilities:</strong> Mock has no RF limitations, Pluto has real-world constraints"
        ],
        examples: [
            { value: "mock", desc: "Simulated SDR, perfect signals, no hardware needed" },
            { value: "pluto", desc: "ADALM-Pluto hardware, real RF signals" }
        ],
        tip: "Use Mock for development, testing, and demonstrations. Use Pluto for actual direction finding with real RF signals. Mock is perfect for learning the system.",
        warning: "Pluto backend requires physical hardware and proper USB drivers. Mock backend ignores TX gain and some other hardware-specific parameters."
    },

    sdrUri: {
        title: "SDR Backend Connection URI",
        definition: "Connection string for hardware SDR. Specifies how to connect to the Pluto SDR - via USB or network. Ignored when using Mock backend.",
        details: [
            "<strong>USB Connection:</strong> Use 'usb:' for direct USB connection",
            "<strong>Network Connection:</strong> Use 'ip:192.168.2.1' for network-connected Pluto",
            "<strong>Auto-discovery:</strong> Some backends support auto-discovery (leave blank)",
            "<strong>Mock Mode:</strong> This field is ignored when backend is Mock"
        ],
        examples: [
            { value: "usb:", desc: "Direct USB connection (most common)" },
            { value: "ip:192.168.2.1", desc: "Network Pluto at default IP" },
            { value: "ip:pluto.local", desc: "Network Pluto via mDNS hostname" }
        ],
        tip: "Use 'usb:' for USB-connected Pluto. Use 'ip:192.168.2.1' for network Pluto (default IP). Verify Pluto is accessible before starting tracking.",
        warning: "Incorrect URI will cause connection failure. Ensure Pluto drivers are installed and device is powered on before connecting."
    },

    mockPhaseDelta: {
        title: "MockSDR Simulated Phase Delta",
        definition: "For MockSDR backend only: Sets the simulated phase difference between channels in degrees, which determines the virtual target's angle. Can be adjusted in real-time via Live Control slider.",
        details: [
            "<strong>Simulation:</strong> Creates a virtual target at the specified angle",
            "<strong>Testing:</strong> Perfect for testing tracking algorithms without hardware",
            "<strong>Live Control:</strong> Can be changed in real-time using the MockSDR Live Control slider on this page",
            "<strong>Hardware:</strong> Ignored when using Pluto or other hardware backends"
        ],
        examples: [
            { value: "0", desc: "Target at boresight (straight ahead)" },
            { value: "30", desc: "Target 30° to the right" },
            { value: "-45", desc: "Target 45° to the left" }
        ],
        tip: "Set initial angle here, then use the MockSDR Live Control slider below for real-time adjustment. Perfect for testing multi-target tracking by changing angle dynamically.",
        warning: "Only affects MockSDR backend. For hardware SDR, this parameter is ignored. Use Live Control slider for interactive testing."
    }
};

// Function to add tooltips to all fields
function addTooltipsToFields() {
    Object.keys(tooltipData).forEach(fieldId => {
        const field = document.getElementById(fieldId);
        if (!field) return;

        const label = field.closest('label');
        if (!label) return;

        const data = tooltipData[fieldId];
        const span = label.querySelector('span');
        if (!span) return;

        // Create tooltip HTML
        let tooltipHTML = `
      <span class="info-icon">ℹ️
        <div class="tooltip">
          <h4>${data.title}</h4>
          <p><strong>Definition:</strong> ${data.definition}</p>
    `;

        if (data.details && data.details.length > 0) {
            tooltipHTML += `<p><strong>Technical Details:</strong></p><ul>`;
            data.details.forEach(detail => {
                tooltipHTML += `<li>${detail}</li>`;
            });
            tooltipHTML += `</ul>`;
        }

        if (data.examples && data.examples.length > 0) {
            tooltipHTML += `<div class="example-section"><p><strong>Examples:</strong></p><ul>`;
            data.examples.forEach(ex => {
                tooltipHTML += `<li><code>${ex.value}</code> - ${ex.desc}</li>`;
            });
            tooltipHTML += `</ul></div>`;
        }

        if (data.tip) {
            tooltipHTML += `<div class="tip-section"><p><strong>💡 Recommendation:</strong> ${data.tip}</p></div>`;
        }

        if (data.warning) {
            tooltipHTML += `<div class="warning-section"><p><strong>⚠️ Warning:</strong> ${data.warning}</p></div>`;
        }

        tooltipHTML += `</div></span>`;

        // Wrap span in field-label-row div and add tooltip
        const wrapper = document.createElement('div');
        wrapper.className = 'field-label-row';
        span.parentNode.insertBefore(wrapper, span);
        wrapper.appendChild(span);
        wrapper.innerHTML += tooltipHTML;
    });
}

// Run when DOM is loaded
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', addTooltipsToFields);
} else {
    addTooltipsToFields();
}
