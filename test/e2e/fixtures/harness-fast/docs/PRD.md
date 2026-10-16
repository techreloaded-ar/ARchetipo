# RideAtlas Harness Lab

Small existing product used to measure delivery overhead with a real coding
agent. It is a local Node.js HTTP catalog, with no frontend, authentication,
external service, database or package dependency. Data is deliberately in memory.
It currently lists and creates motorcycle trips through GET and POST /trips.

The next product increment is support for bicycle trips. Keep the existing API
and motorcycle defaults compatible. Do not add infrastructure, a UI, a database,
payments or a design phase. Real HTTP contract tests are the acceptance surface.
Use the project's registered verification recipe; Node's own test runner is the
complete required gate. This laboratory measures harness reliability and speed,
not RideAtlas production performance or persistence guarantees.
