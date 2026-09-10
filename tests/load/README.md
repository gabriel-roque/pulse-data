# k6 Load Profiles

All profiles require `PULSE_API_URL` and `PULSE_API_KEY`. They perform a real
HTTP POST and require HTTP 202 plus `X-Pulse-Durability`; missing dependencies
or missing credentials fail before load starts.

```sh
PULSE_API_URL=http://127.0.0.1:8080 PULSE_API_KEY=... scripts/run-k6.sh smoke
PULSE_API_URL=http://127.0.0.1:8080 PULSE_API_KEY=... scripts/run-k6.sh baseline
```

Profiles are fixed and intentionally honest: smoke is 10 req/s for 1 minute,
baseline is 1k/s for 5 minutes, progression reaches 1k/5k/10k/25k/50k/75k/
100k/s, spike rises abruptly to 10k/s, stress reaches 100k/s, and soak runs
for 1 hour by default. Override `SOAK_RATE`, `SOAK_DURATION`, and threshold
variables only when recording the reason with the benchmark result.

Default thresholds are error rate below 1%, p95 below 250 ms, p99 below 1 s,
and checks above 99%. A saturation profile is allowed to fail these thresholds;
that failure is the evidence used to identify maximum sustainable throughput,
not a reason to weaken the check.
