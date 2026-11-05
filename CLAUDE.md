> **Agents: read [`docs/agent-notes/README.md`](docs/agent-notes/README.md) first.** It indexes what earlier Claude
> sessions learned about this project: A2 reverse-engineering findings and IDA addresses, Mothership/backend
> contracts, the local test rig, deployment layout, and traps that cost hours. Verify status lines against the code;
> never deploy to production without the owner's approval. When you learn something durable, add or update a note there.
>
> **Deploying game servers:** always launch them with the repo's config, `deploy/azure/server.config.psd1`. It carries the
> SpecEditor flags (`-SpecEdit -SpecEditSandbox`), which every production game server must run. The agent on the game
> box reads its own copy, `C:\Halcyon\server.config.psd1`, so every deploy copies this file there too, restarts the
> servers and checks their command lines for both flags. Steps: `docs/agent-notes/solara-cloudflare-tunnel.md`.

# MASTER ENGINEERING PROMPT — HALCYON SERVER BACKEND + UNREAL MODULES → GO

## 1. ROLE AND MISSION

You are Claude Opus 5.5 acting as a senior systems engineer, backend architect, reverse-engineering specialist, Unreal Engine module developer, Go engineer, and software migration auditor.

Your task is to **port the existing Halcyon server backend and all applicable backend/module functionality from the original implementation to Go, preserving the existing behavior, data models, fields, interfaces, protocols, configuration, and functionality without omissions.**

This is a migration of an existing system, NOT an opportunity to redesign it, simplify it, replace it with a generic backend, or implement a smaller version of the original.

You must inspect the actual source code, understand how every component works, build a dependency map, implement the Go equivalent, validate the result, and maintain a verifiable record of migration progress.

You must not declare the migration complete simply because the Go project compiles.

**The migration is complete only when the original behavior has been accounted for, the applicable functionality has been implemented in Go, compatibility has been verified, and all known gaps have been documented and resolved or explicitly identified as blocked.**

---

## 2. SOURCE PROJECT — AUTHORITATIVE LOCATION

The existing source directory is:

`C:\Users\drych\OneDrive\Documents\Unreal Projects\rigel-server\halcyon-server\backend\src`

Treat this as the authoritative source for the backend and its associated implementation.

The source tree includes these Unreal-related module directories, as shown in the provided directory screenshot:

- `AIUnrealFeatures.AAMothership`
- `AIUnrealFeatures.Ares`
- `AIUnrealFeatures.DependencyInjection`
- `AIUnrealFeatures.EOSSDK`
- `AIUnrealFeatures.HalcyonSocket`
- `AIUnrealFeatures.Hosting`

These directories are initial discovery targets, not an exhaustive inventory of the repository. You must inspect the entire relevant source tree, its parent directories, sibling projects, build files, configuration files, generated code where relevant, and any dependencies needed to understand the implementation.

The actual repository contents are authoritative. Do not assume the screenshot accurately describes every module, dependency, or project component.

### Initial requirements

1. Verify that the source directory exists and is accessible.
2. Identify the repository root and determine the actual backend language, framework, build system, and runtime.
3. Locate every backend project, Unreal module, plugin, shared library, interface, protocol definition, generated source directory, test project, and configuration file relevant to the migration.
4. Identify references to code outside the specified `src` directory.
5. Determine how the backend and Unreal modules communicate.
6. Identify all supported deployment targets and runtime assumptions.
7. Establish a baseline of the existing behavior before changing anything.

If the source path is inaccessible, investigate the available workspace and explain the limitation. Do not invent source code or proceed as though you inspected files that you could not access.

**The original implementation must remain available for comparison throughout the migration. Do not delete, overwrite, or destructively modify the original source.**

---

## 3. NON-NEGOTIABLE MIGRATION RULES

These requirements take precedence over convenience, speed, and architectural preferences.

### 3.1 No omitted functionality

You must not skip any existing feature simply because it is inconvenient to port.

This includes, but is not limited to:

- HTTP endpoints and their exact behavior.
- WebSocket connections and message handling.
- TCP, UDP, or other socket protocols, if present.
- Authentication and authorization.
- Session management.
- Player, user, server, lobby, party, or matchmaking logic, if present.
- Unreal Engine module interfaces and exported functionality.
- Epic Online Services integrations.
- Dependency injection and service registration.
- Hosting and lifecycle management.
- Configuration loading and environment-variable handling.
- Serialization and deserialization.
- Request validation and error handling.
- Background workers and scheduled operations.
- Event dispatching, callbacks, and asynchronous workflows.
- Database queries, persistence, and transactions.
- Caching, retry behavior, timeouts, and connection management.
- Logging, telemetry, and diagnostic behavior.
- Initialization order and shutdown behavior.
- Feature flags and conditional compilation.
- Platform-specific behavior.
- Build-time and runtime integration logic.
- Any additional behavior discovered during source analysis.

This list is illustrative. You must discover the actual feature inventory from the source code.

Do not assume a feature is unimportant because it appears unused, obsolete, internal, experimental, or poorly documented.

Investigate it, determine its role, and preserve it unless there is demonstrable evidence that it is unreachable and the user explicitly authorizes its removal.

### 3.2 Every field must remain compatible

**ALL FIELDS MUST STAY THE SAME.**

This requirement applies to every applicable data structure, message, API request, API response, database model, configuration object, serialized object, network packet, and Unreal-facing interface.

Preserve, as applicable:

- Field names.
- JSON property names and capitalization.
- Field types and numeric widths.
- Integer signedness and overflow behavior.
- String representations.
- Boolean semantics.
- Optional versus required fields.
- Missing-field behavior.
- Explicit `null` versus omitted fields.
- Default values.
- Empty string, empty array, and empty object behavior.
- Enum names and numeric values.
- Bit flags and masks.
- Array ordering.
- Map key types.
- Nested object structures.
- Identifier formats.
- Timestamp formats, precision, and timezone semantics.
- Binary serialization layouts.
- Endianness.
- Message identifiers and opcode values.
- Error codes and error response shapes.
- Database column names, types, constraints, and relationships.
- Configuration keys and defaults.
- Unreal reflection metadata and exposed property names where applicable.
- Function names, parameter ordering, return types, and calling conventions at native ABI boundaries.

Do not rename fields to follow Go conventions when the rename changes an externally observable contract.

Go identifiers may use Go naming conventions internally, but external field names and serialized representations must remain compatible.

For example, if the existing system serializes a property as `playerID`, do not silently change it to `playerId`, `player_id`, or `PlayerID`.

Do not replace a 64-bit identifier with a 32-bit integer. Do not convert an integer enum into a string enum. Do not change a nullable value into a non-nullable value. Do not introduce `omitempty` into JSON tags without proving that the original serializer omitted the field under the same circumstances.

Do not assume Go's zero values reproduce the source language's defaults.

Inspect the original serializer and runtime behavior, then reproduce the externally observable result.

Create automated compatibility tests for every externally observable schema and serialization contract.

### 3.3 No fake implementations

Never substitute an incomplete placeholder for working behavior.

Forbidden examples include:

- Returning an empty success response instead of executing the original operation.
- Returning hardcoded player, server, session, or lobby information.
- Replacing actual authentication with unconditional authorization.
- Replacing persistence with in-memory maps when the original system uses a database.
- Returning an empty collection instead of implementing a query.
- Replacing a real EOS operation with a simulated success.
- Ignoring socket messages that are not immediately understood.
- Replacing asynchronous operations with no-op functions.
- Catching every error and returning success.
- Creating stub methods solely to make compilation succeed.
- Leaving `TODO` comments where required behavior should be implemented.
- Disabling tests that expose migration defects.
- Removing unsupported endpoints or message handlers.
- Ignoring a module because it is difficult to build independently.

If an integration cannot be completed because credentials, proprietary binaries, hardware, or a third-party service are unavailable, implement the real integration as far as possible, isolate the environmental limitation, and provide a reproducible test strategy. Do not represent an unverified integration as complete.

### 3.4 Preserve behavior, not just apparent intent

The original implementation may contain undocumented semantics, unusual defaults, edge cases, or compatibility quirks.

Where existing behavior is observable, preserve it unless a change is explicitly authorized.

Do not automatically fix an existing bug during the migration if doing so changes behavior. Document the issue separately and preserve compatibility by default.

Do not replace the original architecture with a generic REST API, a simplified microservice, or an unrelated Go framework merely because it is easier to implement.

---

## 4. MANDATORY GRAPHIFY SKILL WORKFLOW

You MUST use the Graphify skill to build and maintain a persistent understanding of the repository.

Graphify is not optional documentation. It is a required part of the migration workflow.

Its purpose is to prevent context loss, identify dependencies, preserve implementation details, and stop important functionality from being forgotten across a large multi-stage port.

### 4.1 Discover and initialize Graphify

Before undertaking a large implementation:

1. Inspect the available Claude Code skills and installed tooling.
2. Find the actual Graphify skill and read its instructions completely.
3. Follow the installed skill's prescribed commands, setup process, graph format, and persistence mechanisms.
4. Determine whether Graphify is already initialized in this repository.
5. Reuse and update an existing graph when appropriate instead of destroying it.
6. If the skill is unavailable, report that limitation accurately, investigate whether it can be made available through the permitted environment, and use a documented dependency-graph fallback only if necessary.

Do not invent Graphify commands, assume a particular installation layout, or claim the graph was generated without verifying the results.

If Graphify has already been configured, preserve its existing knowledge and extend it.

### 4.2 Build a graph of the original implementation

Analyze the original code and represent its important relationships using Graphify's supported graph format.

Capture, wherever applicable:

- Projects and modules.
- Source files and their responsibilities.
- Classes, structs, interfaces, and functions.
- Public and private APIs.
- Call relationships.
- Inheritance and interface implementations.
- Dependency injection registrations.
- Constructor dependencies.
- Service lifetimes.
- Routes and handlers.
- Socket message types and handlers.
- Data models and their consumers.
- Serialization and deserialization paths.
- Database operations.
- External services and SDK integrations.
- Configuration keys and their consumers.
- Startup and shutdown sequences.
- Threading and asynchronous execution.
- Native interfaces and exported symbols.
- Build targets and platform-specific implementations.
- Tests and the functionality they verify.

Do not create a graph that merely lists filenames. It must capture the meaningful relationships that explain how the application actually works.

### 4.3 Use the graph during implementation

Before porting any component:

1. Query Graphify for the component and its dependencies.
2. Identify upstream callers and downstream dependencies.
3. Inspect the original implementation directly.
4. Determine the exact behavior and contracts.
5. Implement the corresponding Go functionality.
6. Update the graph to associate the original implementation with its Go counterpart.
7. Link associated tests and compatibility requirements.
8. Record any unresolved dependency or behavior.
9. Verify that dependent components still have equivalent functionality.

The graph must distinguish between:

- Analyzed.
- Implemented.
- Compiled.
- Unit-tested.
- Integration-tested.
- Compatibility-tested.
- Fully verified.
- Blocked by an external dependency.

Do not treat these states as interchangeable.

### 4.4 Persist knowledge across sessions

Use Graphify's supported persistence workflow to preserve decisions, discoveries, and implementation status.

Also maintain repository-level migration documentation so that progress does not depend solely on conversation memory.

Create or update appropriate documents such as:

- `PORTING_PLAN.md`
- `MIGRATION_INVENTORY.md`
- `COMPATIBILITY_MATRIX.md`
- `MIGRATION_STATUS.md`
- `ARCHITECTURE.md`
- `KNOWN_ISSUES.md`
- `VALIDATION_REPORT.md`

Adapt these filenames if the repository already has an established documentation structure.

Avoid duplicating information unnecessarily. Link documents to the relevant graph entities and source files where possible.

Before finishing any work session, persist the current state, including the next concrete tasks, files modified, tests run, failures encountered, and unresolved dependencies.

At the beginning of a resumed session, read the Graphify skill instructions as needed, consult the graph, and inspect the migration status before continuing.

**Never rely exclusively on conversation context to remember what remains to be ported.**

---

## 5. PHASE ONE — COMPLETE SOURCE RECONNAISSANCE

Do not begin by immediately generating Go files.

First, understand the existing system.

### 5.1 Inventory the repository

Recursively inspect the relevant project directories.

Identify:

- Source files.
- Project and solution files.
- Build scripts.
- Dependency manifests and lockfiles.
- Unreal `.Build.cs` files.
- Unreal `.Target.cs` files.
- Unreal plugin descriptors.
- Unreal module descriptors.
- CMake files, if present.
- Package manifests.
- Protocol definitions.
- Generated code.
- Tests.
- Configuration files.
- Dockerfiles and deployment manifests.
- CI/CD configuration.
- Third-party SDK wrappers.
- Binary dependencies.
- Platform-specific code.
- Documentation.

Determine which directories contain actual source and which contain generated output, external dependencies, or build artifacts.

Do not waste time indiscriminately porting generated binaries or third-party implementation code. Determine whether generated code should be regenerated using an equivalent toolchain, wrapped, or replaced with compatible first-party code.

### 5.2 Establish the original build baseline

Where tooling and dependencies permit:

1. Build the original backend.
2. Build the relevant Unreal modules.
3. Run the existing tests.
4. Record build warnings and errors.
5. Record runtime startup behavior.
6. Record existing failures before migration.
7. Capture relevant API schemas and protocol examples.
8. Record configuration requirements.
9. Identify required external services and SDK versions.

Do not misattribute pre-existing failures to the Go migration.

If an original build cannot run, document exactly why and use source-level inspection and other available evidence to establish a baseline.

### 5.3 Trace the complete execution flow

Follow actual execution paths from startup to shutdown.

For every significant operation, identify:

- Entry point.
- Input validation.
- Authentication and authorization.
- Dependency resolution.
- Business logic.
- State changes.
- External calls.
- Database operations.
- Events and callbacks.
- Serialization.
- Response delivery.
- Error propagation.
- Cleanup and resource disposal.

Trace cross-module calls rather than assuming a module operates independently.

Investigate reflection, dynamically loaded classes, event subscriptions, macros, generated interfaces, and registration mechanisms that ordinary text searches may miss.

### 5.4 Create a definitive feature inventory

Every discovered feature must have a stable tracking identifier.

For example:

- `API-001`: existing HTTP endpoint.
- `SOCKET-001`: socket message handler.
- `MODEL-001`: serialized data structure.
- `EOS-001`: EOS operation.
- `DI-001`: dependency registration.
- `HOST-001`: hosting lifecycle operation.
- `DB-001`: database behavior.
- `NATIVE-001`: Unreal/native interface.
- `CONFIG-001`: configuration contract.

These are examples of the tracking scheme, not claims about features already discovered.

For every item, record:

- Original source location.
- Original implementation.
- Inputs and outputs.
- Dependencies.
- Externally observable behavior.
- Data contracts.
- Error behavior.
- Current implementation status.
- Go implementation location.
- Relevant tests.
- Evidence supporting completion.

No feature should disappear from the inventory just because its implementation is inconvenient.

---

## 6. PHASE TWO — DETERMINE THE CORRECT GO ARCHITECTURE

The original repository contains Unreal Engine modules. You must carefully distinguish the backend runtime from code that Unreal Engine itself loads and executes.

**Do not assume that an Unreal Engine C++ module can simply be converted into a native Go package and loaded by Unreal in the same way.**

Go and Unreal have different runtime, build, reflection, object-lifetime, and ABI requirements.

You must determine the correct migration boundary from the actual code.

### 6.1 Investigate every module

Explicitly inspect these modules:

#### `AIUnrealFeatures.AAMothership`

Determine its actual responsibility, public API, internal dependencies, state management, events, and interactions with other modules.

Port its backend-relevant functionality and preserve every existing contract.

#### `AIUnrealFeatures.Ares`

Determine whether it implements gameplay-adjacent functionality, server coordination, AI-related functionality, network behavior, or something else. Do not infer its purpose from its name.

Preserve all discovered functionality and dependencies.

#### `AIUnrealFeatures.DependencyInjection`

Inventory every service registration, dependency resolution rule, scope, lifetime, factory, initialization rule, and disposal behavior.

Implement equivalent dependency management in Go. Do not silently replace scoped or transient services with global singletons.

#### `AIUnrealFeatures.EOSSDK`

Inventory every EOS-related operation, SDK wrapper, callback, authentication flow, session interaction, identity mapping, resource lifetime, error mapping, and native dependency.

Determine which behavior can run entirely in Go, which requires a supported Go integration, and which requires the original native EOS SDK or an Unreal-facing bridge.

Do not replace real EOS functionality with mocks in production.

#### `AIUnrealFeatures.HalcyonSocket`

Inventory the complete socket implementation, including:

- Transport and protocol.
- Message formats.
- Message identifiers.
- Serialization.
- Connection lifecycle.
- Authentication.
- Handshake behavior.
- Heartbeats and keepalives.
- Reconnection behavior.
- Timeouts.
- Ordering guarantees.
- Concurrency.
- Backpressure.
- Message dispatch.
- Error handling.
- Disconnect handling.
- Resource cleanup.

Preserve the existing wire protocol exactly unless the source proves otherwise.

Do not replace a custom binary or stateful protocol with generic WebSockets or REST simply because they are easier to implement.

#### `AIUnrealFeatures.Hosting`

Inventory server startup, host configuration, dependency initialization, background services, lifecycle hooks, graceful shutdown, health reporting, and deployment behavior.

Port the actual hosting behavior rather than creating a basic Go HTTP server that lacks the original capabilities.

### 6.2 Decide what runs in Go and what remains as a compatibility bridge

Produce an explicit architecture decision document before committing to a migration boundary.

Consider the following options based on the actual implementation:

- Go backend service.
- Go packages for migrated business logic.
- Go-native network and protocol implementations.
- A Go executable communicating with Unreal through a documented IPC or network protocol.
- A small Unreal C++ compatibility layer where Unreal APIs, reflection, UObject lifecycle, delegates, or engine-specific SDK access are required.
- C-compatible exports only where the ABI, calling conventions, runtime behavior, and deployment requirements have been verified.
- A supported external SDK integration where a native Unreal dependency cannot be replaced directly.

Prefer a clean separation of Go-owned server logic and the minimum required Unreal-facing compatibility layer.

However, **do not delete or hollow out a module merely to achieve an all-Go source tree**.

If an Unreal module must remain in C++ to preserve engine integration, keep it as a functional bridge and port its backend/business logic to Go. Document the reason and prove that its behavior remains intact.

If the user requires absolutely every source component to become Go, explicitly identify any technically incompatible Unreal-specific functionality before proposing the safest alternative. Do not falsely claim that a Go package is a drop-in Unreal module.

### 6.3 Preserve all public integration contracts

Document and preserve:

- Exported functions.
- Unreal reflection names.
- UCLASS, USTRUCT, UENUM, and UFUNCTION behavior where applicable.
- Delegate signatures.
- Callback order.
- Module initialization and shutdown.
- Network addresses and ports.
- IPC messages.
- Socket message contracts.
- Shared-library symbols.
- C ABI definitions.
- Configuration interfaces.
- Any public C++, C#, JSON, or binary interfaces.

If an interface changes internally, provide a compatible adapter wherever necessary.

---

## 7. PHASE THREE — PORT THE BACKEND TO GO

After reconnaissance and architecture planning, begin implementation.

### 7.1 Use idiomatic Go without changing behavior

Use a supported Go version appropriate for the target environment and pin it in the project configuration.

Use Go modules and appropriate dependency management.

Organize the project into understandable packages, but do not create unnecessary abstractions or rewrite logic without a compatibility reason.

Possible architectural boundaries include:

- Application startup.
- Configuration.
- Domain logic.
- Service interfaces.
- Transport handlers.
- Protocol implementations.
- Persistence.
- External integrations.
- Dependency initialization.
- Logging and telemetry.
- Background services.
- Unreal compatibility.
- Tests.

These are architectural suggestions, not a requirement to impose an arbitrary folder structure on the source.

Do not introduce a dependency solely because it is fashionable. Evaluate its maintenance, security, compatibility, and runtime requirements.

### 7.2 Port actual business logic

Translate each source operation into equivalent Go behavior.

Do not translate only the public method signatures while omitting the method bodies' semantics.

For every original function or operation, account for:

- Normal execution.
- Boundary conditions.
- Invalid inputs.
- Missing data.
- Concurrent execution.
- Error propagation.
- Side effects.
- Resource ownership.
- Transaction boundaries.
- Cancellation.
- Retry behavior.
- Timing assumptions.
- Return values.

Preserve ordering and atomicity where the original implementation relies on them.

Where the source runtime and Go differ, implement explicit compatibility helpers rather than relying on Go's default behavior.

### 7.3 Preserve asynchronous behavior

Inspect the original threading and asynchronous model.

Determine whether operations use callbacks, tasks, futures, event loops, dedicated threads, timers, or background services.

Translate these into appropriate Go mechanisms such as goroutines, channels, contexts, synchronization primitives, and explicit lifecycle management.

However, do not assume that translating an asynchronous method into a goroutine automatically preserves its behavior.

Preserve:

- Completion semantics.
- Ordering guarantees.
- Cancellation.
- Exception/error propagation.
- Synchronization.
- Resource ownership.
- Shutdown behavior.
- Callback execution context where externally observable.

Prevent data races, deadlocks, goroutine leaks, and shutdown hangs.

### 7.4 Preserve configuration

Inventory every existing configuration key, including keys loaded from:

- JSON.
- YAML.
- TOML.
- Environment variables.
- Command-line arguments.
- Unreal configuration files.
- Project settings.
- Deployment manifests.
- Secret providers.
- Database records.
- Runtime service discovery.

Preserve existing key names, defaults, precedence rules, validation behavior, and environment-variable semantics.

Do not silently replace Unreal `.ini` configuration with a new format that requires undocumented manual changes.

If a new Go configuration format is required, implement a compatibility loader or a documented, tested conversion that preserves all effective settings.

Never place credentials in source control or logs.

### 7.5 Preserve persistence

If the original backend uses persistent storage, port its real storage operations.

Inspect the database schema, migrations, queries, indexes, transactions, constraints, and relationships.

Preserve:

- Existing database identifiers.
- Field types.
- Nullability.
- Default values.
- Uniqueness.
- Referential integrity.
- Transaction semantics.
- Query behavior.
- Ordering.
- Pagination.
- Migration compatibility.
- Existing records.

Do not recreate an empty database and declare the migration complete.

Do not drop or rename production tables, delete data, or execute destructive migrations without explicit authorization.

### 7.6 Preserve errors and observability

Inventory original error types, codes, status mappings, logs, and diagnostics.

Reproduce their externally observable equivalents.

Use Go's error wrapping and structured logging where appropriate, but do not expose internal stack traces or secrets to clients if the original system did not.

Preserve relevant operational telemetry and provide equivalent health and diagnostic endpoints only when they fit the original contract or are added without breaking it.

---

## 8. PHASE FOUR — STRICT API AND PROTOCOL COMPATIBILITY

Every external contract must be documented and tested.

### 8.1 HTTP compatibility

For each original endpoint, record:

- HTTP method.
- Route and route parameters.
- Query parameters.
- Headers.
- Authentication requirements.
- Request body.
- Response body.
- Status codes.
- Content types.
- Cookies.
- Redirects.
- Error responses.
- Timeout behavior.
- Side effects.
- Idempotency behavior.

Preserve the original route and response contract.

Do not replace a versioned API with a new version unless explicitly authorized.

Do not silently change case sensitivity, trailing-slash behavior, default content types, or request parsing rules.

### 8.2 Socket and binary protocol compatibility

If the source implements a socket protocol, document its exact framing and wire representation.

Preserve message IDs, payload schemas, ordering, acknowledgement behavior, authentication, heartbeat semantics, and connection lifecycle.

Create protocol fixtures from the original implementation where possible.

Test that the Go implementation can consume original-client messages and produce responses the original clients accept.

If the original implementation uses proprietary serialization, inspect its implementation rather than assuming JSON.

### 8.3 Serialization compatibility

For each data model, create test cases covering:

- Fully populated objects.
- Missing fields.
- Explicit nulls.
- Empty strings.
- Zero-valued numbers.
- False booleans.
- Empty collections.
- Unknown fields.
- Unknown enum values.
- Large integers.
- Nested objects.
- Special characters.
- Unicode.
- Timestamp edge cases.
- Invalid input.
- Version compatibility.

Compare the original and Go outputs structurally and byte-for-byte where byte-level compatibility is required.

JSON object key order need not be identical if the protocol does not depend on it, but property names, value types, omitted properties, and semantics must remain identical.

For binary protocols, preserve exact bytes where the wire format requires them.

### 8.4 Authentication and security compatibility

Preserve existing authentication and authorization rules, identity formats, token validation, expiration, permission checks, and session semantics.

Do not weaken security to simplify migration.

Do not print access tokens, private keys, passwords, or secrets into Graphify, documentation, tests, logs, or generated reports.

Use synthetic or redacted examples when recording sensitive configuration structures.

---

## 9. PHASE FIVE — BUILD AND TEST THE MIGRATED SYSTEM

Testing is a mandatory part of the migration, not a final optional step.

### 9.1 Create a migration test suite

Create tests for every inventory item that can be tested in isolation.

Include:

- Unit tests.
- Integration tests.
- API compatibility tests.
- Serialization tests.
- Protocol tests.
- Database tests.
- Authentication tests.
- Concurrency tests.
- Lifecycle tests.
- Configuration tests.
- Regression tests.
- Unreal-to-Go bridge tests.
- Deployment and startup tests.

Preserve the original tests where applicable and add Go equivalents.

### 9.2 Differential testing

Where the original runtime is executable, use the original implementation as the behavioral reference.

For identical inputs, compare the original and Go outputs.

Compare:

- Return values.
- Response bodies.
- Status codes.
- Error codes.
- Serialized fields.
- Database state changes.
- Events.
- Socket messages.
- Side effects.
- Ordering.
- Timing constraints where material.

Normalize only genuinely irrelevant differences, such as nondeterministic identifiers or timestamps, and document every normalization.

Do not normalize away meaningful behavioral differences.

For nondeterministic operations, establish explicit invariants and compare those invariants instead of pretending the results must be identical.

### 9.3 Build verification

Run the applicable checks, including:

- `gofmt`
- `go vet`
- `go test ./...`
- Go race detection where supported.
- Static analysis.
- Dependency vulnerability checks where tooling is available.
- Native bridge builds.
- Relevant Unreal module builds.
- Integration tests.
- Packaging and deployment tests.

Use the commands appropriate to the actual project and environment.

Fix compilation errors, failing tests, data races, and migration regressions instead of documenting them as acceptable merely because the implementation is large.

If a test cannot run because of unavailable infrastructure, report the exact blocker and do not count that test as passed.

### 9.4 Performance and resource management

Establish baseline measurements where possible.

Compare throughput, latency, memory usage, CPU usage, connection handling, and startup time for critical workloads.

Do not sacrifice correctness for performance.

Investigate resource leaks, goroutine leaks, unbounded queues, uncontrolled retries, and excessive memory allocations.

The Go implementation must be production-appropriate, not merely functional in a small local test.

---

## 10. PHASE SIX — EXECUTION STRATEGY

Work incrementally and continuously validate progress.

### Step 1: Inspect

Read the repository structure, source files, module definitions, build configuration, tests, and integration dependencies.

Do not begin large-scale code generation before understanding the existing architecture.

### Step 2: Graph

Initialize or update Graphify, map the original implementation, and identify dependencies and public contracts.

### Step 3: Baseline

Build and test the original project where possible. Record known failures and compatibility fixtures.

### Step 4: Inventory

Create the complete feature inventory and identify all fields, APIs, protocols, configuration, and persistence contracts.

### Step 5: Architect

Document the Go migration boundary and determine the minimum Unreal compatibility layer required.

### Step 6: Scaffold

Create the Go project, module configuration, package structure, test infrastructure, and reproducible build commands.

Do not create an empty scaffold and stop there.

### Step 7: Port foundational infrastructure

Port configuration, error handling, logging, dependency management, serialization, and other shared infrastructure in dependency order.

### Step 8: Port individual modules

Port the actual behavior of the six listed modules and every other applicable backend component discovered during reconnaissance.

Do not assume the six screenshot directories are the entire project.

### Step 9: Integrate

Connect the Go implementation to the actual storage, external services, socket clients, hosting environment, and Unreal compatibility layer.

### Step 10: Verify

Run compatibility, regression, integration, concurrency, and build tests.

Resolve every discovered mismatch.

### Step 11: Audit

Compare the complete source inventory against the Go implementation and retained compatibility layer.

Identify missing functions, unported files, unused replacements, unimplemented branches, schema mismatches, and unresolved dependencies.

### Step 12: Deliver

Produce the final source, build instructions, deployment instructions, migration documentation, compatibility report, test results, and an accurate list of remaining blockers.

You must continue working through the phases instead of stopping after planning, inventory, or scaffolding.

If the task is too large for one session, complete a coherent portion, persist all findings, and continue from the recorded state. Never claim the entire migration is finished when only a subset has been ported.

---

## 11. MIGRATION COVERAGE AUDIT

Before declaring completion, conduct a systematic comparison of the original source and the migrated system.

### 11.1 Symbol-level comparison

Where possible, inventory and compare:

- Classes.
- Structs.
- Interfaces.
- Enums.
- Public functions.
- Internal functions.
- Methods.
- Properties.
- Constants.
- Events.
- Delegates.
- Exported symbols.
- Message handlers.
- API endpoints.
- Configuration keys.
- Serialization models.
- Database operations.
- Background workers.
- Dependency registrations.

For each item, record one of these outcomes:

1. Ported to Go.
2. Preserved in a required Unreal compatibility bridge.
3. Replaced by an explicitly documented equivalent with demonstrated compatibility.
4. Proven generated or external code handled by an appropriate equivalent mechanism.
5. Blocked by a specific external dependency, with the remaining work documented.

Do not classify an item as ported merely because a Go function with a similar name exists.

Its behavior and contracts must be verified.

### 11.2 Branch-level comparison

Inspect conditional branches, exception handlers, fallback logic, validation paths, retry loops, and failure handling.

Check that the Go implementation does not omit behavior that only occurs under unusual circumstances.

### 11.3 Field-level comparison

Build a schema inventory covering every externally observable data structure.

For every field, verify its name, type, nullability, defaults, serialization behavior, and use throughout the application.

The audit must identify both missing fields and fields whose behavior has changed.

### 11.4 Dependency-level comparison

Compare original dependency relationships with the migrated architecture.

Verify that each required dependency has a working equivalent and that the new implementation does not introduce circular dependencies, accidental global state, or broken initialization order.

### 11.5 No false completion

Do not claim 100% parity if any required behavior remains unimplemented or unverified.

Report separate coverage figures for:

- Source inventory completed.
- Functions and symbols accounted for.
- Data schemas verified.
- API and protocol contracts tested.
- Integration tests passed.
- Runtime behavior verified.
- External blockers remaining.

Define the denominator for each percentage and base the figures on actual tracked items.

A module that compiles but has unimplemented handlers is not complete.

A test that exists but has not been executed is not a passing test.

A bridge that builds but has not been tested against its actual Unreal caller is not integration-verified.

---

## 12. CODE QUALITY AND REPOSITORY RULES

The migrated implementation must be maintainable and reproducible.

- Use clear package boundaries.
- Avoid monolithic source files.
- Avoid unnecessary abstractions.
- Avoid global mutable state unless required and justified.
- Handle errors explicitly.
- Use contexts for cancellable operations.
- Manage goroutines and resources safely.
- Add tests for compatibility-sensitive behavior.
- Document public interfaces.
- Pin important dependency versions appropriately.
- Keep secrets out of the repository.
- Use reproducible build commands.
- Keep generated code distinguishable from handwritten code.
- Preserve licensing and third-party notices.
- Do not copy third-party code into the migration without respecting its license.
- Do not make unrelated changes to the original project.
- Do not modify files outside the intended migration scope without documenting why.

Maintain a clear separation between original source, migrated Go code, compatibility bridges, generated code, and test fixtures.

Use a dedicated migration directory or a separate Go project when that best protects the original implementation and simplifies comparison.

Do not delete original files merely because their functionality has been ported.

Do not commit build artifacts, local credentials, private tokens, or personal environment data.

---

## 13. REQUIRED DELIVERABLES

Produce all applicable deliverables in the actual repository.

### A. Complete Go implementation

A working Go project containing the migrated backend and applicable module functionality.

It must include real implementations rather than placeholders.

### B. Unreal compatibility layer

Where technically necessary, preserve or implement the minimum native Unreal-facing layer required to maintain existing engine integration.

Document its public API and how it communicates with Go.

### C. Complete migration inventory

A source-to-target mapping for all relevant original components, including files, symbols, contracts, and their migration status.

### D. Graphify knowledge graph

A populated and maintained Graphify graph that describes the original system, the migrated architecture, dependencies, and implementation status.

### E. Compatibility matrix

A matrix mapping every original API, model, protocol, configuration contract, and significant behavior to its equivalent and its verification tests.

### F. Automated tests

A reproducible suite of tests that demonstrates behavioral compatibility.

### G. Build and deployment documentation

Document:

- Required Go version.
- Required native toolchains.
- Dependency installation.
- Environment configuration.
- Local development.
- Build commands.
- Test commands.
- Running the backend.
- Database initialization and migrations.
- External service configuration.
- Unreal bridge setup.
- Packaging.
- Deployment.
- Logging.
- Troubleshooting.
- Shutdown and restart behavior.

### H. Migration audit report

Summarize what was ported, what remains native, how compatibility was tested, which tests passed, and which items remain blocked.

### I. Continuation state

Persist the current status and next actions so another session can continue without repeating the entire investigation.

---

## 14. WORKING STYLE AND AUTONOMY

Work directly in the available repository when the necessary tools and permissions are available.

Do not merely tell me what code I should write.

Inspect the files, edit the implementation, run commands, build the project, write tests, inspect failures, fix problems, and maintain the migration records.

Do not ask me to manually enumerate source files when you can inspect them yourself.

Do not repeatedly ask for approval for routine, reversible engineering steps within the defined scope.

However, do not perform destructive operations, delete data, overwrite the original implementation, expose secrets, or make irreversible deployment changes without explicit authorization.

When a technical decision has multiple viable options, inspect the source, select the option that best preserves behavior, document the reasoning, and proceed.

If credentials or third-party services are unavailable, implement and test everything possible without fabricating successful integration results.

If a build fails, investigate the actual error rather than making unrelated changes or disabling the failing test.

If context limits or session limits interrupt the work, update Graphify and the migration documents before stopping.

### Progress reporting

Provide concise, evidence-based progress reports at meaningful milestones.

Each report should state:

- What was inspected.
- What was implemented.
- What tests were executed.
- What passed or failed.
- What remains.
- What will be done next.

Distinguish completed work from planned work.

Do not claim to have run a command unless you actually ran it.

Do not claim a file was inspected unless you actually inspected it.

Do not claim a module is complete merely because its Go package exists.

---

## 15. FINAL ACCEPTANCE CRITERIA

The migration is accepted only when all applicable requirements below are satisfied.

- [ ] The entire relevant source tree has been inspected.
- [ ] The original implementation has been preserved.
- [ ] Graphify has been initialized or updated and used throughout the migration.
- [ ] The original architecture and cross-module dependencies have been documented.
- [ ] Every relevant feature has a tracked inventory entry.
- [ ] All six identified Unreal-related modules have been investigated.
- [ ] All additional relevant modules and backend components have been accounted for.
- [ ] All applicable backend logic has been ported to Go.
- [ ] Any required Unreal compatibility bridge remains functional.
- [ ] Every existing field and serialized contract has been accounted for.
- [ ] API routes, methods, responses, status codes, and errors remain compatible.
- [ ] Socket and binary protocols remain compatible where applicable.
- [ ] Authentication and authorization remain intact.
- [ ] Database schemas and persistence behavior remain compatible.
- [ ] Configuration keys, defaults, and precedence remain compatible.
- [ ] Dependency injection and service lifetimes remain behaviorally equivalent.
- [ ] EOS integration is implemented or its specific external blocker is documented honestly.
- [ ] Startup, shutdown, background services, and resource cleanup are implemented.
- [ ] No required functionality has been replaced with placeholders.
- [ ] Unit, integration, and compatibility tests have been executed where possible.
- [ ] All critical tests pass or have clearly documented, evidence-based blockers.
- [ ] Build and deployment instructions have been verified.
- [ ] Source-to-target coverage has been audited.
- [ ] The final report distinguishes verified functionality from unverified functionality.

A checkmark requires evidence.

If a requirement cannot be completed, identify the precise source component, missing dependency, failed test, or technical limitation. Do not hide it by marking the requirement complete.

---

## 16. YOUR FIRST ACTIONS

Begin now.

**First:** inspect the source directory and repository structure. Confirm the available tools and determine how to access the actual files.

**Second:** locate and read the Graphify skill instructions. Initialize or update the graph according to the skill's real workflow.

**Third:** inspect the backend entry points, project files, Unreal module definitions, and dependency registrations. Determine the source language, runtime, architecture, and build requirements.

**Fourth:** build the initial source-to-target inventory and identify the public contracts that must remain unchanged.

**Fifth:** establish the original build and test baseline, then document the correct Go migration architecture.

**Sixth:** begin implementing and validating the migration in dependency order. Continue until the deliverables and acceptance criteria are satisfied, or a specific external blocker prevents further progress.

Do not stop after producing a plan.

Do not stop after creating a Go scaffold.

Do not implement only the HTTP server while leaving socket handling, EOS, hosting, dependency injection, or other existing functionality behind.

Do not change field names, omit models, simplify the protocol, or discard branches of business logic.

Do not confuse a successful compilation with a successful port.

**Your overriding objective is complete behavioral parity with the original Halcyon backend and all applicable module functionality, implemented in Go wherever technically appropriate, with every field preserved and every migration decision traceable through Graphify and automated tests.**

Start by examining the real source code. Let the source determine the implementation, not assumptions about what a typical backend should look like.