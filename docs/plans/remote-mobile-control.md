# Remote control of a desktop Atlas from a phone — architecture and development plan (review draft)

[中文](../zh-CN/plans/remote-mobile-control.md)

> Status: **Implemented (phases A/B/C delivered, 2026-09-08)** · Drafted 2026-09-08. Delivery record: phase A (`atlas_ws` + `atlas server`) and phase B (Flutter mobile remote connection) were implemented and verified against this document; the phased plan in §15 and Q1/Q4/Q9 of §21 adopted their recommended defaults (atlas server as the only runtime owner, multiple connections allowed). The working directory is no longer required up front in a profile: neither connecting nor browsing history needs it, and the UI guides the user to set it and writes it back to the profile before the first message (the ACP wire requires `cwd` on `session/new`; adjusted 2026-09-09). The remote file browser and remote terminal (Q7) and P3 (desktop using the server uniformly) remain follow-up items.

---

## 1. Background and goals

Atlas is a local general-purpose agent on a computer: the single `atlas_runtime` engine, the built-in tools (read/write/edit/shell/plan), the model providers, and SQLite persistence (`~/.atlas/atlas.db`) all live on that computer. Existing interfaces are the Nocterm TUI and the Flutter desktop/mobile client. Flutter local mode starts an `AcpServer` in-process and consumes it as an `AcpClient`; "remote connection" currently only spawns a third-party ACP agent over stdio.

**Goal**: let a phone (Android/iOS) operate the Atlas running on the computer — list/create/resume/delete sessions, send prompts, watch model output and tool execution live, cancel, switch model/effort, `/compact`, answer permission requests — while model requests, shell commands, file edits, and the database stay on the computer.

### 1.1 Completion criteria

- `atlas server` on the computer starts a WebSocket ACP server bound to loopback only, reusing the single runtime composed by `atlas_composition`.
- The Atlas app on the phone composes no local runtime; the user enters an address and token, then performs every operation above through the existing workspace UI.
- After a reconnect, `session/load` restores the display from the persisted timeline; **the prompt sent before the disconnect is not replayed automatically**.
- The server binds 127.0.0.1 by default; cross-network access is expected to go through Tailscale; no public port is opened.

### 1.2 Non-goals (not in the first version)

| Not doing | Reason |
|---|---|
| Remote screen/mouse/keyboard control (remote desktop) | Different product shape |
| The phone reading/writing the computer's filesystem or running its shell directly | The agent's tools run on the computer and return results over ACP |
| The phone storing or using model API keys | Credentials stay on the computer |
| Public anonymous access / cloud relay / user accounts / multiple server nodes | No need, and it widens the attack surface |
| QR pairing in the first version | Manual entry is enough to validate; QR is a later phase |
| A remote interactive terminal panel in the first version | Separate large feature, see §11 |
| Inventing a second business REST API for remote use | Reuse ACP directly (§5) |

---

## 2. Current state and reusable assets

```text
apps/atlas_flutter       desktop/mobile ACP client (local = in-process AcpServer+AcpClient)
apps/atlas_cli           composition root: atlas→TUI; atlas acp→stdio ACP; planned atlas server
packages/atlas_composition   composeRuntime(config)→the single AgentRuntime
packages/atlas_runtime   domain models/events/ports/the single agent loop/session lock
packages/atlas_acp       AcpServer+AcpClient (acpd engine; stdio/channel/memory transports)
packages/atlas_ws        empty package (planned: versioned WebSocket wire contract)
packages/atlas_tools     built-in tools; packages/atlas_storage Drift persistence
```

Key conclusions:

1. **No second agent loop is needed**: ACP already covers the full session lifecycle, prompts, event streams, permission requests, cancellation, and compaction.
2. **`AcpServer` transports are already decoupled from the channel**: `serveChannel(StreamChannel<String>)` accepts any string channel, so a WebSocket only has to provide "one message per frame" for a zero-protocol-change integration. The `atlas_acp` increment is tiny (exposing `serveTransport`, if needed).
3. **`AcpClient` is decoupled from presentation**: `RuntimeEnvironment.runtime` is a `PresentationAgentSession`, so the remote connection only injects another `AcpClient` instance and the workspace code never sees the WebSocket.
4. **The permission model already exists**: the client's `PermissionPort` handles `session/request_permission`, which is how the phone shows a permission dialog today.
5. **`AcpConnection` (command+args) ≠ remote connection (url+token)**: a separate connection model is required (§9.2).

---

## 3. Overall architecture

```text
┌─────────────────────────────────┐
│  phone Atlas app (android/ios)  │
│  RemoteConnectionProfile        │
│  AcpClient                      │
│  Workspace UI (existing)        │
└───────────────┬─────────────────┘
                │ WebSocket text frame = one JSON-RPC message
                │ ws://<computer-tailnet-name>:8765/acp  or  wss://… (tailscale serve)
┌───────────────▼─────────────────┐
│           Tailscale             │   private encrypted mesh / optional serve HTTPS
└───────────────┬─────────────────┘
                │ 127.0.0.1:8765
┌───────────────▼─────────────────┐
│          atlas server           │   (new atlas_cli subcommand)
│  WebSocket listener             │
│  token auth / limits / audit    │
│  one AcpServer per connection   │
└───────────────┬─────────────────┘
                │ one AgentRuntime in-process (session lock)
                ▼
   atlas_provider / atlas_tools / atlas_storage (atlas.db)
```

Invariant: model requests, tool execution, files/shell, and SQLite stay on the computer; the phone is always an ACP client and never holds provider credentials or duplicates the agent loop.

---

## 4. Decision summary

| # | Decision | One-line reason |
|---|---|---|
| D1 | Protocol = ACP over WebSocket, one JSON-RPC message per frame | ACP already defines the lifecycle, events, and permissions, and the official RFD treats WS as the remote shape |
| D2 | `atlas server` is the only runtime owner | The in-process session lock stays effective and concurrent turns over one shared database are impossible |
| D3 | Network: localhost + Tailscale, no public port | The agent holds shell and file permissions, so exposing it publicly is remote code execution |
| D4 | Security = Tailscale (network) + bearer token (application), two layers | Being on the network is not authorization, and the token can be rotated independently |
| D5 | A disconnect replays nothing; the server finishes in-flight turns | Prevent duplicated side effects; results are recovered with `session/load` |
| D6 | Mobile starts no local runtime and goes straight to the remote screen | A phone has no config, credentials, or working tree, so a local runtime only produces wrong data |
| D7 | Remote mode hides the local FileBrowser/TerminalPanel | On a phone they would operate on the phone's own filesystem and shell |

---

## 5. Protocol design: ACP over WebSocket

### 5.1 Why not design a new REST API

REST+SSE would have to recreate session lifecycle, event mapping, the permission round trip, cancellation, response/request correlation, and compaction. That ends in two protocols, two clients, and double maintenance. ACP is bidirectional (the agent can send `session/request_permission` and await the reply on the same connection), which a full-duplex WebSocket matches naturally.

### 5.2 Basis

- ACP v1 transports: stdio is the standard, and **custom transports are allowed** as long as the JSON-RPC message format and ACP lifecycle are preserved.
- The ACP RFD "Streamable HTTP & WebSocket Transport" (in progress) promotes WS as the remote shape with the same JSON-RPC and lifecycle; v1 has no replay and resumability belongs to v2. This plan follows it and can move to it later.
- References: agentclientprotocol.com/protocol/v1/transports.md, agentclientprotocol.com/rfds/streamable-http-websocket-transport.md

### 5.3 Wire contract (Atlas custom transport v1, self-documented in atlas_ws)

- Endpoint: `GET /acp` (HTTP upgrade).
- Authentication: HTTP `Authorization: Bearer <token>` (the token never appears in the URL).
- Each **text frame** carries one complete JSON-RPC 2.0 text message (framing is inherent, so NDJSON is unnecessary).
- Bidirectional: requests, responses, and notifications in both directions; `session/update` is a notification.
- **Binary frames, invalid JSON, oversized frames, and unauthenticated connections make the server close the connection.**
- JSON-RPC `id` values come from the client; responses correlate by id; notifications expect no response.
- Capability negotiation, session methods, event structures, and the `_atlas.dev` extensions all reuse `atlas_acp` (identical to stdio).
- One connection runs multiple sessions concurrently while turns within a session stay serialized by the runtime lock.

```jsonc
// phone → computer (text frame)
{ "jsonrpc": "2.0", "id": 12, "method": "session/prompt",
  "params": { "sessionId": "session-…", "prompt": [
      { "type": "text", "text": "Investigate the Android build failure" } ] } }

// computer → phone (text frame)
{ "jsonrpc": "2.0", "method": "session/update",
  "params": { "sessionId": "session-…",
              "update": { "sessionUpdate": "agent_message_chunk",
                          "content": { "type": "text", "text": "Reading the logs first." } } } }
```

### 5.4 Lifecycle

```text
phone                             atlas server
 │  WebSocket upgrade (Authorization) │
 │  initialize ─────────────────────> │
 │ <─ initialize (capabilities/version)│
 │  session/list|new|load|resume      │
 │  session/prompt ─────────────────> │
 │ <─ session/update streaming events │
 │ <─ session/request_permission ──── │ (when needed, bidirectional)
 │   (permission reply) ────────────> │
 │  session/cancel ─────────────────> │
 │ <─ terminal prompt response ────── │
```

The only difference is that `AcpClient`'s transport becomes a WebSocket string channel.

---

## 6. Key decisions in detail

### 6.1 D2: a single runtime owner on the computer (most important)

`AgentRuntime`'s session lock is **in-process**. If "the phone connects to a runtime in one process" while "the computer's TUI/desktop local runtime" shares the same `atlas.db`, neither process knows the other holds the lock: concurrent turns on one session and interleaved persistence follow.

**Adopted**: the `atlas server` process becomes the only runtime owner (the only SQLite writer), and every client (the phone today, a desktop Flutter remote connection later) connects to it as an ACP client.

> If the real requirement is to control the runtime inside an already-open desktop Flutter window, do not start another `atlas server` against the same database; listen for WebSocket inside that Flutter process and reuse its existing `AgentRuntime` (wrapping one `AcpServer` per remote connection). The protocol does not change, only the host — see Q1.

### 6.2 D5: no automatic replay after a disconnect

- Server: a dropped connection does **not** cancel a running turn; notification writes become no-ops and the turn finishes and persists (completed/aborted/failed). Only an explicit `session/cancel` cancels it.
- Client: after reconnecting, `session/load` replaces local state with the server timeline. When the last prompt has no terminal response, the app says the result may already have run and asks the user to confirm instead of resending.
- Basis: the ACP remote transport RFD: v1 has no message replay or stream resumption.

### 6.3 D1: protocol details belong to atlas_ws

`packages/atlas_ws` (an empty package at the time of writing) owns the WS endpoint, the authentication handshake, frame boundaries and size, connection count, ping/pong, disconnect cleanup, and auditing — it composes no runtime and owns no ACP semantics. The dependency direction `atlas_ws → atlas_acp → atlas_runtime` matches the architecture document.

---

## 7. Disconnect and recovery semantics (contract)

Phones are on mobile networks, so disconnects are normal. The contract comes first:

```text
connection lost:
  - server: the in-flight turn finishes (D5) and its result is persisted
  - client: keep local cache → exponential backoff reconnect (1s/2s/4s/…/30s cap)
      → connected → refresh session/list → session/load the current session
      → reconcile the display with the persisted timeline
      → say "connection restored; the computer is authoritative for the last message", no auto-resend
  - user presses stop → session/cancel → the runtime cancels and keeps received text
  - server unreachable/restarted → the UI shows "the computer is not running" and suggests starting atlas server
```

- The app stops fast reconnecting in the background and tries once immediately on return to foreground; a user-initiated disconnect never reconnects automatically.
- Only live deltas during the outage are lost, never session data.
- Server shutdown: close the listener → close each connection socket → clients treat it as a disconnect.

---

## 8. Authentication and authorization

### Threat model (key premise)

AGENTS.md states it plainly: Atlas tools run with the permissions of the Atlas process, with **no sandbox, no permission prompt, and no approval gate**, and protocol adapters must not claim a security boundary the runtime does not have. Therefore a client that can reach the server is a client that can read and write local files and run shell commands. Authentication is not about limiting tools; it is about ensuring that only authorized people and devices can connect at all.

### 8.1 Recommended: Tailscale (network) + bearer token (application)

| Layer | Role | Controlled by |
|---|---|---|
| Tailscale | Network reachability, link encryption (WireGuard), ACLs | The user's tailnet ACL |
| Bearer token | Application authentication: the ACP `initialize` handshake needs it | atlas server |
| (Optional) firewall | Defense in depth so port 8765 never reaches the LAN or internet | The user |

- No account system, no OAuth, no public registration.
- Token: `Random.secure()`, 32+ bytes, hex/base64url; generated on first start; stored in `~/.atlas/remote_token` with mode 0600; printed on every start; compared in **constant time**; rejections are delayed uniformly; never placed in URLs, logs, or process arguments.
- Mobile storage: `flutter_secure_storage ^11.0.0` (Android Keystore / iOS Keychain; the current release was checked against the current SDK and Flutter). The rest of the profile (URL and name) uses ordinary preferences.

### 8.2 Rejected alternatives

| Option | Why rejected |
|---|---|
| Public port plus token | A sandbox-free agent on the public internet; a leaked token means a remote shell |
| Trusting Tailscale identity alone (no token) | Requires extra libraries to parse tailnet identity, and the token already provides per-device authorization inside the tailnet; kept as Q8 |

---

## 9. Mobile client changes

### 9.1 Startup split

- Desktop: unchanged (local `bootstrapRuntime`).
- Android/iOS: skip local bootstrap and go to the remote connection screen (empty state → enter URL/token → connecting → connected into the workspace / retryable failure that keeps the profile). The working directory is not a required profile field: before the first draft is sent, a hint above the composer says "Choose directory" → the user enters an absolute path → after validation the app writes it back to the profile (secure storage) and creates a draft rooted in that directory. Until it is set, `send()` is blocked by a guard instead of creating a remote session with the phone's own sandboxed path.

### 9.2 Connection model: a new RemoteConnectionProfile

```dart
final class RemoteConnectionProfile {
  final String name;            // "My computer"
  final Uri wsUrl;              // wss://host/acp
  final String token;           // memory/secure storage only, never plain JSON
  final String? workingDirectory; // absolute path on the computer; null = set before the first message
  final DateTime lastConnectedAt;
}
```

- It coexists with `AcpConnection` (command+args, a third-party ACP agent over stdio on desktop) without either affecting the other; the ACP connections list in the settings dialog keeps its meaning.
- Storage: plain JSON holds only name/wsUrl/lastConnectedAt; the token goes to secure storage.

### 9.3 Connection flow bootstrapRemoteConnection

```text
AcpClient over WsChannel(wsUrl, {Authorization: Bearer token})
  → client.connect() (initialize)
  → temporary session/new probe → read the model catalog → delete the probe session
    (reusing the existing bootstrapAcpClient pattern)
  → RuntimeEnvironment(runtime: client, models: catalog, …)
  → RuntimeEnvironmentController switch (existing mechanism + onClose closes the ws)
```

`AcpClient` already implements `PresentationAgentSession` and `PermissionPort`, and `WorkspaceController._subscribePermissions()` naturally receives server permission requests and shows the existing dialog. Presentation code needs no changes.

### 9.4 UI additions

- A connection screen (profile management: create/edit/delete/switch).
- A connection status bar: connected / reconnecting / offline (server unreachable).
- A remote-directory hint: when the profile has no working directory and the focus is a draft, the composer shows "Sessions run in a directory on your computer → Choose directory"; once set, that profile never asks again.
- Every session operation reuses the existing workspace (list/new/resume/prompt/cancel/rename/delete/compact/usage/diff display).
- No desktop regression: desktop keeps using the local runtime, and the ACP connections list is untouched.

---

## 10. Server: atlas server (an atlas_cli subcommand) + atlas_ws

### 10.1 CLI

```text
atlas server [--listen 127.0.0.1:8765] [--token-file PATH]   # prints the token on start
atlas server --rotate-token
```

- Default `127.0.0.1:8765` (a safe default; never 0.0.0.0 unless asked). `--listen 0.0.0.0:…` opens it explicitly and prints a warning (public exposure should go through Tailscale).
- Reuses `composeRuntime(config)`; multiple connections share one runtime and the session lock already serializes turns.
- Shutdown semantics match `atlas acp`: flush, then exit explicitly, closing every connection gracefully.

### 10.2 The atlas_ws package (the planned work, delivered)

```text
packages/atlas_ws/
  lib/atlas_ws.dart
  lib/src/ws_transport.dart   WebSocket ↔ acpd Transport (StreamChannel adapter)
  lib/src/ws_server.dart      HttpServer + upgrade → one AcpServer per connection
  lib/src/token_auth.dart     token generation/constant-time check/rotation/0600 file
  test/…
```

Dependencies: `atlas_acp`, `dart:io`, `crypto` (constant-time comparison; confirmed present in the lockfile). No third-party WebSocket library was added (AGENTS.md: add a dedicated dependency only when the implementation needs it; evaluate `dart:io` first).

### 10.3 atlas_acp increment (very small)

`AcpServer` is already transport-abstracted (stdio/channel/memory). WebSocket is a new transport shape, so it is enough to expose `serveTransport(Transport)` (currently private) and **not duplicate protocol logic**.

### 10.4 Connection lifecycle and concurrency

- Each client is a separate WS + a separate `AcpServer` instance + one `initialize`.
- The server does not arbitrate session ownership: the runtime session lock covers concurrent access to one session from several clients, and "one client per session" stays a UI recommendation.
- `atlas acp` and `atlas server` run one at a time per process; a user can still run two processes over one database, the same risk as running `atlas` and `atlas acp` today (no cross-process lock). **This plan does not solve that** (Q6; the boundary is documented).

---

## 11. Files and terminals (presentation boundary in remote mode)

`FileBrowser` and `TerminalPanel` use local `dart:io` and `pty2`. In phone remote mode they would operate on the phone's own filesystem and shell, which is the wrong semantics, so:

- Remote mode **does not render** the local file browser or the local interactive terminal.
- Remote mode **keeps**: the session list, conversation, model/effort selectors, usage, tool calls and results, file-edit diffs (ACP `tool_call_update` already carries diff content blocks), the permission dialog, compaction, and session metadata.
- Shell tool output is presented as text (`rawOutput`). A phone↔computer streaming pseudo-terminal and a remote file browser are **not** built (a separate large feature, Q7, later phase; if built, the server restricts it to `workingDirectory` + `additionalDirectories` through ACP fs/terminal extensions).

---

## 12. Test plan

### 12.1 atlas_ws (Dart unit and integration tests)

- Authentication: no token rejected, wrong token rejected, old token invalid after rotation, token file is 0600.
- Frames: valid JSON-RPC round trips; invalid JSON, binary frames, and oversized frames close the connection.
- Transport: request/response id correlation; server-initiated requests (permissions) reach the client; `session/update` notifications stream to the client; concurrent connections; parallel sessions on one connection.
- Lifecycle: a client disconnecting mid-turn lets the server finish the in-flight turn (slow fake provider) and a reconnect sees the terminal state through `session/load`; server shutdown closes clients.
- Several connections over one runtime: the session-lock semantics do not regress.

### 12.2 atlas_acp increment

- Once `serveTransport` is public, the same handler tests run over stdio/channel/memory/ws (add a ws variant to the existing tests; no new protocol behavior).

### 12.3 atlas_cli server

- Start/stop/help; `--rotate-token`; an assertion that the default bind is 127.0.0.1.

### 12.4 Flutter

- `remote_bootstrap`: start a fake ws server locally → connect with a profile → catalog → session list → prompt → permission dialog → disconnect/reconnect state machine.
- Secure storage: `FlutterSecureStorage.setMockInitialValues` proves the token never reaches ordinary storage or logs.
- Platform split: Android/iOS targets start no local runtime.
- Full desktop local-mode regression (the existing 135 cases must stay green).

### 12.5 End-to-end manual acceptance (performed by the user)

```text
computer: tailscale up; atlas server --listen 127.0.0.1:8765
phone: on the same tailnet; profile ws://<tailnet-name>:8765/acp
flow: create a session → prompt → streaming reply → have the agent read/modify a file (see the diff) →
      turn wifi off for 5s → reconnect → session/load shows the finished turn (no duplicate execution) →
      cancel works → rotate the token → the old connection drops and the new token connects
```

---

## 13. Dependency list (all additions)

| Purpose | Package | Version (latest verified) | Notes |
|---|---|---|---|
| Mobile secure storage | flutter_secure_storage | 11.0.0 | Added with `flutter pub add` during implementation (per repository rules) |
| Constant-time comparison | crypto | Already in the lockfile (transitive) | Used directly |
| WebSocket | dart:io (no third party) | — | Evaluate first; add a dedicated package only if it is not enough |
| Icons/UI | lucide/material_ui, already present | — | No additions |

Tailscale is a deployment dependency, not a code dependency: it belongs in the docs and the acceptance steps, not in `pubspec.yaml`.

---

## 14. Security checklist

Implementation status (reviewed after the phase A/B delivery, 2026-09-08):
- [x] Loopback-only by default; `--listen` prints an explicit warning when exposed
- [x] Authorization is checked before the upgrade; tokens are ≥256 bit; the file is 0600
- [x] Tokens never appear in queries, logs, or process arguments; after rotation the old token is rejected for new connections immediately (established connections stay, which is intentional); comparison is constant time
- [x] Frame limit (8 MiB), connection limit (4), 30s ping
- [ ] **Not implemented (left for follow-up)**: 90s idle disconnect and unauthenticated rate limiting; the connection limit and ping bound a malicious slot today (a rejected connection is released immediately)
- [x] Non-`/acp` paths return 404; binary and oversized frames are rejected and their connection slot is released immediately
- [x] Server shutdown closes every connection; connect/disconnect events are logged (stdout/stderr callbacks without tokens or provider keys); persisted audit files remain follow-up work
- [x] The phone only sees model descriptors, usage, tool updates, and error summaries; API keys and upstream request/response headers never leave the computer

---

## 15. Phased delivery plan (each phase independently mergeable and revertible)

### Phase A: atlas_ws + atlas server (computer side only)

- Content: the new `atlas_ws` package (transport/auth/server), the `atlas_cli` `server` subcommand, and exposing `serveTransport` in `atlas_acp` if needed.
- After delivery: the computer can run `atlas server`, and a Dart test client/script verifies the whole protocol (initialize → session/new → prompt → cancel → load). The phone has no UI yet, but the protocol and its reliability are settled.
- Verification: `dart test packages/atlas_ws`, `cli-build`, and a local ws smoke test.
- Independently shippable: any ACP client can connect (a future desktop remote connection will rely on it too).

### Phase B: Flutter mobile remote connection (depends on the wire contract from A)

- Content: platform split, `RemoteConnectionProfile`, `remote_bootstrap`, the connection screen and status bar, and hiding the local file/terminal panels in remote mode.
- After delivery: the phone performs every session operation against the desktop Atlas plus disconnect/reconnect.
- Verification: `mise run ci` fully green; manual acceptance on a real device or simulator plus the computer; `mise run app-build-android` and `app-build-ios`.
- Note: the B UI shell (connection screen, status bar) can be developed in parallel with A, but integration depends on A's wire being final or it will be reworked.

### Phase C: desktop/mobile consistency polish and documentation (optional wrap-up)

- Add a "remote connection" entry to desktop settings (reusing B).
- Documentation: README, docs/architecture.md, docs/protocol-acp.md, docs/configuration.md plus their zh-CN counterparts; move `atlas_ws` and `atlas server` from Planned to Available; archive this review draft.
- Security review follow-up.

---

## 16. Rollback strategy

- A/B/C ship as separate commits. Reverting A means reverting `atlas_ws` and the `server` subcommand without touching the existing acp/stdio/local paths; reverting B means reverting the mobile split (desktop is unaffected; phones had no working version before B, so the rollback only returns to the previous state with no data risk).
- Data: zero schema changes and zero migrations; deleting the token file just disconnects, and it can be regenerated.
- External state: no cloud services, no public API, no subscriptions.

---

## 17. Rejected approaches

| Approach | Why rejected |
|---|---|
| A. The phone calls provider APIs directly | Keys on the phone, a second agent loop, tools that cannot run safely, split sessions |
| B. REST + SSE business API | A bidirectional protocol would have to be rebuilt; two clients; double maintenance on ACP upgrades |
| C. Public port forwarding | A sandbox-free agent exposed to the internet |
| D. Trusting the network as authentication (Tailscale only) | Network membership is not Atlas authorization and cannot be revoked separately |

---

## 18. Risks and mitigations

| Risk | Description | Mitigation |
|---|---|---|
| The lack of a sandbox is mistaken for a security boundary | ACP and the token only control who can connect, not what a connected client can do | Say so in the docs; bind 127.0.0.1 by default; never claim a permission gate |
| Divergent disconnect semantics | A local turn runs to completion, so cancelling on disconnect would lose results | Contract: the server finishes in-flight turns and the client reconciles with `load` (§7) |
| No formal ACP standard for WS | A custom wire has to align with ACP v2 later | Minimal wire (JSON-RPC over text frames); `atlas_ws` documents its own version |
| Android/iOS killing the connection in the background | Reconnect storms and confusing state | Reconnect on foreground plus a `load` reconcile; backoff capped at 30s |
| Desktop local-mode regression | The startup split affects desktop | Branch by platform, leave the desktop path untouched, and run the full existing suite |
| Token leakage in plaintext | Screenshots and logs | Print once, 0600, never in a URL, secure storage |
| **The most fragile assumption** | `atlas server` carries the authoritative runtime on the computer | If the real requirement is to control the runtime of an open desktop window, move the host to an in-process Flutter listener; the protocol, authentication, and mobile design are unchanged (Q1) |

---

## 19. Architecture decision record (ADR summary)

| Decision | Outcome | Rejected alternative |
|---|---|---|
| D1 protocol | ACP over WebSocket (`atlas_ws`) | REST+SSE |
| D2 server host | The `atlas server` subcommand + `composeRuntime` | A server embedded in the desktop app (decided after Q1; not ruled out for a later phase) |
| D3 authentication | Tailscale + bearer token, two layers | Exposed to the internet; tailnet identity alone (Q8 deferred) |
| D4 disconnects | The server never cancels in-flight work; the client reconciles with `load` and never resends | Cancel as soon as the socket drops |
| D5 mobile | No local runtime, a pure remote client | A local runtime on the phone |
| D6 files/terminals | Remote mode renders no local FileBrowser/Terminal; diffs are text | Remote filesystem and remote pty (later phase) |

---

## 20. Glossary

| Term | Meaning |
|---|---|
| ACP | Agent Client Protocol (the JSON-RPC protocol between an agent and a client) |
| AcpServer / AcpClient | The existing `atlas_acp` implementations (agent side / client side) |
| RuntimeEnvironment | The runtime facade Flutter injects into presentation code (existing) |
| RemoteConnectionProfile | New: the connection model of name + wsUrl + token |
| Tailscale Serve | Tailscale's official proxy that exposes a local port to the tailnet over HTTPS |

---

## 21. Open questions (to settle during review)

| # | Question | Options | My recommendation |
|---|---|---|---|
| Q1 | Is the "desktop Atlas" being controlled (a) a new `atlas server` process (what this plan assumes) or (b) the runtime already running inside a desktop Flutter window? | a/b | (a); choosing (b) turns phase A into an in-process Flutter WS listener with the protocol unchanged |
| Q2 | How should the computer side stay resident? | Manual / systemd / embedded in the desktop app | Manual plus documentation; embedded startup is a separate feature |
| Q3 | Is a connected-device list with forced disconnect needed? | Yes/no | Yes (`atlas server sessions` plus disconnect) for security visibility |
| Q4 | May several phones connect to one server at the same time? | Allow multiple connections | Allow them (the runtime lock already serializes); one client per session is recommended |
| Q5 | Mobile token entry UX | Manual paste | Manual paste for the first version; QR later |
| Q6 | Concurrency over one `atlas.db` from several processes (server and desktop local mode together) | Ignore/handle | Ignore and document the known boundary (the same risk exists today) |
| Q7 | Remote file browser / remote terminal | Not doing it / later phase | A separate later feature through ACP fs/terminal extensions |
| Q8 | Tailscale identity without a token | Do it / skip | Skip (the token is enough for the first version) |
| Q9 | Working directory for remote sessions | The computer's home directory by default / the phone picks a project first | Home directory by default; project selection later (a server-side cwd allowlist) |

**Q1 decides the phase A host; Q2/Q3 shape the server command surface; the rest can take the defaults.**

---

## 22. Appendix: command reference (after implementation)

```sh
# computer
tailscale up
atlas server --listen 127.0.0.1:8765

# phone: Atlas app → new remote connection
# ws://<computer-tailnet-name>:8765/acp (Tailscale MagicDNS) or wss://… through serve

# verification (development-time dart script/test)
# initialize → session/new → session/prompt → session/update stream → session/cancel
```
