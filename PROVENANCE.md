# Provenance

Everything in this repository was written for it, on personal time, by Tran Trung Dung.

- A load laboratory for a cache, and the harness that turns a load assumption into a number. No employer code, no client code, no proprietary data. Nothing here is
  extracted from a product I worked on; it talks to a Valkey or Redis it starts itself; the keys and the workload are generated in this repository.
- Third-party code is not vendored: the dependencies are github.com/redis/go-redis/v9 and, for the in-process tests, github.com/alicebob/miniredis/v2, and each keeps its own licence.
- The diagrams are drawn here. The commit history is the history of this repository.
- The measured numbers quoted in the README come from the harness in this repository, and `make measure` is the command that produced them.
- `scripts/chart-check.sh` renders the chart in CI and checks that the manifest still makes the claims the README does; it is part of the evidence, not decoration.

If you believe something here belongs to you, open an issue and it will be dealt with.
