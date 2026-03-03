---
id: event_listener
type: implementation
path_filters: ["./decentralized-api/internal/event_listener/*.go"]
---

### Event Listener Dispatcher Routine Overview

The event listener in the Gonka project's `decentralized-api` uses a specialized worker pool and queueing system to handle blockchain events (blocks and transactions) asynchronously and concurrently.

#### Worker Configuration
The current implementation uses the following distribution:

*   **Block Events (`processBlockEvents`)**: There are **2 workers** dedicated to processing block events (labeled `process_block_events`).
*   **Other Events (`processEvents`)**: There are **10 workers** dedicated to processing other events (labeled `process_events_0` through `process_events_9`). These primarily handle transaction (Tx) events and system barriers sourced from the `BlockObserver`.

block events get their own queue because in a high-load scenario, the normal queue can get filled with events waiting for the next block. 

#### Queueing Mechanism: `UnboundedQueue`
The system uses a custom `UnboundedQueue[T]` implementation (found in `decentralized-api/internal/event_listener/unbounded_queue.go`). Key characteristics include:

1.  **Thread-Safe FIFO**: It is a first-in, first-out queue that uses Go channels (`In` and `Out`) for communication.
2.  **Internal Management**: A background goroutine (`manage()`) maintains an internal slice (`items []T`) that acts as the unbounded buffer.
3.  **Flow**:
    *   Producers send events to the `In` channel.
    *   The `manage` routine appends these to the internal slice.
    *   When the slice is non-empty, the `manage` routine attempts to send the first item to the `Out` channel whenever a worker is ready to receive.

#### Selection Mechanism: Go Channel Multiplexing
The selection mechanism for which worker gets an event is the native **Go channel selection/competition**:

*   **Competition**: All workers for a specific queue (e.g., the 10 "other" workers) are listening on the same `eventQueue.Out` channel within a `select` statement inside the `worker` function.
*   **Non-Deterministic Selection**: Go's runtime selects one available goroutine from the set of goroutines blocked on the channel. While this often behaves similarly to round-robin in high-load scenarios (distributing work among idle goroutines), the Go specification does not guarantee a strict round-robin order; it is essentially "whichever idle worker is picked by the scheduler."

#### Routine Dispatch Logic
The `listen` routine (in `event_listener.go`) acts as the primary dispatcher for incoming websocket messages:

1.  It reads raw messages from the blockchain's websocket.
2.  It unmarshals them into `JSONRPCResponse` objects.
3.  **Classification**:
    *   If the event type is `NewBlock`, it is sent to the `blockQueue.In` (consumed by the 2 block workers).
    *   Transaction events are currently ignored in the `listen` loop because they are polled and injected into the `mainQueue` by the `BlockObserver.Process` routine (which then feeds the 10 general workers).