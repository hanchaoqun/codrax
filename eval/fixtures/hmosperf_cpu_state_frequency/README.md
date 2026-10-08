# CPU state × frequency independent oracle

Only `events.systrace` is attached. This README is not model input.
Window `[1.000,1.040)` seconds, wall duration 40ms, observed CPU set 0/1/2.
The `<idle>` row-header name does not classify CPU state. Native idle state 0 is an idle state; only the exit sentinel means active.

| CPU | Interval (seconds) | State | Frequency kHz | ms |
| --- | --- | --- | ---: | ---: |
| 0 | 1.000–1.010 | idle 0 | 1000000 | 10 |
| 0 | 1.010–1.020 | active | 1000000 | 10 |
| 0 | 1.020–1.030 | active | 2000000 | 10 |
| 0 | 1.030–1.040 | idle 1 | 2000000 | 10 |
| 1 | 1.000–1.010 | unknown | unknown | 10 |
| 1 | 1.010–1.020 | unknown | 1500000 | 10 |
| 1 | 1.020–1.035 | active | 1500000 | 15 |
| 1 | 1.035–1.040 | idle 0 | 1500000 | 5 |
| 2 | 1.000–1.040 | idle 2 | unknown | 40 |

CPU0 both-known 40ms/100%; CPU1 20ms/50%; CPU2 0ms/0%.
Frequency-known time: 40+30+0=70 CPU-ms; state-known: 40+20+40=100 CPU-ms.
All-core denominator 120 CPU-ms: joint-known 60 (50%), joint-unknown 60 (50%).
Each CPU's denominator remains 40 wall-ms; do not compare summed core time to 40ms as though it were one elapsed interval.
CPU0's first samples are carry-in, not absent. The right-boundary 3000000kHz contributes no positive duration. CPU1 has no evidenced state before 1.020. CPU2 must not borrow another CPU's frequency. No cluster topology, task execution, hardware demand, causality or power is established by this fixture.
