# HTTP drain and borrowed-store shutdown ordering

Both cancellation and unexpected HTTP Serve termination enter the same shutdown sequence. Serve returning only proves
the accept loop stopped; already accepted handlers can still borrow history and container state. Drain HTTP for at most
10 seconds before stopping the log collector (5 seconds) and host collector (15 seconds).

Only successful HTTP drain AND joined collectors permit history/container cleanup. If HTTP drain times out or fails,
close network connections, still attempt both bounded collector stops, but withhold shared-resource cleanup until process
exit. Server.Close is not a handler join. Do not signal managed lifecycle completion on this failed-shutdown path.

Regression tests use an actual loopback listener with an induced Accept failure and a synthetic held handler; storage
cleanup cannot run while it is held. A drain-timeout case proves collectors stop but cleanup stays withheld even after
connection closure. Ordinary cancellation still drains and cleans up. No native collection or host logs are test inputs.
