# Architecture

[中文](zh-CN/architecture.md)

## Layered Design

Atlas is divided into entry layer, orchestration layer, core loop, capability layer, and persistence layer. All entry points share the same `runtime.Runtime`. The core agent loop remains headless, dependency-injected, and independently testable; runtime owns configuration, persistence, and orchestration.

```mermaid
graph TD
    subgraph Entry Layer
        TUI[Terminal UI]
        CLI[CLI Commands]
        ACP[ACP Adapter]
        WS[WebSocket Channel]
    end

    subgraph Orchestration Layer
        RT[runtime.Runtime]
    end

    subgraph Core Loop
        AG[agent loop]
        PR[Provider Interface]
    end

    subgraph Capability Layer
        TOOLS[Tool Registry]
        PRMPT[System Prompt]
        CMP[Context Compaction]
    end

    subgraph Persistence Layer
        TR[transcript]
        SS[session SQLite]
    end

    TUI --> RT
    CLI --> RT
    ACP --> RT
    WS --> RT
    RT --> AG
    AG --> PR
    AG --> TOOLS
    RT --> PRMPT
    RT --> CMP
    AG --> TR
    RT --> SS
```

The TUI is an entry-layer adapter like ACP and WebSocket. It converts keyboard and mouse input into runtime turns and manual compaction requests, consumes ordered observer events for streaming output and tool activity, and leaves orchestration and persistence in `runtime.Runtime`.

## Core Loop

A turn starts with user input: appended to the transcript, then the model is called in a loop. When the model returns text deltas, they are streamed out; when it returns tool calls, they are executed in order and results are written back to the transcript. The loop ends when there are no tool calls, an error occurs, or the step limit is reached.

```mermaid
sequenceDiagram
    participant RT as runtime.RunTurn
    participant AG as agent loop
    participant PR as provider.Stream
    participant TR as transcript
    participant TOOLS as tool.Registry

    RT->>AG: user input
    AG->>TR: append user message
    loop step < maxSteps
        AG->>PR: Stream(ChatRequest)
        PR-->>AG: streaming delta
        PR-->>AG: ChatResponse
        AG->>TR: append assistant message
        alt no tool calls
            AG-->>RT: return final reply
        else has tool calls
            loop each tool call in order
                AG->>TOOLS: Run(call)
                TOOLS-->>AG: result or error
                AG->>TR: append tool message
            end
        end
    end
    AG-->>RT: step limit exceeded
```

Key constraints:

- Every tool call has a paired tool result, in the same order the model returned them.
- Tool errors are written back as model-visible tool results, letting the model adjust accordingly.
- Observer events preserve occurrence order so streaming clients can render model output, tool calls, and completion without regrouping them.
- The loop ends when there are no tool calls, an error occurs, or `max_steps` (default 20) is reached.

## Context Compaction and Todos

Runtime triggers context compaction automatically at the configured threshold. Entry adapters and CLI commands can also request it manually through `CompactSession`. Both paths summarize earlier messages, keep recent messages active, and preserve the full transcript. If the model has been using `todo_write` to track tasks, the last todo list is extracted from the transcript and incomplete items are injected into the summary prompt. This keeps pending work available after compaction without persisting todo state to the database.
