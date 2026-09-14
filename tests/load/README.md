# Capacity Load Test

Pulse has one load profile: `capacity.js`. It validates 100,000 events/s at
the Kafka acknowledgement boundary through `POST /v1/events/batch`.

The default workload is:

```text
200 HTTP requests/s x 500 events/request = 100,000 events/s for 5 minutes
```

Run it through the project entry point:

```sh
make capacity-test
```

The test requires:

- every response to be HTTP `202` with `X-Pulse-Durability: kafka-ack`;
- every response count to match the submitted batch size;
- zero HTTP failures and zero dropped iterations;
- p95 below 1 second and p99 below 2 seconds;
- accepted throughput of at least 99.5% of the configured target;
- requested, accepted, and Kafka topic event counts to match exactly.

The target, batch size, duration, and thresholds are intentionally fixed so a
successful `make capacity-test` always represents the same qualification.
