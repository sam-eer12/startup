# M0 closure tracker

23 October 2026. **M0 is open; W0/pilot dates have not started.** Week 3 work
continues only as an independent synthetic technical spike authorized by the user.
No signature, contract, performance or commercial qualification is inferred from it.

| Item | Current evidence | Required closure evidence | Responsible role / status |
|---|---|---|---|
| Reference runtime performance | [Week 2 measurements](week-2.md#diagnostic-measurements), 2-vCPU/~3-GiB Podman VM | Declared 4-vCPU/8-GiB Linux host; sustained 500 req/s; payload/concurrency distribution; cold startup and executor replacement under unchanged 25 ms deadline; failures, saturation, CPU/RSS and p50/p95/p99 | Runtime/platform; named owner and host access pending |
| W0 partner commitment and routes | All four route/contract packs are synthetic | ≥2 signed pilot LOIs with shadow access; selected real routes and provider releases; named consumer invariant owners; locality/retention/trust/deployment decisions. W0 also requires ≥10 interviews across ≥3 prospects | Founder/product; no signed evidence supplied |
| Provider access | Stripe/Shopify signature fixtures; NetSuite fixture-only | Approved test accounts and scoped access; NetSuite sandbox authorization plus exact read-only operation; live qualification evidence kept separate from fixture results | Backend/integration with partner contacts; access pending |
| Effort and schedule | [Preliminary 12-day allocation](week-1-capacity.md) | Each actual owner confirms task estimates, dependencies and available capacity; technical lead records revised scope/dates within 12 engineer-days/week | Technical lead plus individual owners; confirmations pending |
| eBPF partner prerequisites | [ADR 0002 checklist](decisions/0002-runtime-trust-spike.md) | Each partner's architecture, kernel/BTF, capabilities, TLS library/build, Go linking/runtime/stripping, HTTP/2/compression and restart details | Runtime/platform plus partner infrastructure owner; collect after onboarding. Future capture qualification, not a prerequisite to this local ext_proc/registry spike |
| Proxy-Wasm comparison | Explicit descope item 2 | No M0 closure action required; ext_proc remains primary | Deferred; nonblocking |

Role names identify responsibility, not staffed assignments or owner acceptance.
Credentials belong in an approved secret store; the tracker should contain access
references and scope, never secret values. Nothing has been sent to partners.

## Reference-host qualification handoff

1. Record Linux distribution/kernel, architecture, allocated **4 vCPU / 8 GiB**,
   container limits, toolchains, image digests, repository commit plus dirty diff,
   and background services. Keep the load generator off the measured CPU budget
   or account for its resource use explicitly.
2. Run `make check build`, `make week2-up week2-check` and retain the output.
   The existing `scripts/week2_bench.py` is a short diagnostic sweep; it cannot
   certify sustained capacity.
3. Agree the sustained duration, representative payload mix and allowed failure
   rate before measuring. Drive an open-loop **500 requests/s** target, recording
   offered/completed rates, missed schedules, queueing, timeout/rejection rates,
   protected-field correctness and pass-through versus transformed latency.
   Include the 32-KiB normal mix and separately report the 256-KiB boundary.
4. Record startup and executor replacement probes from the first request, readiness
   transitions and connection establishment. Include failures in the report.
   Validate the unchanged 25 ms ext_proc deadline; the earlier cold 504 remains
   unresolved until explained and retested.
5. Record CPU, RSS/high-water and saturation with p50/p95/p99. Apply the PRD's
   added-latency budget (**p95 ≤5 ms, p99 ≤10 ms** for warm 32-KiB JSON at target
   load) using a methodology that can support that claim; subtracting
   unrelated percentile samples is insufficient. If limits fail, narrow the
   qualified payload/concurrency scope and have owners re-baseline it.
6. Attach raw evidence, methodology, scope/known limits and a named reviewer decision.
   Host qualification alone does not close the partner or owner-sizing gates.

There is no reference host available in the supplied context, and no sustained-load
or cold-start qualification is claimed by this change.
