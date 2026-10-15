# 13 — Chain authentication securely

**What to build:** Run login followed by authenticated requests while keeping newly extracted tokens hidden from agents and stored reports.

**Blocked by:** 06 — Use credentials without exposing them; 12 — Run sequential scenarios

**Status:** complete

**Specification coverage:** User stories 25, 41, 43, 47.

- [x] TUI can approve protected JSON Pointer token extraction and its permitted later destinations/placements.
- [x] CLI may use approved extraction definitions but cannot create or alter protected bindings to redirect credentials.
- [x] Secret extracts are registered before the originating login response is rendered, logged, exported or stored.
- [x] Extracted values remain memory-local to a run, appear only as opaque references and never persist automatically into environment variables.
- [x] Later requests use the token only at permitted positions and origins; changed definitions or destination settings require fresh approval.
- [x] Failed token extraction prevents dependent sends; ordinary extraction cannot be used to read back an approved protected secret field.
- [x] Tests demonstrate login-to-request success and verify token absence from all outputs, unauthorized downstream destinations and modified extraction/binding attempts.
